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
	request             persistence.SafeZoneBindingRequest
	auth                persistence.Auth
	room                *room
	conn                Connection
	actor               actorRef
	movement, cellEntry uint64
	center              domain.RoleBinding
	zoneID              uint64
	fingerprint         string
}

// 只为本次合法walk预备候选。来源HumanObject.Walk:2480在成功落格后绑定，不能普挂relocate。
func (w *worker) prepareSafeZoneWalk(r *room, e *entity, point devconfig.Point) (*safeZoneBindingFact, bool) {
	f := &w.e.Store.Cfg.Fixture
	if !f.SafeZoneBindingEnabled() || e != &r.Player {
		return nil, true
	}
	m, known := w.currentMap(r)
	if !known {
		return nil, false
	}
	zone, inside := m.SafeZoneAt(point)
	if !inside {
		return nil, true
	}
	if !m.Walkable(point) || zone.ID == 0 || zone.Center.X < 0 || zone.Center.Y < 0 || zone.Center.X > math.MaxInt32 || zone.Center.Y > math.MaxInt32 || !m.Walkable(zone.Center) {
		return nil, false
	}
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
	q := persistence.SafeZoneBindingRequest{SourceSeq: r.Role.SourceSeq + 1, ExpectedRevision: r.Role.Revision, ExpectedRuleVersion: f.RuleVersion, ExpectedConfigHash: f.Hash, MapID: m.ID, Position: point, Action: "walk", EventID: domain.ID()}
	raw, err := json.Marshal(q)
	if err != nil {
		return nil, false
	}
	return &safeZoneBindingFact{request: q, auth: r.Auth, room: r, conn: r.Conn, center: center, zoneID: zone.ID, fingerprint: persistence.Hash(raw)}, true
}

func (w *worker) submitSafeZoneBinding(r *room, fact *safeZoneBindingFact, now time.Time) {
	// 调用点已完成本次walk和独立时钟。此后既有Pending屏障停止本tick的宠物/怪物新动作。
	fact.actor = reference(r, &r.Player)
	fact.movement, fact.cellEntry = r.Player.MovementRevision, r.Player.CellEntry
	r.PendingBinding = fact
	r.Pending = true
	r.PendingOperationID = fact.request.EventID
	r.PendingFingerprint = fact.fingerprint
	var pending job
	pending = job{target: w, run: func() func(*worker) {
		role, receipt, err := w.e.Store.BindSafeZoneWithReceipt(w.e.ctx, fact.auth, fact.request)
		return func(w *worker) { w.finishSafeZoneBinding(fact, role, receipt, err, &pending) }
	}}
	if !w.submit(pending) {
		r.PendingRetry = &pending
		r.RetryAt = now.Add(100 * time.Millisecond) // 复用现有永久job入队失败重试节拍。
	}
}

func bindingReceiptMatches(fact *safeZoneBindingFact, role domain.Role, receipt persistence.Operation) bool {
	b := receipt.Binding
	return role.ID == fact.auth.RoleID && role.Revision > fact.request.ExpectedRevision && role.SourceSeq >= fact.request.SourceSeq && role.BindingSourceSeq >= fact.request.SourceSeq && role.Binding != nil && role.BindingSafeZoneID > 0 &&
		receipt.ID == fmt.Sprintf("binding:%s:%d", fact.auth.RoleID, fact.request.SourceSeq) && receipt.RoleID == fact.auth.RoleID && receipt.Kind == "safe_zone_binding" && receipt.Code == 0 && receipt.SourceSeq == fact.request.SourceSeq && receipt.Digest == fact.fingerprint && b != nil && b.EventID == fact.request.EventID && b.Action == "walk" && b.Point == fact.center && b.ZoneID == fact.zoneID && b.RuleVersion == fact.request.ExpectedRuleVersion && b.ConfigHash == fact.request.ExpectedConfigHash
}

func (w *worker) finishSafeZoneBinding(fact *safeZoneBindingFact, role domain.Role, receipt persistence.Operation, err error, retry *job) {
	r := fact.room
	current := w.rooms[fact.auth.RoleID]
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
	r.Pending = false
	r.PendingRetry = nil
	r.PendingBinding = nil
	r.PendingOperationID = ""
	r.PendingFingerprint = ""
	// 绑定不改HP/MP/CD/Buff/药效，也不更改客户端economic command游标。
	w.assets(r)
	cause := uint32(1)
	if r.Conn == fact.conn && r.Auth == fact.auth && r.Scene == fact.actor.Scene && r.Player.Life == fact.actor.Life && r.Role.BindingSourceSeq == fact.request.SourceSeq && r.Role.Binding != nil && *r.Role.Binding == fact.center && r.Role.BindingSafeZoneID == fact.zoneID {
		cause = 2
	}
	w.bindingState(r, cause)
	w.snapshot(r)
	w.flushKillFacts(r)
	w.after(r)
}

func (w *worker) bindingState(r *room, cause uint32) {
	if r.Conn == nil {
		return
	}
	state := &pb.BindingState{Role: &pb.RoleKey{RoleId: domain.Bytes(r.Role.ID), OriginServerId: r.Role.Origin}, SceneEpoch: r.Scene, RoleRevision: uint64(r.Role.Revision), SourceSeq: uint64(r.Role.BindingSourceSeq), Cause: cause}
	if r.Role.Binding != nil {
		point := r.Role.Binding
		state.Binding = &pb.RoleBindingPoint{MapId: point.MapID, Center: &pb.GridPoint{X: point.X, Y: point.Y}}
		state.Trusted = r.Role.BindingSourceSeq > 0 && r.Role.BindingSafeZoneID > 0
		if state.Trusted {
			state.SafeZoneId = r.Role.BindingSafeZoneID
		}
	}
	w.send(r, p.MsgS2CBindingState, 0, state)
}
