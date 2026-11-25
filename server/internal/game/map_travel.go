// 本文件实现城镇与个人地图换图计划、暂停新增操作、提交迁移和新场景确认流程。
package game

import (
	"encoding/hex"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
)

// 数值为协议固定枚举；入口、开关及全部期限来自唯一Excel配置。
const (
	// travelPrepared 表示计划已生成，正在等待客户端提交。
	travelPrepared uint32 = 0
	// travelQuiescing 表示停止新增行为，等待原世界已经受理的工作完成。
	travelQuiescing uint32 = 1
	// travelCommitted 表示已迁移到目标世界，可能仍等待新场景资源确认。
	travelCommitted uint32 = 2
	// travelCancelled 表示换图计划已明确取消。
	travelCancelled uint32 = 3
	// travelExpired 表示换图计划错过当前阶段的有效期限。
	travelExpired uint32 = 4
	// travelToTown 表示目标是配置中允许进入的城镇。
	travelToTown uint32 = 1
	// travelToPersonal 表示返回该角色保存的个人现场。
	travelToPersonal uint32 = 2
)

// 只保存一个休眠个人现场，不复制角色、钱包、宠物或永久提交所有者。
type personalWorld = worldState

// 只保存已接受ID的墓碑。满时拒新计划，不能遗忘旧取消/到期ID再让迟到确认复活。
// 命名空间由连接、真实会话代次和原scene共同限定，不因取消或超时自行重置。
type mapTravelHistory struct {
	Conn         Connection          // 已接受换图编号所属的附着连接。
	SessionEpoch int64               // 该历史范围所属的持久会话代次。
	Scene        uint64              // 该历史范围所属的原客户端场景代次。
	IDs          map[string]struct{} // 此连接与场景已经接受的换图编号集合。
}

// currentTravelHistory 按连接、会话和原场景界定换图历史，身份改变时才建立新范围。
// r 指向当前执行器拥有并更新的目标房间。
func currentTravelHistory(r *room) *mapTravelHistory {
	// history 保存地图切换“history”，用于当前步骤中的“history”。
	history := r.TravelHistory
	if history == nil || history.Conn != r.Conn || history.SessionEpoch != r.Auth.SessionEpoch || history.Scene != r.Scene {
		history = &mapTravelHistory{Conn: r.Conn, SessionEpoch: r.Auth.SessionEpoch, Scene: r.Scene, IDs: make(map[string]struct{})}
		r.TravelHistory = history
	}
	return history
}

// 最多一个当前/最近终态plan。旧ID不能借新prepare重新附着到另一笔旅行。
type mapTravelPlan struct {
	ID             string                     // 连接侧提供的唯一换图操作编号。
	Destination    uint32                     // 选择进入城镇或返回个人图的一类方向。
	RequestedMapID uint64                     // 请求指定的配置地图编号。
	Conn           Connection                 // 准备阶段接受换图的原连接。
	Auth           persistence.Auth           // 准备阶段核验的持久角色授权。
	FromScene      uint64                     // 发起换图的原客户端场景代次。
	FromMapID      uint64                     // 发起换图的原地图编号。
	PlayerLife     uint64                     // 发起换图时的角色生命代次。
	TargetMapID    uint64                     // 实际计划前往的地图编号。
	TargetKind     uint32                     // 目标地图的配置场景类型。
	Position       devconfig.Point            // 准备阶段冻结的目标逻辑坐标。
	Personal       *personalWorld             // 该计划需要保存或恢复的个人世界。
	Rules          devconfig.TownTravelConfig // 接受该计划时冻结的换图规则快照。
	ConfigHash     string                     // 该计划对应的玩法配置摘要。
	RuleVersion    uint64                     // 该计划对应的配置发布版本。
	Expires        time.Time                  // 当前换图计划等待确认的截止时刻。
	QuiesceUntil   time.Time                  // 等待原世界收束阶段的独立截止时刻。
	AckUntil       time.Time                  // 切换后等待新场景资源准备确认的截止时刻。
	Phase          uint32                     // 计划当前所处的准备、停更、提交、撤销或过期状态。
	Request        uint32                     // 与计划阶段回复关联的客户端请求编号。
	TargetScene    uint64                     // 提交时分配给新场景的客户端观察身份。
	TargetBaseline uint64                     // 提交时新场景要求客户端确认的基线编号。
	Acked          bool                       // 新场景的匹配资源 ACK 已接受时为真。
	AckExpired     bool                       // 新场景等待资源 ACK 已超时时为真。
}

// mapTravelQuiescing 报告当前计划是否正在等待原场景收束。
// r 指向当前执行器拥有并更新的目标房间。
func mapTravelQuiescing(r *room) bool {
	return r != nil && r.Travel != nil && r.Travel.Phase == travelQuiescing
}

// validTravelID 检查换图编号是否符合项目的十六进制随机编号格式。
// id 区分当前步骤指向的目标对象。
func validTravelID(id string) bool {
	// 沿用domain.ID的16随机字节标识格式，不另设游戏数值预算。
	if len(id) != 32 || strings.ToLower(id) != id {
		return false
	}
	// b 保存由解码文本取得的b，err 接收解码文本过程中可能产生的错误，供后续失败分支处理。
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16
}

// travelAttached 验证当前帧与房间、连接和会话代次是否一致。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，frame 记录协议帧。
func (w *worker) travelAttached(r *room, frame wire.Frame) bool {
	return r != nil && !r.Isolated && !w.e.stopped.Load() && r.Conn != nil && r.Conn.Alive() &&
		r.Auth.RoleID != "" && r.Role.ID == r.Auth.RoleID && w.rooms[r.Auth.RoleID] == r &&
		r.Conn.Binding() == uint64(r.Auth.SessionEpoch) && frame.Binding == uint64(r.Auth.SessionEpoch)
}

// travelBound 复核已接受计划是否仍属于原房间和授权。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，plan 记录计划，使用地图地图切换计划表示。
func (w *worker) travelBound(r *room, plan *mapTravelPlan) bool {
	return r.Conn == plan.Conn && r.Auth == plan.Auth && r.Role.ID == plan.Auth.RoleID && r.Conn != nil && r.Conn.Alive() &&
		r.Conn.Binding() == uint64(plan.Auth.SessionEpoch) && w.rooms[plan.Auth.RoleID] == r
}

// travelConfigMatches 比较当前生效的配置、规则开关及各项期限是否仍与计划一致。
// w 是调用该方法的房间执行器实例，plan 记录计划，使用地图地图切换计划表示。
func (w *worker) travelConfigMatches(plan *mapTravelPlan) bool {
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	return f.TownTravel != nil && f.TownTravel.Enabled && f.TownTravel.Matches(plan.Rules) &&
		f.Hash == plan.ConfigHash && f.RuleVersion == plan.RuleVersion
}

// validTravelRules 确认换图规则开启且全部时限可安全转换为时长。
// rules 记录规则集合，使用城镇地图切换配置表示。
func validTravelRules(rules *devconfig.TownTravelConfig) bool {
	if rules == nil || !rules.Enabled || rules.TownMapID == 0 || rules.MaxPlansPerScene <= 0 {
		return false
	}
	// ms 接收遍历数值列表时当前的元素。
	for _, ms := range []int64{rules.PlanTTLMS, rules.QuiesceTTLMS, rules.AckTTLMS} {
		if ms <= 0 || ms > math.MaxInt64/int64(time.Millisecond) {
			return false
		}
	}
	return true
}

// travelFailure 对仍可见的连接返回原请求失败。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，frame 记录协议帧，code 记录结果码，使用数值表示。
func (w *worker) travelFailure(r *room, frame wire.Frame, code uint32) {
	if r != nil && r.Conn != nil {
		w.failure(r, frame, code)
	}
}

// sendTravelState 按当前计划阶段返回正确的场景归属及终态。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，plan 记录计划，使用地图地图切换计划表示，request 关联客户端请求与服务端的对应响应。
func (w *worker) sendTravelState(r *room, plan *mapTravelPlan, request uint32) {
	if !w.travelBound(r, plan) {
		return
	}
	// scene 保存来源场景，用于当前步骤中的场景。
	scene := plan.FromScene
	if plan.Phase == travelCommitted {
		scene = plan.TargetScene
	}
	w.send(r, p.MsgS2CMapTravelState, request, &pb.MapTravelState{TravelId: plan.ID,
		FromSceneEpoch: plan.FromScene, SceneEpoch: scene, Phase: plan.Phase, TargetMapId: plan.TargetMapID})
}

// sendTravelPlan 发送目标地图、落点、原场景和确认截止时间。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，plan 记录计划，使用地图地图切换计划表示，request 关联客户端请求与服务端的对应响应。
func (w *worker) sendTravelPlan(r *room, plan *mapTravelPlan, request uint32) {
	w.send(r, p.MsgS2CMapTravelPlan, request, &pb.MapTravelPlan{TravelId: plan.ID, FromSceneEpoch: plan.FromScene,
		TargetMapId: plan.TargetMapID, TargetSceneKind: plan.TargetKind, Position: pointPB(plan.Position), ExpiresAtUnixMs: plan.Expires.UnixMilli()})
}

// prepareMapTravel 使用服务端实时发起换图准备。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，frame 记录协议帧。
func (w *worker) prepareMapTravel(r *room, frame wire.Frame) {
	w.prepareMapTravelAt(r, frame, time.Now())
}

// prepareMapTravelAt 验证目标场景、授权和配置，创建有期限的计划及原身份快照。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，frame 记录协议帧，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) prepareMapTravelAt(r *room, frame wire.Frame, now time.Time) {
	// q 记录地图地图切换准备。
	var q pb.MapTravelPrepare
	if proto.Unmarshal(frame.Payload, &q) != nil || !validTravelID(q.TravelId) || q.SceneEpoch == 0 ||
		(q.Destination != travelToTown && q.Destination != travelToPersonal) {
		w.travelFailure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.travelAttached(r, frame) {
		w.travelFailure(r, frame, p.CodeStaleSession)
		return
	}
	// 新准备请求也推进旧plan的期限检查，不能靠持续发新ID让过期槽一直占用。
	w.progressMapTravel(r, now)
	// old 保存地图切换，用于当前步骤中的原状态。
	if old := r.Travel; old != nil && old.ID == q.TravelId && old.FromScene == q.SceneEpoch && old.Conn == r.Conn && old.Auth == r.Auth {
		if old.Destination != q.Destination || old.RequestedMapID != q.TargetMapId || !w.travelBound(r, old) {
			w.travelFailure(r, frame, p.CodeOperationConflict)
			return
		}
		w.progressMapTravel(r, now)
		if old.Phase == travelPrepared {
			w.sendTravelPlan(r, old, frame.Request)
		} else {
			w.sendTravelState(r, old, frame.Request)
		}
		return
	}
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	if !r.Ready || q.SceneEpoch != r.Scene || r.Scene == math.MaxUint64 || r.Player.HP <= 0 || !validTravelRules(f.TownTravel) {
		w.travelFailure(r, frame, p.CodeLineForbidden)
		return
	}
	// history 保存由当前状态地图切换“history”取得的“history”。
	history := currentTravelHistory(r)
	// seen 表示当前附着范围已接受过这个换图编号，不可重新创建计划。
	if _, seen := history.IDs[q.TravelId]; seen {
		w.travelFailure(r, frame, p.CodeOperationConflict)
		return
	}
	// old 保存地图切换，用于当前步骤中的原状态。
	if old := r.Travel; old != nil && (old.Phase == travelPrepared || old.Phase == travelQuiescing || old.Phase == travelCommitted && !old.Acked) {
		w.travelFailure(r, frame, p.CodeOperationPending)
		return
	}
	if len(history.IDs) >= f.TownTravel.MaxPlansPerScene {
		w.travelFailure(r, frame, p.CodeServerBusy)
		return
	}
	// current 保存由当前状态地图取得的当前状态，ok 表明查询、校验或类型转换是否成功。
	current, ok := w.currentMap(r)
	if !ok || r.MapID == 0 {
		w.travelFailure(r, frame, p.CodeLineForbidden)
		return
	}
	// target 记录目标，使用地图模板表示。
	var target *devconfig.MapTemplate
	// position 记录位置，使用点位表示。
	var position devconfig.Point
	if q.Destination == travelToTown {
		// mapID 标识当前业务使用的地图。
		mapID := q.TargetMapId
		if f.RuleVersion < 24 && mapID == 0 {
			mapID = f.TownTravel.TownMapID
		}
		// fromPersonal 表示源地图是角色的个人图，而且尚未保留另一份个人现场。
		fromPersonal := current.SceneKindValue() == devconfig.SceneKindPersonal && r.SavedPersonal == nil &&
			(r.MapID == f.Map.ID || f.OrdinaryMonsterRosterEnabled() && r.currentWorld() == r.PersonalWorld)
		// fromTown 表示当前规则允许已验证城镇间互转，原个人世界仍匹配已保存的返回现场。
		fromTown := f.RuleVersion >= 24 && current.SceneKindValue() == devconfig.SceneKindTown && f.TownDestination(r.MapID) &&
			r.SavedPersonal != nil && r.SavedPersonal == r.PersonalWorld && (r.SavedPersonal.MapID == f.Map.ID || f.OrdinaryMonsterRosterEnabled())
		if (!fromPersonal && !fromTown) || !f.TownDestination(mapID) || mapID == r.MapID {
			w.travelFailure(r, frame, p.CodeLineForbidden)
			return
		}
		target, ok = f.MapByID(mapID)
		if !ok || target.SceneKindValue() != devconfig.SceneKindTown {
			w.travelFailure(r, frame, p.CodeLineForbidden)
			return
		}
		position = target.PlayerSpawn
	} else if f.OrdinaryMonsterRosterEnabled() {
		target, position, ok = w.selectOrdinaryDestination(r, current, q.TargetMapId)
		if !ok {
			w.travelFailure(r, frame, p.CodeLineForbidden)
			return
		}
	} else {
		if current.SceneKindValue() != devconfig.SceneKindTown || !f.TownDestination(r.MapID) || r.SavedPersonal == nil ||
			q.TargetMapId != 0 && q.TargetMapId != r.SavedPersonal.MapID {
			w.travelFailure(r, frame, p.CodeLineForbidden)
			return
		}
		target, ok = f.MapByID(r.SavedPersonal.MapID)
		if !ok || target.SceneKindValue() != devconfig.SceneKindPersonal || target.ID != f.Map.ID {
			w.travelFailure(r, frame, p.CodeLineForbidden)
			return
		}
		position = r.SavedPersonal.PlayerPosition
	}
	if !target.Walkable(position) {
		w.travelFailure(r, frame, p.CodeLineForbidden)
		return
	}
	history.IDs[q.TravelId] = struct{}{}
	r.Travel = &mapTravelPlan{ID: q.TravelId, Destination: q.Destination, RequestedMapID: q.TargetMapId, Conn: r.Conn, Auth: r.Auth,
		FromScene: r.Scene, FromMapID: r.MapID, PlayerLife: r.Player.Life, TargetMapID: target.ID, TargetKind: target.SceneKindValue(),
		Position: position, Personal: r.SavedPersonal, Rules: f.TownTravel.Snapshot(), ConfigHash: f.Hash, RuleVersion: f.RuleVersion,
		Expires: now.Add(time.Duration(f.TownTravel.PlanTTLMS) * time.Millisecond)}
	w.sendTravelPlan(r, r.Travel, frame.Request)
}

// commitMapTravel 使用服务端实时处理客户端提交或撤销。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，frame 记录协议帧。
func (w *worker) commitMapTravel(r *room, frame wire.Frame) {
	w.commitMapTravelAt(r, frame, time.Now())
}

// commitMapTravelAt 根据原编号和原连接解释提交，进入等待原世界收束的阶段。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，frame 记录协议帧，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) commitMapTravelAt(r *room, frame wire.Frame, now time.Time) {
	// q 记录地图地图切换提交。
	var q pb.MapTravelCommit
	if proto.Unmarshal(frame.Payload, &q) != nil || !validTravelID(q.TravelId) || q.SceneEpoch == 0 {
		w.travelFailure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.travelAttached(r, frame) {
		w.travelFailure(r, frame, p.CodeStaleSession)
		return
	}
	// plan 保存地图切换，用于当前步骤中的计划。
	plan := r.Travel
	if plan == nil || plan.ID != q.TravelId || plan.FromScene != q.SceneEpoch || !w.travelBound(r, plan) {
		w.travelFailure(r, frame, p.CodeOperationConflict)
		return
	}
	if plan.Phase >= travelCommitted {
		w.sendTravelState(r, plan, frame.Request)
		return
	}
	if q.Cancel {
		// deadline 保存过期集合，用于当前步骤中的截止时间。
		deadline := plan.Expires
		if plan.Phase == travelQuiescing {
			deadline = plan.QuiesceUntil
		}
		plan.Phase = travelCancelled
		if !now.Before(deadline) {
			plan.Phase = travelExpired
		}
		w.sendTravelState(r, plan, frame.Request)
		return
	}
	w.progressMapTravel(r, now)
	if plan.Phase >= travelCommitted {
		w.sendTravelState(r, plan, frame.Request)
		return
	}
	if plan.Phase == travelPrepared {
		plan.Phase, plan.Request = travelQuiescing, frame.Request
		plan.QuiesceUntil = now.Add(time.Duration(plan.Rules.QuiesceTTLMS) * time.Millisecond)
		w.sendTravelState(r, plan, frame.Request)
	} else {
		w.sendTravelState(r, plan, frame.Request)
	}
	// 这里只观察既有队列，不提前结算、丢弃或重写任何旧scene事件。
	w.progressMapTravel(r, now)
}

// true只表示本帧必须停新AI（包括刚完成移交，避免使用step顶部的旧地图模板）。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) progressMapTravel(r *room, now time.Time) bool {
	if r == nil || r.Travel == nil {
		return false
	}
	// plan 保存地图切换，用于当前步骤中的计划。
	plan := r.Travel
	if plan.Phase == travelCommitted {
		if !plan.Acked && !plan.AckExpired && !now.Before(plan.AckUntil) {
			plan.AckExpired = true
			if w.travelBound(r, plan) {
				r.Conn.Close("travel acknowledgement timeout")
			}
		}
		return false
	}
	if plan.Phase == travelCancelled || plan.Phase == travelExpired {
		return false
	}
	if r.Isolated || w.e.stopped.Load() || !r.Ready || !w.travelBound(r, plan) || r.Scene != plan.FromScene || r.MapID != plan.FromMapID ||
		r.Player.Life != plan.PlayerLife || r.Player.HP <= 0 || !w.travelConfigMatches(plan) {
		plan.Phase = travelCancelled
		w.sendTravelState(r, plan, plan.Request)
		return false
	}
	// deadline 保存过期集合，用于当前步骤中的截止时间。
	deadline := plan.Expires
	if plan.Phase == travelQuiescing {
		deadline = plan.QuiesceUntil
	}
	if !now.Before(deadline) {
		plan.Phase = travelExpired
		w.sendTravelState(r, plan, plan.Request)
		return false
	}
	if plan.Phase != travelQuiescing {
		return false
	}
	if r.Pending || r.PendingRetry != nil || r.PendingPetBirth != nil || len(r.KillFacts) != 0 || len(r.Waiting) != 0 || r.eventCount() != 0 {
		return true
	}
	if w.flushWear(r, now, true) {
		return true
	}
	return w.finishMapTravel(r, plan, now)
}

// travelOccupied 判断当前位置是否有已激活的存活怪物占用。
// monsters 记录怪物集合，使用实体列表表示，count 记录数量，使用数值表示，point 记录点位。
func travelOccupied(monsters []*entity, count int, point devconfig.Point) bool {
	// monster 接收遍历怪物集合中的范围时当前的元素。
	for _, monster := range monsters[:count] {
		if monster != nil && monster.HP > 0 && monster.Pos == point {
			return true
		}
	}
	return false
}

// travelPetPoint 在目标地图为驻留宠物查找合法落点。
// m 记录地图模板，position 记录位置，使用点位表示，preferred 记录“preferred”，使用点位表示，monsters 记录怪物集合，使用实体列表表示。
// count 记录数量，使用数值表示。
func travelPetPoint(m *devconfig.MapTemplate, position devconfig.Point, preferred *devconfig.Point, monsters []*entity, count int) (devconfig.Point, bool) {
	// free 封装免费或空闲对应的后续处理，point 记录点位。
	free := func(point devconfig.Point) bool {
		return point != position && m.Walkable(point) && !travelOccupied(monsters, count, point)
	}
	if preferred != nil && free(*preferred) {
		return *preferred, true
	}
	// direction 接收遍历朝向集合时当前的元素。
	for _, direction := range directions {
		// point 组装点位所需的数据。
		point := devconfig.Point{X: position.X + direction.X, Y: position.Y + direction.Y}
		if direction.X != 0 && direction.Y != 0 && (!m.Walkable(devconfig.Point{X: position.X + direction.X, Y: position.Y}) || !m.Walkable(devconfig.Point{X: position.X, Y: position.Y + direction.Y})) {
			continue
		}
		if free(point) {
			return point, true
		}
	}
	return devconfig.Point{}, false
}

// nextTravelBaseline 为新场景分配比现有基线更新的观察编号。
// w 是调用该方法的房间执行器实例，current 记录当前状态，使用数值表示。
func (w *worker) nextTravelBaseline(current uint64) (uint64, bool) {
	for {
		// counter 保存由加载取得的计数。
		counter := w.e.baseline.Load()
		// base 保存当前处理所用的基础值。
		base := counter
		if current > base {
			base = current
		}
		if base == math.MaxUint64 {
			return 0, false
		}
		if w.e.baseline.CompareAndSwap(counter, base+1) {
			return base + 1, true
		}
	}
}

// finishMapTravel 提交原角色向目标世界迁移，并发送需要客户端确认的场景基线。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，plan 记录计划，使用地图地图切换计划表示，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) finishMapTravel(r *room, plan *mapTravelPlan, now time.Time) bool {
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	// target 是即将迁入的地图配置，ok 表示该模板存在。
	target, ok := f.MapByID(plan.TargetMapID)
	// movePet 表示存在主人宠物且目标地图允许宠物进入。
	movePet := r.Pet != nil && !mapForbidsPets(target)
	// monsters 记录怪物集合，使用实体列表表示。
	var monsters []*entity
	// count 规定数量的初始取值。
	count := 0
	// preferred 保存宠物可优先恢复的目标格，以目标世界的真实驻留位置优先。
	var preferred *devconfig.Point
	// residentWorld 是已经保留的目标世界；不同普通图只在提交时分配新现场。
	residentWorld := r.townWorld(plan.TargetMapID)
	newPersonal := false
	if plan.Destination == travelToPersonal {
		residentWorld = r.SavedPersonal
		if f.OrdinaryMonsterRosterEnabled() && ok && r.PersonalWorld != nil {
			if r.PersonalWorld.MapID != plan.TargetMapID {
				residentWorld, newPersonal = nil, true
				preferred = &target.PetSpawn
			} else {
				residentWorld = r.PersonalWorld
			}
		}
		if r.SavedPersonal != plan.Personal || residentWorld == nil && !newPersonal {
			ok = false
		} else if residentWorld != nil {
			monsters, count = residentWorld.Monsters, residentWorld.MonsterCount
			if r.Pet != nil && residentWorld.HasPet && residentWorld.PetLife == r.Pet.Life {
				preferred = &residentWorld.PetPosition
			}
		}
	}
	// pet 保存由场景世界宠物取得的宠物。
	if pet := r.worldPet(residentWorld); pet != nil {
		preferred = &pet.Pos // 目的World已有同一宠物时，真实现场优先于历史位置提示。
	}
	if !ok || target.SceneKindValue() != plan.TargetKind || !target.Walkable(plan.Position) || count < 0 || count > len(monsters) ||
		travelOccupied(monsters, count, plan.Position) || r.Scene == math.MaxUint64 || r.Player.MovementRevision == math.MaxUint64 ||
		movePet && r.Pet.MovementRevision == math.MaxUint64 {
		plan.Phase = travelCancelled
		w.sendTravelState(r, plan, plan.Request)
		return false
	}
	// petPosition 初始化用作宠物位置的点位。
	petPosition := devconfig.Point{}
	if movePet {
		petPosition, ok = travelPetPoint(target, plan.Position, preferred, monsters, count)
		if !ok {
			plan.Phase = travelCancelled
			w.sendTravelState(r, plan, plan.Request)
			return false
		}
	}
	// moves 是本次迁移需要分配的格进入序号数量，玩家占一个，携带宠物时再加一个。
	moves := uint64(1)
	if movePet {
		moves++
	}
	if r.CellEntrySequence > math.MaxUint64-moves {
		plan.Phase = travelCancelled
		w.sendTravelState(r, plan, plan.Request)
		return false
	}
	// baseline 保存由下一状态地图切换场景基线取得的场景基线。
	baseline, ok := w.nextTravelBaseline(r.Baseline)
	if !ok {
		plan.Phase = travelCancelled
		w.sendTravelState(r, plan, plan.Request)
		return false
	}
	// destinationWorld 记录目的地场景世界，使用场景世界状态表示。
	var destinationWorld *worldState
	if plan.Destination == travelToTown {
		destinationWorld = w.ensureTownWorld(r, plan.TargetMapID)
		if destinationWorld == nil || destinationWorld.MapID != plan.TargetMapID {
			plan.Phase = travelCancelled
			w.sendTravelState(r, plan, plan.Request)
			return false
		}
		if r.MapID == f.Map.ID && r.PersonalWorld == nil {
			r.PersonalWorld = r.currentWorld()
		}
		destinationWorld.Auto, destinationWorld.Destination, destinationWorld.Navigation = false, nil, navigationState{}
	} else if newPersonal {
		destinationWorld = r.allocateWorld(plan.TargetMapID, math.MaxUint64)
		if destinationWorld == nil || !populateOrdinaryMonsters(f, destinationWorld, target, r.PersonalWorld, now) {
			plan.Phase = travelCancelled
			w.sendTravelState(r, plan, plan.Request)
			return false
		}
		destinationWorld.Auto = r.PersonalWorld.Auto
		r.TravelPersonalWorld = destinationWorld
		defer func() { r.TravelPersonalWorld = nil }()
	} else {
		destinationWorld = residentWorld
		// 原复活/掉落截止原样恢复，由正常step按当前时钟处理一次；不回放休眠期攻击或收益。
	}
	var petLanding *devconfig.Point // 只在需要迁移宠物时保留它的落点。
	if movePet {
		petLanding = &petPosition
	}
	if !w.activateWorldAt(r, destinationWorld, plan.Position, petLanding, true) {
		plan.Phase = travelCancelled
		w.sendTravelState(r, plan, plan.Request)
		return false
	}
	if plan.Destination == travelToPersonal {
		r.adoptOrdinaryWorld(destinationWorld)
		plan.Personal = nil // 终态重放不应持有已经弃用的普通现场。
	}
	plan.Position = r.Player.Pos // 城镇出生格被占时同步服务端最终分配的落点。
	r.Scene, r.Baseline = r.Scene+1, baseline
	r.TravelHistory = nil                // 实际换scene后，旧scene请求已有身份门禁；当前终态plan仍可查询。
	r.Manual, r.InputSequence = false, 0 // 摇杆属于连接输入，不在新图自动恢复按住状态。
	r.PreviewInFlight = false
	if r.PreviewGeneration < math.MaxUint64 {
		r.PreviewGeneration++
	}
	r.Ready, r.LastTick, r.LastBroadcast = false, now, now
	r.PetBornReady = nil
	plan.Phase, plan.TargetScene, plan.TargetBaseline = travelCommitted, r.Scene, r.Baseline
	plan.AckUntil = now.Add(time.Duration(plan.Rules.AckTTLMS) * time.Millisecond)
	r.AwaitUntil = plan.AckUntil
	w.sendTravelState(r, plan, plan.Request)
	// entry 保存新场景的编号、基线、版本和角色位置，供客户端装载。
	if entry := w.enterLineResult(r); entry != nil {
		w.send(r, p.MsgS2CEnterLine, 0, entry)
		w.bindingState(r, 1)
		w.baseline(r)
	}
	return true
}

// 根ACK已经匹配当前scene/baseline后，在Ready置true之前调用。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准。
func (w *worker) finishMapTravelAck(r *room, now time.Time) bool {
	if r.Travel == nil || r.Travel.Phase != travelCommitted || r.Travel.Acked {
		return true
	}
	// plan 保存地图切换，用于当前步骤中的计划。
	plan := r.Travel
	if !w.travelBound(r, plan) || plan.TargetScene != r.Scene || plan.TargetBaseline != r.Baseline || r.Player.Life != plan.PlayerLife || plan.AckExpired || !now.Before(plan.AckUntil) {
		if w.travelBound(r, plan) && !now.Before(plan.AckUntil) {
			plan.AckExpired = true
			r.Conn.Close("travel acknowledgement timeout")
		}
		return false
	}
	plan.Acked = true
	return true
}

// 断线或接管只能取消未提交plan；已提交world保留，下一次健康Enter重新附着它。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间。
func (w *worker) cancelMapTravelOnDetach(r *room) {
	if r == nil || r.Travel == nil {
		return
	}
	if r.Travel.Phase == travelPrepared || r.Travel.Phase == travelQuiescing {
		r.Travel.Phase = travelCancelled
	} else if r.Travel.Phase == travelCommitted {
		r.Travel.Acked = true
	}
}
