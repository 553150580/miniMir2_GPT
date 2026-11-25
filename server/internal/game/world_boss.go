// 本文件将世界首领的查询、入场、操作和返回路由到同运行服唯一的场次分片。
package game

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const (
	worldBossClosed   uint32 = iota + 1 // 下一场的开放时间已确定，当前不可进入。
	worldBossEntering                   // 席位已保留，客户端正在确认场景。
	worldBossFighting                   // 场次正在开放，确认席位可参与格子战斗。
	worldBossKilled                     // 本场共享首领已被击杀。
	worldBossExpired                    // 开放窗口结束，首领尚未击杀。
	worldBossLeft                       // 原席位离开或失去附着。
	worldBossDefeated                   // 原角色在本场先被击败。
)

// worldBossOccurrence 由运行服和运营时钟计算，客户端不能创建或延长场次。
type worldBossOccurrence struct {
	runID    uint64    // 当前逻辑运行服。
	id       string    // 运行服、模板及开场时刻共同决定的唯一编号。
	opensAt  time.Time // 原窗口开场时刻。
	closesAt time.Time // 原窗口结束时刻。
}

// worldBossObservation 跨分片传递的原会话身份在初始化后只读。
// shown 仅归角色 owner 写，revoked 供共享场次分片检查原附着是否已失效。
type worldBossObservation struct {
	occurrence worldBossOccurrence // 本次申请的场次身份。
	owner      *worker             // 保留永久写权限和原挂机角色的分片。
	actor      *worker             // 独占该运行服共享场次的分片。
	conn       Connection          // 原在线附着及其线程安全投递入口。
	auth       persistence.Auth    // 原角色、账号和会话代次。
	scene      uint64              // 此连接观察共享场次用的场景代次。
	baseline   uint64              // 此连接必须确认的场景基线。
	assets     roleAssetView       // 原角色 owner 冻结的只读资产视图，供匹配场次 ACK 后同步。
	shown      bool                // 角色 owner 已暂停本连接的普通场景投影。
	started    atomic.Bool         // 已由场次分片真正提交新场景首帧，离开时据此决定是否发返回场景。
	revoked    atomic.Bool         // 本席位的原命令和投递已经失效。
	rejoin     atomic.Bool         // 原连接断开或接管，允许保留已确认席位等候新认证。
}

// worldBossEnter 将原角色确认过的战斗属性交给场次分片，内容不引用原角色的可变背包或身体。
type worldBossEnter struct {
	observation *worldBossObservation // 原附着身份和接收回调的分片。
	snapshot    personalBossSnapshot  // 复制的已确认战斗快照。
	role        domain.Role           // 仅保留本次战斗所需的角色显示、基础身份和水位。
	store       *persistence.Store    // 原角色已经获准的运行服视图。
	snapshotID  string                // 快照内容哈希，用于本场席位校验。
	request     uint32                // 产生本次申请的原请求编号。
	resumeOnly  bool                  // 自动恢复只接受原场次已有的暂挂席位。
}

// worldBossWindow 按上海固定运营时区找到覆盖当前时刻的窗口或最近的下一场。
func worldBossWindow(cfg *devconfig.WorldBossConfig, runID uint64, now time.Time) (worldBossOccurrence, bool) {
	local := now.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	for shift := -1; shift <= 1; shift++ {
		base := day.AddDate(0, 0, shift)
		for _, offset := range cfg.DailyOpenOffsets {
			start := base.Add(offset)
			end := start.Add(cfg.Window)
			if !now.Before(end) {
				continue
			}
			key := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d", runID, cfg.TemplateID, start.UnixMilli())))
			return worldBossOccurrence{runID: runID, id: hex.EncodeToString(key[:16]), opensAt: start, closesAt: end}, !now.Before(start)
		}
	}
	return worldBossOccurrence{}, false
}

// worldBossForFixture 选择正在开放或最近预告的一场，保留运行服和真实开场生成的原身份。
func worldBossForFixture(f *devconfig.Fixture, runID uint64, now time.Time) (worldBossOccurrence, *devconfig.WorldBossConfig, bool) {
	if f.WorldBoss == nil {
		return worldBossOccurrence{}, nil, false
	}
	selected := f.WorldBoss
	window, open := worldBossWindow(selected, runID, now)
	for i := range f.WorldBossVariants {
		candidate := &f.WorldBossVariants[i]
		occurrence, active := worldBossWindow(candidate, runID, now)
		if !occurrence.opensAt.IsZero() && (window.opensAt.IsZero() || occurrence.opensAt.Before(window.opensAt)) {
			selected, window, open = candidate, occurrence, active
		}
	}
	return window, selected, open
}

// worldBossCommand 分流共享场次协议和已经切到共享场景的输入。
func (w *worker) worldBossCommand(owner *room, c command) bool {
	f := c.frame
	if f.ID == p.MsgC2SWorldBossQuery {
		w.queryWorldBoss(owner, f)
		return true
	}
	if f.ID == p.MsgC2SWorldBossEnter {
		w.beginWorldBoss(owner, f, time.Now())
		return true
	}
	obs := owner.worldBossObservation
	if obs == nil || !obs.shown {
		if f.ID == p.MsgC2SWorldBossLeave || obs != nil {
			w.failure(owner, f, p.CodeLineRecovering)
			return true
		}
		return false
	}
	switch f.ID {
	case p.MsgC2SWorldBossLeave, p.MsgC2SEnterWorldAck, p.MsgC2SMoveIntent, p.MsgC2SAttackIntent, p.MsgC2SCastIntent, p.MsgC2SSkillModeIntent, p.MsgC2SAutoBattle:
		if !w.postWorldBoss(obs.actor, func(actor *worker) { actor.applyWorldBossCommand(obs, f, time.Now()) }) {
			w.failure(owner, f, p.CodeServerBusy)
		}
	default:
		w.failure(owner, f, p.CodeLineForbidden)
	}
	return true
}

// postWorldBoss 在原有有界分片队列投递，跨分片消息不阻塞角色和日程时钟。
func (w *worker) postWorldBoss(target *worker, apply func(*worker)) bool {
	if target == w {
		apply(w)
		return true
	}
	return w.e.post(target, command{done: apply})
}

// queryWorldBoss 只观察已登记的运行服及配置日程。
func (w *worker) queryWorldBoss(owner *room, f wire.Frame) {
	var q pb.WorldBossQuery
	if proto.Unmarshal(f.Payload, &q) != nil || len(q.ProtoReflect().GetUnknown()) != 0 {
		w.failure(owner, f, p.CodeInvalidArgument)
		return
	}
	scene := owner.Scene
	if obs := owner.worldBossObservation; obs != nil && obs.shown {
		scene = obs.scene
	}
	if !owner.Ready || q.SceneEpoch != scene {
		w.failure(owner, f, p.CodeLineRecovering)
		return
	}
	store := w.roomStore(owner)
	if !store.Cfg.Fixture.ValidWorldBoss() {
		w.failure(owner, f, p.CodeLineForbidden)
		return
	}
	conn, auth := owner.Conn, owner.Auth
	runID, fixture := store.Cfg.RunID, &store.Cfg.Fixture
	actor := w.e.workers[runID%uint64(len(w.e.workers))]
	if !w.postWorldBoss(actor, func(actor *worker) {
		if conn.Alive() && conn.Binding() == uint64(auth.SessionEpoch) {
			conn.Deliver(wire.Message(p.MsgS2CWorldBossState, f.Request, uint64(auth.SessionEpoch), actor.worldBossStatus(fixture, runID, scene, time.Now())))
		}
	}) {
		w.failure(owner, f, p.CodeServerBusy)
	}
}

// beginWorldBoss 用普通 owner 的现有附着批准唯一特殊场景席位，不改变原挂机执行权。
func (w *worker) beginWorldBoss(owner *room, f wire.Frame, now time.Time) {
	var q pb.WorldBossEnter
	if proto.Unmarshal(f.Payload, &q) != nil || len(q.ProtoReflect().GetUnknown()) != 0 || q.SceneEpoch != owner.Scene || !validTravelID(q.OccurrenceId) {
		w.failure(owner, f, p.CodeInvalidArgument)
		return
	}
	store := w.roomStore(owner)
	fixture := &store.Cfg.Fixture
	if !fixture.ValidWorldBoss() || owner.currentWorld() != owner.PersonalWorld ||
		owner.Player.HP <= 0 || owner.Scene >= math.MaxUint64-1 {
		w.failure(owner, f, p.CodeLineForbidden)
		return
	}
	if !owner.Ready || owner.Pending || owner.personalBoss != nil || owner.worldBossObservation != nil || owner.Travel != nil && owner.Travel.Phase < travelCommitted || pendingPersonalBossTransfer(owner) {
		w.failure(owner, f, p.CodeLineRecovering)
		return
	}
	occurrence, cfg, open := worldBossForFixture(fixture, store.Cfg.RunID, now)
	if !open || occurrence.id != q.OccurrenceId || cfg.TemplateID != q.TemplateId || owner.Role.Level < cfg.MinimumLevel {
		w.failure(owner, f, p.CodeLineForbidden)
		return
	}
	w.submitWorldBossSeat(owner, store, cfg, occurrence, f, false)
}

// submitWorldBossSeat 将冻结的新身份交给场次，自动重入由原 actor 核对旧快照。
func (w *worker) submitWorldBossSeat(owner *room, store *persistence.Store, cfg *devconfig.WorldBossConfig, occurrence worldBossOccurrence, frame wire.Frame, resumeOnly bool) {
	baseline, available := w.nextTravelBaseline(owner.Baseline)
	if !available {
		if !resumeOnly {
			w.failure(owner, frame, p.CodeLineRecovering)
		}
		return
	}
	var snapshot personalBossSnapshot
	var snapshotID string
	if !resumeOnly {
		snapshot = w.freezePersonalBossSnapshot(owner, &devconfig.PersonalBossConfig{Map: cfg.Map})
		encoded, err := json.Marshal(snapshot)
		if err != nil || owner.Isolated || snapshot.Fighter.HP <= 0 {
			w.isolateRoom(owner)
			return
		}
		snapshotID = persistence.Hash(encoded)
	}
	assets, err := snapshotRoleAssets(owner.Role, store.Cfg.Fixture)
	if err != nil {
		w.isolateRoom(owner)
		return
	}
	obs := &worldBossObservation{occurrence: occurrence, owner: w, actor: w.e.workers[occurrence.runID%uint64(len(w.e.workers))],
		conn: owner.Conn, auth: owner.Auth, scene: owner.Scene + 1, baseline: baseline, assets: assets}
	role := domain.Role{ID: owner.Role.ID, Name: owner.Role.Name, Job: owner.Role.Job, Level: owner.Role.Level,
		EconomicSeq: owner.Role.EconomicSeq, Revision: owner.Role.Revision, Skills: snapshot.Skills, HostingSeconds: owner.Role.HostingSeconds,
		SkillSlotIDs: snapshot.SkillSlotIDs}
	enter := worldBossEnter{observation: obs, snapshot: snapshot, role: role, store: store, snapshotID: snapshotID, request: frame.Request, resumeOnly: resumeOnly}
	owner.worldBossObservation = obs
	if !w.postWorldBoss(obs.actor, func(actor *worker) { actor.reserveWorldBoss(enter, time.Now()) }) {
		w.detachWorldBoss(owner)
		if !resumeOnly {
			w.failure(owner, frame, p.CodeServerBusy)
		}
	}
}

// suspendWorldBossObservation 撤销旧会话输入权，并记住普通场景新 ACK 后应接回的场次。
func (w *worker) suspendWorldBossObservation(owner *room) {
	if obs := owner.worldBossObservation; obs != nil && obs.started.Load() {
		obs.rejoin.Store(true)
		owner.worldBossResume = obs.occurrence
	}
	w.detachWorldBoss(owner)
}

// resumeWorldBossObservation 在新短票和普通场景 ACK 后重新请求同一旧席位。
func (w *worker) resumeWorldBossObservation(owner *room, now time.Time) {
	previous := owner.worldBossResume
	owner.worldBossResume = worldBossOccurrence{}
	if previous.id == "" || owner.personalBoss != nil || owner.worldBossObservation != nil || owner.Scene >= math.MaxUint64-1 {
		return
	}
	store := w.roomStore(owner)
	if previous.runID != store.Cfg.RunID || !store.Cfg.Fixture.ValidWorldBoss() {
		return
	}
	window, cfg, active := worldBossForFixture(&store.Cfg.Fixture, previous.runID, now)
	if active && window.id == previous.id {
		w.submitWorldBossSeat(owner, store, cfg, window, wire.Frame{}, true)
	}
}

// detachWorldBoss 仅撤回原连接的参战资格，场次已经确认的贡献仍归原角色。
func (w *worker) detachWorldBoss(owner *room) {
	if obs := owner.worldBossObservation; obs != nil {
		obs.revoked.Store(true)
		owner.worldBossObservation = nil
	}
}

// finishWorldBossObservation 恢复原挂机场景的同一实体，只更换该连接的显示代次。
func (w *worker) finishWorldBossObservation(obs *worldBossObservation) {
	owner := w.rooms[obs.auth.RoleID]
	if owner == nil || owner.worldBossObservation != obs {
		return
	}
	w.detachWorldBoss(owner)
	if !obs.started.Load() || owner.Conn != obs.conn || owner.Auth != obs.auth || !obs.conn.Alive() || owner.Isolated {
		return
	}
	baseline, ok := w.nextTravelBaseline(obs.baseline)
	if !ok {
		w.isolateRoom(owner)
		return
	}
	owner.Scene, owner.Baseline = obs.scene+1, baseline
	owner.Ready, owner.PetBornReady = false, nil
	owner.AwaitUntil = time.Now().Add(10 * time.Second)
	owner.InputSequence, owner.TravelHistory, owner.Travel = 0, nil, nil
	if entry := w.enterLineResult(owner); entry != nil {
		entry.ServerTransitionId = obs.occurrence.id
		w.send(owner, p.MsgS2CEnterLine, 0, entry)
		w.bindingState(owner, 1)
		w.baseline(owner)
	}
}
