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
	travelPrepared   uint32 = 0
	travelQuiescing  uint32 = 1
	travelCommitted  uint32 = 2
	travelCancelled  uint32 = 3
	travelExpired    uint32 = 4
	travelToTown     uint32 = 1
	travelToPersonal uint32 = 2
)

// 只保存一个休眠个人现场，不复制角色、钱包、宠物或永久提交所有者。
type personalWorld = worldState

// 只保存已接受ID的墓碑。满时拒新计划，不能遗忘旧取消/到期ID再让迟到确认复活。
// 命名空间由连接、真实会话代次和原scene共同限定，不因取消或超时自行重置。
type mapTravelHistory struct {
	Conn         Connection
	SessionEpoch int64
	Scene        uint64
	IDs          map[string]struct{}
}

func currentTravelHistory(r *room) *mapTravelHistory {
	history := r.TravelHistory
	if history == nil || history.Conn != r.Conn || history.SessionEpoch != r.Auth.SessionEpoch || history.Scene != r.Scene {
		history = &mapTravelHistory{Conn: r.Conn, SessionEpoch: r.Auth.SessionEpoch, Scene: r.Scene, IDs: make(map[string]struct{})}
		r.TravelHistory = history
	}
	return history
}

// 最多一个当前/最近终态plan。旧ID不能借新prepare重新附着到另一笔旅行。
type mapTravelPlan struct {
	ID             string
	Destination    uint32
	Conn           Connection
	Auth           persistence.Auth
	FromScene      uint64
	FromMapID      uint64
	PlayerLife     uint64
	TargetMapID    uint64
	TargetKind     uint32
	Position       devconfig.Point
	Personal       *personalWorld
	Rules          devconfig.TownTravelConfig
	ConfigHash     string
	RuleVersion    uint64
	Expires        time.Time
	QuiesceUntil   time.Time
	AckUntil       time.Time
	Phase          uint32
	Request        uint32
	TargetScene    uint64
	TargetBaseline uint64
	Acked          bool
	AckExpired     bool
}

func mapTravelQuiescing(r *room) bool {
	return r != nil && r.Travel != nil && r.Travel.Phase == travelQuiescing
}

func validTravelID(id string) bool {
	// 沿用domain.ID的16随机字节标识格式，不另设游戏数值预算。
	if len(id) != 32 || strings.ToLower(id) != id {
		return false
	}
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16
}

func (w *worker) travelAttached(r *room, frame wire.Frame) bool {
	return r != nil && !r.Isolated && !w.e.stopped.Load() && r.Conn != nil && r.Conn.Alive() &&
		r.Auth.RoleID != "" && r.Role.ID == r.Auth.RoleID && w.rooms[r.Auth.RoleID] == r &&
		r.Conn.Binding() == uint64(r.Auth.SessionEpoch) && frame.Binding == uint64(r.Auth.SessionEpoch)
}

func (w *worker) travelBound(r *room, plan *mapTravelPlan) bool {
	return r.Conn == plan.Conn && r.Auth == plan.Auth && r.Role.ID == plan.Auth.RoleID && r.Conn != nil && r.Conn.Alive() &&
		r.Conn.Binding() == uint64(plan.Auth.SessionEpoch) && w.rooms[plan.Auth.RoleID] == r
}

func (w *worker) travelConfigMatches(plan *mapTravelPlan) bool {
	f := &w.e.Store.Cfg.Fixture
	return f.TownTravel != nil && f.TownTravel.Enabled && *f.TownTravel == plan.Rules &&
		f.Hash == plan.ConfigHash && f.RuleVersion == plan.RuleVersion
}

func validTravelRules(rules *devconfig.TownTravelConfig) bool {
	if rules == nil || !rules.Enabled || rules.TownMapID == 0 || rules.MaxPlansPerScene <= 0 {
		return false
	}
	for _, ms := range []int64{rules.PlanTTLMS, rules.QuiesceTTLMS, rules.AckTTLMS} {
		if ms <= 0 || ms > math.MaxInt64/int64(time.Millisecond) {
			return false
		}
	}
	return true
}

func (w *worker) travelFailure(r *room, frame wire.Frame, code uint32) {
	if r != nil && r.Conn != nil {
		w.failure(r, frame, code)
	}
}

func (w *worker) sendTravelState(r *room, plan *mapTravelPlan, request uint32) {
	if !w.travelBound(r, plan) {
		return
	}
	scene := plan.FromScene
	if plan.Phase == travelCommitted {
		scene = plan.TargetScene
	}
	w.send(r, p.MsgS2CMapTravelState, request, &pb.MapTravelState{TravelId: plan.ID,
		FromSceneEpoch: plan.FromScene, SceneEpoch: scene, Phase: plan.Phase, TargetMapId: plan.TargetMapID})
}

func (w *worker) sendTravelPlan(r *room, plan *mapTravelPlan, request uint32) {
	w.send(r, p.MsgS2CMapTravelPlan, request, &pb.MapTravelPlan{TravelId: plan.ID, FromSceneEpoch: plan.FromScene,
		TargetMapId: plan.TargetMapID, TargetSceneKind: plan.TargetKind, Position: pointPB(plan.Position), ExpiresAtUnixMs: plan.Expires.UnixMilli()})
}

func (w *worker) prepareMapTravel(r *room, frame wire.Frame) {
	w.prepareMapTravelAt(r, frame, time.Now())
}

func (w *worker) prepareMapTravelAt(r *room, frame wire.Frame, now time.Time) {
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
	if old := r.Travel; old != nil && old.ID == q.TravelId && old.FromScene == q.SceneEpoch && old.Conn == r.Conn && old.Auth == r.Auth {
		if old.Destination != q.Destination || !w.travelBound(r, old) {
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
	f := &w.e.Store.Cfg.Fixture
	if !r.Ready || q.SceneEpoch != r.Scene || r.Scene == math.MaxUint64 || r.Player.HP <= 0 || !validTravelRules(f.TownTravel) {
		w.travelFailure(r, frame, p.CodeLineForbidden)
		return
	}
	history := currentTravelHistory(r)
	if _, seen := history.IDs[q.TravelId]; seen {
		w.travelFailure(r, frame, p.CodeOperationConflict)
		return
	}
	if old := r.Travel; old != nil && (old.Phase == travelPrepared || old.Phase == travelQuiescing || old.Phase == travelCommitted && !old.Acked) {
		w.travelFailure(r, frame, p.CodeOperationPending)
		return
	}
	if len(history.IDs) >= f.TownTravel.MaxPlansPerScene {
		w.travelFailure(r, frame, p.CodeServerBusy)
		return
	}
	current, ok := w.currentMap(r)
	if !ok || r.MapID == 0 {
		w.travelFailure(r, frame, p.CodeLineForbidden)
		return
	}
	var target *devconfig.MapTemplate
	var position devconfig.Point
	if q.Destination == travelToTown {
		if current.SceneKindValue() != devconfig.SceneKindPersonal || r.MapID != f.Map.ID || r.SavedPersonal != nil {
			w.travelFailure(r, frame, p.CodeLineForbidden)
			return
		}
		target, ok = f.MapByID(f.TownTravel.TownMapID)
		if !ok || target.SceneKindValue() != devconfig.SceneKindTown {
			w.travelFailure(r, frame, p.CodeLineForbidden)
			return
		}
		position = target.PlayerSpawn
	} else {
		if current.SceneKindValue() != devconfig.SceneKindTown || r.MapID != f.TownTravel.TownMapID || r.SavedPersonal == nil {
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
	r.Travel = &mapTravelPlan{ID: q.TravelId, Destination: q.Destination, Conn: r.Conn, Auth: r.Auth,
		FromScene: r.Scene, FromMapID: r.MapID, PlayerLife: r.Player.Life, TargetMapID: target.ID, TargetKind: target.SceneKindValue(),
		Position: position, Personal: r.SavedPersonal, Rules: *f.TownTravel, ConfigHash: f.Hash, RuleVersion: f.RuleVersion,
		Expires: now.Add(time.Duration(f.TownTravel.PlanTTLMS) * time.Millisecond)}
	w.sendTravelPlan(r, r.Travel, frame.Request)
}

func (w *worker) commitMapTravel(r *room, frame wire.Frame) {
	w.commitMapTravelAt(r, frame, time.Now())
}

func (w *worker) commitMapTravelAt(r *room, frame wire.Frame, now time.Time) {
	var q pb.MapTravelCommit
	if proto.Unmarshal(frame.Payload, &q) != nil || !validTravelID(q.TravelId) || q.SceneEpoch == 0 {
		w.travelFailure(r, frame, p.CodeInvalidArgument)
		return
	}
	if !w.travelAttached(r, frame) {
		w.travelFailure(r, frame, p.CodeStaleSession)
		return
	}
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
func (w *worker) progressMapTravel(r *room, now time.Time) bool {
	if r == nil || r.Travel == nil {
		return false
	}
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
	if r.Pending || r.PendingRetry != nil || len(r.KillFacts) != 0 || len(r.Waiting) != 0 || r.eventCount() != 0 {
		return true
	}
	return w.finishMapTravel(r, plan, now)
}

func travelOccupied(monsters []*entity, count int, point devconfig.Point) bool {
	for _, monster := range monsters[:count] {
		if monster != nil && monster.HP > 0 && monster.Pos == point {
			return true
		}
	}
	return false
}

func travelPetPoint(m *devconfig.MapTemplate, position devconfig.Point, preferred *devconfig.Point, monsters []*entity, count int) (devconfig.Point, bool) {
	free := func(point devconfig.Point) bool {
		return point != position && m.Walkable(point) && !travelOccupied(monsters, count, point)
	}
	if preferred != nil && free(*preferred) {
		return *preferred, true
	}
	for _, direction := range directions {
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

func (w *worker) nextTravelBaseline(current uint64) (uint64, bool) {
	for {
		counter := w.e.baseline.Load()
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

func (w *worker) finishMapTravel(r *room, plan *mapTravelPlan, now time.Time) bool {
	f := &w.e.Store.Cfg.Fixture
	target, ok := f.MapByID(plan.TargetMapID)
	var monsters []*entity
	count := 0
	var preferred *devconfig.Point
	if plan.Destination == travelToPersonal {
		if r.SavedPersonal == nil || r.SavedPersonal != plan.Personal {
			ok = false
		} else {
			monsters, count = r.SavedPersonal.Monsters, r.SavedPersonal.MonsterCount
			if r.SavedPersonal.HasPet {
				preferred = &r.SavedPersonal.PetPosition
			}
		}
	}
	if !ok || target.SceneKindValue() != plan.TargetKind || !target.Walkable(plan.Position) || count < 0 || count > len(monsters) ||
		travelOccupied(monsters, count, plan.Position) || r.Scene == math.MaxUint64 || r.Player.MovementRevision == math.MaxUint64 ||
		r.Pet != nil && r.Pet.MovementRevision == math.MaxUint64 {
		plan.Phase = travelCancelled
		w.sendTravelState(r, plan, plan.Request)
		return false
	}
	petPosition := devconfig.Point{}
	if r.Pet != nil {
		petPosition, ok = travelPetPoint(target, plan.Position, preferred, monsters, count)
		if !ok {
			plan.Phase = travelCancelled
			w.sendTravelState(r, plan, plan.Request)
			return false
		}
	}
	moves := uint64(1)
	if r.Pet != nil {
		moves++
	}
	if r.CellEntrySequence > math.MaxUint64-moves {
		plan.Phase = travelCancelled
		w.sendTravelState(r, plan, plan.Request)
		return false
	}
	baseline, ok := w.nextTravelBaseline(r.Baseline)
	if !ok {
		plan.Phase = travelCancelled
		w.sendTravelState(r, plan, plan.Request)
		return false
	}
	var destinationWorld *worldState
	if plan.Destination == travelToTown {
		r.PersonalWorld, r.SavedPersonal = r.currentWorld(), r.currentWorld()
		if r.TownWorld == nil {
			r.TownWorld = r.newWorld(plan.TargetMapID)
		}
		destinationWorld = r.TownWorld
		if destinationWorld == nil || destinationWorld.MapID != plan.TargetMapID {
			plan.Phase = travelCancelled
			w.sendTravelState(r, plan, plan.Request)
			return false
		}
		destinationWorld.Auto, destinationWorld.Destination, destinationWorld.Navigation = false, nil, navigationState{}
	} else {
		destinationWorld = r.SavedPersonal
		// 原复活/掉落截止原样恢复，由正常step按当前时钟处理一次；不回放休眠期攻击或收益。
	}
	if !w.activateWorld(r, destinationWorld, plan.Position, &petPosition) {
		plan.Phase = travelCancelled
		w.sendTravelState(r, plan, plan.Request)
		return false
	}
	if plan.Destination == travelToPersonal {
		r.SavedPersonal = nil
	}
	r.Scene, r.Baseline = r.Scene+1, baseline
	r.TravelHistory = nil                // 实际换scene后，旧scene请求已有身份门禁；当前终态plan仍可查询。
	r.Manual, r.InputSequence = false, 0 // 摇杆属于连接输入，不在新图自动恢复按住状态。
	r.PreviewInFlight = false
	if r.PreviewGeneration < math.MaxUint64 {
		r.PreviewGeneration++
	}
	r.Ready, r.LastTick, r.LastBroadcast = false, now, now
	plan.Phase, plan.TargetScene, plan.TargetBaseline = travelCommitted, r.Scene, r.Baseline
	plan.AckUntil = now.Add(time.Duration(plan.Rules.AckTTLMS) * time.Millisecond)
	r.AwaitUntil = plan.AckUntil
	w.sendTravelState(r, plan, plan.Request)
	if entry := w.enterLineResult(r); entry != nil {
		w.send(r, p.MsgS2CEnterLine, 0, entry)
		w.bindingState(r, 1)
		w.baseline(r)
	}
	return true
}

// 根ACK已经匹配当前scene/baseline后，在Ready置true之前调用。
func (w *worker) finishMapTravelAck(r *room, now time.Time) bool {
	if r.Travel == nil || r.Travel.Phase != travelCommitted || r.Travel.Acked {
		return true
	}
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
