// 本文件在进入安全区时捕获原绑定事实，按房间来源顺序提交并同步可信绑定。
package game

import (
	"encoding/json"
	"fmt"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"time"

	"minimir/internal/devconfig"
)

// 唯一槽位属于永久owner；原请求/授权/事件ID冻结至确定终态，不能由当前坐标重建。
type safeZoneBindingFact struct {
	// request 记录请求，使用安全区域会话绑定请求表示。
	request persistence.SafeZoneBindingRequest
	// auth 记录授权。
	auth persistence.Auth
	// room 记录房间。
	room *room
	// conn 记录连接。
	conn Connection
	// actor 记录行动实体，使用行动实体引用表示。
	actor actorRef
	// scene 记录场景，使用数值表示。
	scene uint64
	// order 记录顺序，使用数值表示。
	order uint64
	// movement 记录移动，使用数值表示。
	// cellEntry 记录地图格入场，使用数值表示。
	movement, cellEntry uint64
	// center 记录中心格，使用角色会话绑定表示。
	center domain.RoleBinding
	// zoneID 标识当前业务使用的区域。
	zoneID uint64
	// fingerprint 记录“fingerprint”，使用文本表示。
	fingerprint string
}

// 只为本次合法walk预备候选。来源HumanObject.Walk:2480在成功落格后绑定，不能普挂relocate。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，e 记录实体，point 记录点位。
func (w *worker) prepareSafeZoneWalk(r *room, e *entity, point devconfig.Point) (*safeZoneBindingFact, bool) {
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	if !f.SafeZoneBindingEnabled() || e != &r.Player {
		return nil, true
	}
	// m 接收当前状态地图供后续步骤使用，known 保存由当前状态地图取得的已知状态。
	m, known := w.currentMap(r)
	if !known {
		return nil, false
	}
	// zone 保存由安全区域发生时间取得的区域，inside 保存由安全区域发生时间取得的“inside”。
	zone, inside := m.SafeZoneAt(point)
	if !inside {
		return nil, true
	}
	if !m.Walkable(point) || zone.ID == 0 || zone.Center.X < 0 || zone.Center.Y < 0 || zone.Center.X > math.MaxInt32 || zone.Center.Y > math.MaxInt32 || !m.Walkable(zone.Center) {
		return nil, false
	}
	// center 建立带初始值的角色会话绑定，供本段继续处理。
	center := domain.RoleBinding{MapID: m.ID, X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
	if r.Role.Binding != nil && *r.Role.Binding == center && r.Role.BindingSourceSeq > 0 && r.Role.BindingSafeZoneID > 0 {
		return nil, true
	}
	// 旧kill来源先结清。只接已附着玩家的真实行走，不替spawn/join/复活伪造动作。
	if r.PendingBinding != nil || len(r.KillFacts) > 0 || r.Isolated || !r.Ready || r.Conn == nil || !r.Conn.Alive() || r.Role.ID == "" || r.Auth.RoleID != r.Role.ID || w.rooms[r.Role.ID] != r || r.Auth.SessionEpoch <= 0 || r.Conn.Binding() != uint64(r.Auth.SessionEpoch) || r.Role.SourceSeq < 0 || r.Role.SourceSeq == math.MaxInt64 || r.Role.Revision <= 0 || r.Role.Revision == math.MaxInt64 {
		return nil, false
	}
	if distance(r.Player.Pos, point) != 1 || !w.neighbor(r, r.Player.Pos, point, e) {
		return nil, false
	}
	// q 组装安全区域会话绑定请求所需的数据。
	q := persistence.SafeZoneBindingRequest{SourceSeq: r.Role.SourceSeq + 1, ExpectedRevision: r.Role.Revision, ExpectedRuleVersion: f.RuleVersion, ExpectedConfigHash: f.Hash, MapID: m.ID, Position: point, Action: "walk", EventID: domain.ID()}
	// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	raw, err := json.Marshal(q)
	if err != nil {
		return nil, false
	}
	return &safeZoneBindingFact{request: q, auth: r.Auth, room: r, conn: r.Conn, scene: r.Scene, center: center, zoneID: zone.ID, fingerprint: persistence.Hash(raw)}, true
}

// submitSafeZoneBinding 为原绑定更新登记在途状态并投递持久任务。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，fact 记录已发生的事实，使用安全区域会话绑定已发生的事实表示。
// now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) submitSafeZoneBinding(r *room, fact *safeZoneBindingFact, now time.Time) {
	// 调用点已完成本次walk和独立时钟。此后既有Pending屏障停止本tick的宠物/怪物新动作。
	if fact.actor.World == (worldRef{}) {
		fact.actor = reference(r, &r.Player)
		fact.movement, fact.cellEntry = r.Player.MovementRevision, r.Player.CellEntry
	}
	r.PendingBinding = fact
	r.Pending = true
	r.PendingOperationID = fact.request.EventID
	r.PendingFingerprint = fact.fingerprint
	// store 保存产生绑定事实时的运行服视图，原操作重试不改所属围栏。
	store := w.roomStore(r)
	// pending 记录待确认，使用任务或职业编号表示。
	var pending job
	pending = job{target: w, run: func() func(*worker) {
		// role 保存由绑定安全区域带有回执取得的角色，receipt 保存由绑定安全区域带有回执取得的回执。
		// err 接收绑定安全区域带有回执过程中可能产生的错误，供后续失败分支处理。
		role, receipt, err := store.BindSafeZoneWithReceipt(w.e.ctx, fact.auth, fact.request)
		// w 记录房间执行器。
		return func(w *worker) { w.finishSafeZoneBinding(fact, role, receipt, err, &pending) }
	}}
	if !w.submit(pending) {
		r.PendingRetry = &pending
		r.RetryAt = now.Add(100 * time.Millisecond) // 复用现有永久job入队失败重试节拍。
	}
}

// bindingReceiptMatches 确认绑定位置、配置和最初来源与实际回执相符。
// fact 记录已发生的事实，使用安全区域会话绑定已发生的事实表示，role 记录角色，receipt 记录回执，使用操作表示。
func bindingReceiptMatches(fact *safeZoneBindingFact, role domain.Role, receipt persistence.Operation) bool {
	// b 保存会话绑定，用于当前步骤中的b。
	b := receipt.Binding
	return role.ID == fact.auth.RoleID && role.Revision > fact.request.ExpectedRevision && role.SourceSeq >= fact.request.SourceSeq && role.BindingSourceSeq >= fact.request.SourceSeq && role.Binding != nil && role.BindingSafeZoneID > 0 &&
		receipt.ID == fmt.Sprintf("binding:%s:%d", fact.auth.RoleID, fact.request.SourceSeq) && receipt.RoleID == fact.auth.RoleID && receipt.Kind == "safe_zone_binding" && receipt.Code == 0 && receipt.SourceSeq == fact.request.SourceSeq && receipt.Digest == fact.fingerprint && b != nil && b.EventID == fact.request.EventID && b.Action == fact.request.Action && b.Point == fact.center && b.ZoneID == fact.zoneID && b.RuleVersion == fact.request.ExpectedRuleVersion && b.ConfigHash == fact.request.ExpectedConfigHash
}

// 已成功动作的候选先占唯一未排号槽；SourceSeq/Revision只在owner事实队首冻结。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，fact 记录已发生的事实，使用安全区域会话绑定已发生的事实表示。
func (w *worker) queueBindingCandidate(r *room, fact *safeZoneBindingFact) bool {
	if fact == nil || r.BindingCandidate != nil || r.PendingBinding != nil || r.FactOrder == math.MaxUint64 || r.Isolated {
		return false
	}
	// cap 保存击杀队列容量，用于当前步骤中的上限。
	if cap := w.e.Store.Cfg.Fixture.CombatTiming.KillQueueCapacity; cap > 0 && len(r.KillFacts)+1 > cap {
		return false
	}
	r.FactOrder++
	fact.order, fact.room, fact.auth, fact.conn, fact.scene = r.FactOrder, r, r.Auth, r.Conn, r.Scene
	fact.actor = reference(r, &r.Player)
	fact.movement, fact.cellEntry = r.Player.MovementRevision, r.Player.CellEntry
	fact.request.SourceSeq, fact.request.ExpectedRevision = 0, 0
	r.BindingCandidate = fact
	return true
}

// admitBindingCandidate 在轮到绑定事实时占用下一来源流水并提交。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) admitBindingCandidate(r *room, now time.Time) {
	// fact 保存会话绑定候选，用于当前步骤中的已发生的事实。
	fact := r.BindingCandidate
	if fact == nil || r.Pending || r.Isolated {
		return
	}
	if fact.auth != r.Auth || fact.room != r || r.Role.SourceSeq == math.MaxInt64 || r.Role.Revision == math.MaxInt64 {
		w.isolateRoom(r)
		return
	}
	fact.request.SourceSeq, fact.request.ExpectedRevision = r.Role.SourceSeq+1, r.Role.Revision
	// raw 保存由序列化取得的原始值，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	raw, err := json.Marshal(fact.request)
	if err != nil {
		w.isolateRoom(r)
		return
	}
	fact.fingerprint = persistence.Hash(raw)
	r.BindingCandidate = nil
	w.submitSafeZoneBinding(r, fact, now)
}

// finishSafeZoneBinding 处理原绑定回执及未知提交，成功后同步已确认的状态。
// w 是调用该方法的房间执行器实例，fact 记录已发生的事实，使用安全区域会话绑定已发生的事实表示，role 记录角色，receipt 记录回执，使用操作表示。
// err 保存当前操作的失败原因，供调用方检查或返回，retry 记录重试，使用任务或职业编号表示。
func (w *worker) finishSafeZoneBinding(fact *safeZoneBindingFact, role domain.Role, receipt persistence.Operation, err error, retry *job) {
	// r 保存当前处理所用的房间。
	r := fact.room
	// current 保存从房间集合取出的当前状态。
	current := w.rooms[fact.auth.RoleID]
	// owns 保存根据当前状态与r的计算结果与已隔离的计算结果与授权与授权的计算结果的计算结果与待确认的计算结果与待确认会话绑定与已发生的事实的计算结果的计算结果与待确认操作标识与事件标识的计算结果的计算结果与待确认“fingerprint”与“fingerprint”的计算结果计算的“owns”。
	owns := current == r && !r.Isolated && r.Auth == fact.auth && r.Pending && r.PendingBinding == fact && r.PendingOperationID == fact.request.EventID && r.PendingFingerprint == fact.fingerprint
	if !owns {
		return
	} // 新owner/新Pending不受旧回调释放；其Enter事务读取已提交永久事实。
	if err != nil && (persistence.Code(err) == p.CodeOperationPending || persistence.Code(err) == p.CodeServerBusy) {
		r.PendingRetry = retry
		r.RetryAt = time.Now().Add(500 * time.Millisecond) // 与既有Settle原job重试保持相同节拍。
		return
	}
	if err != nil || !bindingReceiptMatches(fact, role, receipt) {
		r.PendingRetry = nil
		w.isolateRoom(r) // 保留原fact供诊断；不能将不确定/损坏回执冒充已验证绑定。
		return
	}
	if role.Revision >= r.Role.Revision {
		r.Role = role
	}
	r.resumeAfterPending(time.Now())
	r.PendingRetry = nil
	r.PendingBinding = nil
	r.PendingOperationID = ""
	r.PendingFingerprint = ""
	// 绑定不改HP/MP/CD/Buff/药效，也不更改客户端economic command游标。
	w.assets(r)
	// cause 保存由无符号整数“32”取得的原因。
	cause := uint32(1)
	if r.Conn == fact.conn && r.Auth == fact.auth && r.Scene == fact.scene && r.Player.Life == fact.actor.Life && r.Role.BindingSourceSeq == fact.request.SourceSeq && r.Role.Binding != nil && *r.Role.Binding == fact.center && r.Role.BindingSafeZoneID == fact.zoneID {
		cause = 2
	}
	w.bindingState(r, cause)
	w.snapshot(r)
	w.flushKillFacts(r)
	w.after(r)
}

// bindingState 公开有回执支持的已确认安全区绑定与观察原因。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，cause 记录原因，使用数值表示。
func (w *worker) bindingState(r *room, cause uint32) {
	if r.Conn == nil {
		return
	}
	// state 保存当前处理所用的状态。
	state := &pb.BindingState{Role: &pb.RoleKey{RoleId: domain.Bytes(r.Role.ID), OriginServerId: r.Role.Origin}, SceneEpoch: r.Scene, RoleRevision: uint64(r.Role.Revision), SourceSeq: uint64(r.Role.BindingSourceSeq), Cause: cause}
	if r.Role.Binding != nil {
		// point 保存会话绑定，用于当前步骤中的点位。
		point := r.Role.Binding
		state.Binding = &pb.RoleBindingPoint{MapId: point.MapID, Center: &pb.GridPoint{X: point.X, Y: point.Y}}
		state.Trusted = r.Role.BindingSourceSeq > 0 && r.Role.BindingSafeZoneID > 0
		if state.Trusted {
			state.SafeZoneId = r.Role.BindingSafeZoneID
		}
	}
	w.send(r, p.MsgS2CBindingState, 0, state)
}
