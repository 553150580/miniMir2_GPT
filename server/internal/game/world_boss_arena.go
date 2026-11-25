// 本文件管理共享世界首领场次的唯一实体、席位、观察基线与个人离场。
package game

import (
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// worldBossArena 只由承载本 run 的场次分片读写。共享首领在这里始终只有一个实体。
type worldBossArena struct {
	occurrence worldBossOccurrence        // 运营窗口和逻辑运行服。
	config     *devconfig.WorldBossConfig // 当前装配冻结的地图、怪物和席位约束。
	challenge  *bossChallenge             // 共享生命、命中事实、贡献和唯一终态。
	boss       *entity                    // 所有参战角色在同一个坐标系统观察的唯一身体。
	seats      map[string]*worldBossSeat  // 仍占场的原会话席位。
	resumable  map[string]*worldBossSeat  // 已确认战斗但连接失效的身体和动作冷却。
	names      map[string]string          // 已加入角色的显示名，离场后用于当前榜单。
	nextHandle uint32                     // 本场唯一的玩家和宠物句柄分配水位。
	castID     uint64                     // 同场全部角色共用的施法去重水位。
	eventID    uint64                     // 同场战斗观察事件的有序分配水位。
	lastTick   time.Time                  // 避免旧逻辑帧回拨全场时钟。
	lastState  time.Time                  // 最近一次合并发送共享排行榜的时刻。
}

// worldBossSeat 保存世界 actor 自行拥有的战斗身体和只读原入场授权。
type worldBossSeat struct {
	observation *worldBossObservation // 原角色连接与控制代次。
	room        *room                 // 统一移动和技能引擎使用的临时角色战斗上下文。
	shown       bool                  // 已向原连接发送独立地图及首基线。
	done        bool                  // 已通知唯一场次终态，等待自动返回。
	priorEpoch  int64                 // 暂挂时最后认证代次，防止旧票进入已撤销的席位。
}

// worldBossStatus 为查询返回当前开放、终态或下一场的日程与共享进度。
func (w *worker) worldBossStatus(cfg *devconfig.WorldBossConfig, runID, scene uint64, now time.Time) *pb.WorldBossState {
	occurrence, open := worldBossWindow(cfg, runID, now)
	if arena := w.worldBossArenas[runID]; arena != nil && arena.occurrence.id == occurrence.id {
		return arena.state(scene, worldBossFighting)
	}
	state := worldBossClosed
	if open {
		state = worldBossFighting
	}
	return &pb.WorldBossState{SceneEpoch: scene, OccurrenceId: occurrence.id, TemplateId: cfg.TemplateID, State: state,
		OpensAtUnixMs: occurrence.opensAt.UnixMilli(), ExpiresAtUnixMs: occurrence.closesAt.UnixMilli(), Hp: cfg.Monster.HP, HpMax: cfg.Monster.HP}
}

// state 冻结当前观察的共享生命和榜单，顺序以有效伤害、后以原角色身份确定。
func (a *worldBossArena) state(scene uint64, phase uint32) *pb.WorldBossState {
	if phase == worldBossFighting && a.challenge.finish != nil {
		phase = worldBossExpired
		if a.challenge.finish.kind == bossDefeated {
			phase = worldBossKilled
		}
	}
	contributions := make([]*bossContribution, 0, len(a.challenge.participants))
	for _, contribution := range a.challenge.participants {
		if contribution.damage > 0 {
			contributions = append(contributions, contribution)
		}
	}
	sort.Slice(contributions, func(i, j int) bool {
		left, right := contributions[i], contributions[j]
		return left.damage > right.damage || left.damage == right.damage && left.grant.roleID < right.grant.roleID
	})
	ranks := make([]*pb.WorldBossRank, 0, len(contributions))
	for index, contribution := range contributions {
		ranks = append(ranks, &pb.WorldBossRank{Position: uint32(index + 1), DisplayName: a.names[contribution.grant.roleID], Damage: contribution.damage})
	}
	return &pb.WorldBossState{SceneEpoch: scene, OccurrenceId: a.occurrence.id, TemplateId: a.config.TemplateID, State: phase,
		OpensAtUnixMs: a.occurrence.opensAt.UnixMilli(), ExpiresAtUnixMs: a.occurrence.closesAt.UnixMilli(), Hp: a.challenge.hp, HpMax: a.boss.MaxHP, Ranks: ranks}
}

// reserveWorldBoss 只在相同真实窗口内接受原 owner 的有界申请。
func (w *worker) reserveWorldBoss(enter worldBossEnter, now time.Time) {
	obs := enter.observation
	if obs.revoked.Load() || !obs.conn.Alive() || obs.conn.Binding() != uint64(obs.auth.SessionEpoch) {
		return
	}
	fixture := &enter.store.Cfg.Fixture
	cfg := fixture.WorldBoss
	occurrence, open := worldBossWindow(cfg, enter.store.Cfg.RunID, now)
	if w.e.stopped.Load() || !open || occurrence.id != obs.occurrence.id || obs.auth.RoleID != enter.role.ID {
		w.rejectWorldBoss(enter, p.CodeLineForbidden)
		return
	}
	if w.worldBossArenas == nil {
		w.worldBossArenas = make(map[uint64]*worldBossArena)
	}
	arena := w.worldBossArenas[occurrence.runID]
	if arena == nil || arena.occurrence.id != occurrence.id {
		if arena != nil && len(arena.seats) > 0 {
			w.rejectWorldBoss(enter, p.CodeLineRecovering)
			return
		}
		identity := bossChallengeIdentity{runID: occurrence.runID, challengeID: occurrence.id, execution: ownerExecutionEpoch.Add(1), ruleVersion: fixture.RuleVersion, configHash: fixture.Hash}
		challenge, err := newBossChallenge(identity, "", occurrence.opensAt, occurrence.closesAt, cfg.Monster.HP, cfg.ParticipantLimit)
		if err != nil {
			w.rejectWorldBoss(enter, p.CodeLineForbidden)
			return
		}
		boss := &entity{Handle: 10, Kind: 3, Def: cfg.Monster.ID, Monster: &cfg.Monster, Pos: cfg.Map.MonsterSpawns[0],
			HP: cfg.Monster.HP, MaxHP: cfg.Monster.HP, MP: cfg.Monster.MP, MaxMP: cfg.Monster.MP, Life: 1}
		arena = &worldBossArena{occurrence: occurrence, config: cfg, challenge: challenge, boss: boss,
			seats: make(map[string]*worldBossSeat), resumable: make(map[string]*worldBossSeat), names: make(map[string]string), nextHandle: 100}
		w.worldBossArenas[occurrence.runID] = arena
	}
	if arena.challenge.finish == nil {
		if previous := arena.seats[enter.role.ID]; previous != nil && shouldSuspendWorldBoss(previous) {
			w.suspendWorldBossSeat(arena, previous)
		}
		if saved := arena.resumable[enter.role.ID]; saved != nil {
			if !arena.resumeSeat(enter, saved, now) {
				w.rejectWorldBoss(enter, p.CodeLineRecovering)
				return
			}
			w.offerWorldBossView(enter, arena)
			return
		}
	}
	if enter.resumeOnly || arena.challenge.finish != nil || arena.seats[enter.role.ID] != nil || arena.challenge.participants[enter.role.ID] != nil {
		w.rejectWorldBoss(enter, p.CodeLineForbidden)
		return
	}
	occupiedSeats := len(arena.challenge.participants)
	for _, seat := range arena.seats {
		if arena.challenge.participants[seat.room.Role.ID] == nil {
			occupiedSeats++
		}
	}
	if occupiedSeats >= cfg.ParticipantLimit {
		w.rejectWorldBoss(enter, p.CodeLineFull)
		return
	}
	if arena.addSeat(enter, now) == nil {
		w.rejectWorldBoss(enter, p.CodeLineFull)
		return
	}
	w.offerWorldBossView(enter, arena)
}

// offerWorldBossView 通过原 owner 注册本次地图附着，两个分片保持各自的唯一写权。
func (w *worker) offerWorldBossView(enter worldBossEnter, arena *worldBossArena) {
	obs := enter.observation
	if !w.postWorldBoss(obs.owner, func(ownerShard *worker) { ownerShard.showWorldBoss(enter, arena) }) {
		obs.revoked.Store(true)
		obs.conn.Close("world boss owner busy")
	}
}

// addSeat 给实际身体分配共享占格和稳定句柄，尚未确认时只保留落格。
func (a *worldBossArena) addSeat(enter worldBossEnter, now time.Time) *worldBossSeat {
	obs := enter.observation
	mapTemplate := &a.config.Map
	occupied := make(map[devconfig.Point]uint64)
	a.eachLiving(func(body *entity) { occupied[body.Pos] = 1 })
	point, ok := sharedTownLanding(mapTemplate, mapTemplate.PlayerSpawn, nil, occupied, 0, true)
	if !ok {
		return nil
	}
	pet := enter.snapshot.Pet
	if pet != nil {
		petPos, available := sharedTownLanding(mapTemplate, pet.Pos, &point, occupied, 0, true)
		if !available {
			return nil
		}
		pet.Pos = petPos
	}
	grant := bossParticipantGrant{challenge: a.challenge.identity, roleID: enter.role.ID, reservationID: domain.ID(), combatSnapshot: enter.snapshotID}
	world := &worldState{MapID: mapTemplate.ID, BossMap: mapTemplate, Monsters: []*entity{a.boss}, MonsterCount: 1}
	a.nextHandle++
	fighter := enter.snapshot.Fighter
	child := &room{worldState: world, runStore: enter.store, Auth: obs.auth, Conn: obs.conn, Role: enter.role, Scene: obs.scene, Baseline: obs.baseline,
		Player:     entity{Handle: a.nextHandle, Kind: 1, Def: enter.role.Job, Pos: point, HP: fighter.HP, MaxHP: fighter.HP, MP: fighter.MP, MaxMP: fighter.MP, Life: 1},
		bossBattle: &personalBossBattle{challenge: a.challenge, grant: grant, fighter: fighter, boss: a.boss}, worldBossCombat: a,
		AwaitUntil: minBossTime(now.Add(10*time.Second), a.occurrence.closesAt), LastTick: now, LastBroadcast: now}
	if pet != nil {
		a.nextHandle++
		pet.Handle = a.nextHandle
		child.Pet, child.PetWorld, child.PetLifeSequence, child.PetHandleSequence = pet, child.currentWorld().Ref, pet.Life, pet.Handle
	}
	seat := &worldBossSeat{observation: obs, room: child}
	a.seats[enter.role.ID], a.names[enter.role.ID] = seat, enter.role.Name
	return seat
}

// rejectWorldBoss 让原角色清理未发布的观察申请，不占用场次或普通场景。
func (w *worker) rejectWorldBoss(enter worldBossEnter, code uint32) {
	obs := enter.observation
	if !w.postWorldBoss(obs.owner, func(ownerShard *worker) {
		owner := ownerShard.rooms[obs.auth.RoleID]
		if owner != nil && owner.worldBossObservation == obs {
			ownerShard.detachWorldBoss(owner)
			if !enter.resumeOnly && owner.Conn == obs.conn && obs.conn.Alive() {
				ownerShard.failure(owner, wire.Frame{ID: p.MsgC2SWorldBossEnter, Request: enter.request}, code)
			}
		}
	}) {
		obs.revoked.Store(true)
		obs.conn.Close("world boss owner busy")
	}
}

// showWorldBoss 的前半由原 owner 停止旧地图投影，后半只由场次分片发送新地图。
func (w *worker) showWorldBoss(enter worldBossEnter, arena *worldBossArena) {
	obs := enter.observation
	owner := w.rooms[obs.auth.RoleID]
	if owner == nil || owner.worldBossObservation != obs || owner.Auth != obs.auth || owner.Conn != obs.conn || !obs.conn.Alive() || owner.Isolated {
		obs.revoked.Store(true)
		return
	}
	owner.Manual, owner.Destination, owner.PetBornReady = false, nil, nil
	owner.Navigation = navigationState{}
	obs.shown = true
	if !w.postWorldBoss(obs.actor, func(actor *worker) {
		seat := arena.seats[obs.auth.RoleID]
		if seat == nil || seat.observation != obs || obs.revoked.Load() || !obs.conn.Alive() {
			return
		}
		seat.shown = true
		actor.send(seat.room, p.MsgS2CWorldBossState, enter.request, arena.state(obs.scene, worldBossEntering))
		if entry := actor.enterLineResult(seat.room); entry != nil {
			entry.ServerTransitionId = arena.occurrence.id
			obs.started.Store(true)
			actor.send(seat.room, p.MsgS2CEnterLine, 0, entry)
			actor.baseline(seat.room)
		}
	}) {
		w.detachWorldBoss(owner)
		obs.conn.Close("world boss actor busy")
	}
}

// applyWorldBossCommand 验证角色、连接、真实场次和观察代次后才允许临时战斗输入。
func (w *worker) applyWorldBossCommand(obs *worldBossObservation, frame wire.Frame, now time.Time) {
	arena := w.worldBossArenas[obs.occurrence.runID]
	if arena == nil || arena.occurrence.id != obs.occurrence.id || obs.revoked.Load() || !obs.conn.Alive() || obs.conn.Binding() != frame.Binding {
		return
	}
	seat := arena.seats[obs.auth.RoleID]
	if seat == nil || seat.observation != obs || !seat.shown {
		return
	}
	child := seat.room
	if frame.ID == p.MsgC2SWorldBossLeave {
		var leave pb.WorldBossLeave
		if proto.Unmarshal(frame.Payload, &leave) != nil || len(leave.ProtoReflect().GetUnknown()) != 0 || leave.SceneEpoch != obs.scene || leave.OccurrenceId != arena.occurrence.id {
			w.failure(child, frame, p.CodeInvalidArgument)
			return
		}
		until := minBossTime(now, arena.occurrence.closesAt.Add(-time.Nanosecond))
		if arena.challenge.finish == nil {
			w.advanceWorldBossEvents(arena, until)
			arena.challenge.expire(now)
		}
		w.removeWorldBossSeat(arena, seat, worldBossLeft)
		return
	}
	if !now.Before(arena.occurrence.closesAt) || arena.challenge.finish != nil || child.Isolated || child.Player.HP <= 0 {
		w.failure(child, frame, p.CodeLineRecovering)
		return
	}
	if frame.ID == p.MsgC2SEnterWorldAck {
		var ack pb.EnterWorldAck
		if child.Ready || !now.Before(child.AwaitUntil) || proto.Unmarshal(frame.Payload, &ack) != nil || ack.SceneEpoch != obs.scene || ack.BaselineId != obs.baseline {
			return
		}
		w.advanceWorldBossEvents(arena, now)
		if now.After(arena.lastTick) {
			arena.lastTick = now
		}
		if !arena.challenge.admit(child.bossBattle.grant, now) {
			w.removeWorldBossSeat(arena, seat, worldBossLeft)
			return
		}
		child.Ready, child.LastTick = true, now
		w.snapshot(child)
		// 其它端先收到该新身体，再能依据句柄和出生代次播放它的首个施法。
		for _, peer := range arena.seats {
			if peer != seat && visibleBossSeat(arena, peer) {
				w.snapshot(peer.room)
			}
		}
		w.modes(child, now)
		w.sendSyncReady(child, frame.Request, &pb.SyncReady{SceneEpoch: obs.scene, BaselineId: obs.baseline, NextEconomicCommandSeq: uint64(child.Role.EconomicSeq + 1)})
		w.send(child, p.MsgS2CWorldBossState, 0, arena.state(obs.scene, worldBossFighting))
		return
	}
	if !child.Ready {
		w.failure(child, frame, p.CodeLineRecovering)
		return
	}
	w.roomCommand(child, command{conn: obs.conn, frame: frame})
}

// removeWorldBossSeat 归还战斗占格，由原角色 owner 完成连接返回。
func (w *worker) removeWorldBossSeat(arena *worldBossArena, seat *worldBossSeat, phase uint32) {
	obs := seat.observation
	delete(arena.seats, obs.auth.RoleID)
	if !seat.room.Ready {
		delete(arena.names, obs.auth.RoleID)
	}
	if seat.shown && !obs.revoked.Load() && obs.conn.Alive() && (phase != worldBossFighting || !seat.done) {
		w.send(seat.room, p.MsgS2CWorldBossState, 0, arena.state(obs.scene, phase))
	}
	obs.revoked.Store(true)
	if !w.postWorldBoss(obs.owner, func(ownerShard *worker) { ownerShard.finishWorldBossObservation(obs) }) {
		obs.conn.Close("world boss return queue busy")
	}
}
