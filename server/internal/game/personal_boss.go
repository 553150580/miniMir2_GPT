// 本文件协调个人首领的永久预约、独立场景附着、结束结算与普通场景恢复。
package game

import (
	"encoding/json"
	"math"
	"slices"
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
	bossStateBooking        uint32 = iota + 1 // 正在等待角色永久预约确认。
	bossStateEntering                         // 独立战斗已建好，等待正确的场景基线确认。
	bossStateFighting                         // 客户端已确认基线，可以操作角色。
	bossStateDefeated                         // 首领击杀由角色 owner 确认。
	bossStateEscaped                          // 未击杀且正确到期。
	bossStateCancelled                        // 玩家退出或执行归属中断。
	bossStatePlayerDefeated                   // 玩家先被首领击败。
)

// personalBossRun 是普通房间唯一挑战槽。角色连接与永久写权限仍归普通房间。
type personalBossRun struct {
	requestID string                            // 客户端首次入口请求身份，用于重复请求识别。
	begin     persistence.PersonalBossBegin     // 已冻结的完整预约，未知结果用原参数重试。
	grant     persistence.PersonalBossChallenge // 已确认的永久预约。
	auth      persistence.Auth                  // 当前拥有原预约提交权的角色会话。
	conn      Connection                        // 正在观察挑战的附着。掉线时释放，原战斗保持。
	battle    *room                             // 同一分片持有的独立场景，临时状态和普通挂机隔离。
	shown     bool                              // 当前附着已收到挑战换图状态，需要恢复普通场景。
	started   bool                              // 首次已确认战斗基线，离线及后续换票期间继续行动。
	abort     bool                              // 持久预约未定时收到中断，确认后直接提交终态。
}

// personalBossSnapshot 保存在服务器的固定入场战斗数据，账务版本和资产不复制到战斗房间。
type personalBossSnapshot struct {
	Fighter      devconfig.Fighter     // 按原装备、被动技能、角色等级计算的属性。
	Skills       []domain.LearnedSkill // 当时确认为已学习的技能与等级。
	SkillSlotIDs []uint32              // 当时已保存的四个攻击技能槽，缺省保留原选择规则。
	Pet          *entity               // 当时允许参战宠物的独立临时身体副本。
	autoSkills   *autoSkillSettings    // 只读偏好快照，独立于用于持久预约的战斗摘要。
}

// personalBossCommand 在当前连接上分流入口、退出、场景确认和战斗指令。
func (w *worker) personalBossCommand(owner *room, c command) bool {
	frame := c.frame
	if frame.ID == p.MsgC2SPersonalBossQuery {
		w.personalBossQuery(owner, frame)
		return true
	}
	if frame.ID == p.MsgC2SPersonalBossEnter {
		w.beginPersonalBoss(owner, frame, time.Now())
		return true
	}
	if frame.ID == p.MsgC2SPersonalBossLeave {
		var q pb.PersonalBossLeave
		run := owner.personalBoss
		if run == nil || proto.Unmarshal(frame.Payload, &q) != nil || q.ChallengeId != run.begin.ChallengeID ||
			run.battle == nil && q.SceneEpoch != owner.Scene || run.battle != nil && (!run.shown || q.SceneEpoch != run.battle.Scene) {
			w.failure(owner, frame, p.CodeInvalidArgument)
			return true
		}
		w.interruptPersonalBoss(owner, time.Now())
		return true
	}
	run := owner.personalBoss
	if run == nil || run.battle == nil {
		if run != nil && (frame.ID == p.MsgC2SMapTravelPrepare || frame.ID == p.MsgC2SMapTravelCommit) {
			w.failure(owner, frame, p.CodeLineRecovering)
			return true
		}
		return false
	}
	battle := run.battle
	if frame.ID == p.MsgC2SEnterWorldAck {
		if !run.shown {
			return false // 健康重连先确认普通场景，再授予新的挑战观察。
		}
		w.ackPersonalBoss(owner, run, frame, time.Now())
		return true
	}
	if !battle.Ready || battle.bossBattle.challenge.finish != nil || !time.Now().Before(run.grant.ExpiresAt) {
		w.failure(owner, frame, p.CodeLineRecovering)
		return true
	}
	switch frame.ID {
	case p.MsgC2SAutoBattle:
		w.bossAutoBattleIntent(battle, frame)
	case p.MsgC2SMoveIntent, p.MsgC2SSkillModeIntent, p.MsgC2SAttackIntent, p.MsgC2SCastIntent:
		w.roomCommand(battle, c)
	default:
		w.failure(owner, frame, p.CodeLineForbidden)
	}
	return true
}

// personalBossState 向当前附着发送已经验证的原挑战状态变化。
func (w *worker) personalBossState(owner *room, run *personalBossRun, request, state, code uint32) {
	if owner.Conn == nil || owner.Conn != run.conn || owner.Auth != run.auth || !run.conn.Alive() {
		return
	}
	scene := owner.Scene
	if run.battle != nil {
		scene = run.battle.Scene
	}
	payload := &pb.PersonalBossState{ChallengeId: run.begin.ChallengeID, SceneEpoch: scene, TemplateId: run.begin.TemplateID, State: state, ErrorCode: code}
	if !run.grant.ExpiresAt.IsZero() {
		payload.ExpiresAtUnixMs = run.grant.ExpiresAt.UnixMilli()
	}
	w.send(owner, p.MsgS2CPersonalBossState, request, payload)
}

// beginPersonalBoss 冻结原 owner 的战斗快照并异步预约，确认前不创建可游玩实例。
func (w *worker) beginPersonalBoss(owner *room, frame wire.Frame, now time.Time) {
	var msg pb.PersonalBossEnter
	if proto.Unmarshal(frame.Payload, &msg) != nil || !validTravelID(msg.ChallengeId) || msg.SceneEpoch != owner.Scene {
		w.failure(owner, frame, p.CodeInvalidArgument)
		return
	}
	if run := owner.personalBoss; run != nil {
		if run.requestID != msg.ChallengeId || run.begin.TemplateID != msg.TemplateId {
			w.failure(owner, frame, p.CodeLineRecovering)
			return
		}
		state := bossStateBooking
		if run.shown {
			state = bossStateEntering
			if run.battle.Ready {
				state = bossStateFighting
			}
		}
		w.personalBossState(owner, run, frame.Request, state, 0)
		return
	}
	store := w.roomStore(owner)
	fixture := &store.Cfg.Fixture
	template := fixture.PersonalBossByTemplate(msg.TemplateId)
	if template == nil || owner.currentWorld() != owner.PersonalWorld ||
		owner.Player.HP <= 0 || owner.Scene >= math.MaxUint64-1 {
		w.failure(owner, frame, p.CodeLineForbidden)
		return
	}
	if !owner.Ready || owner.Pending || owner.worldBossObservation != nil || owner.Travel != nil && owner.Travel.Phase < travelCommitted || pendingPersonalBossTransfer(owner) {
		w.failure(owner, frame, p.CodeLineRecovering)
		return
	}
	if owner.Role.PersonalBoss != nil && owner.Role.PersonalBoss.ActiveChallengeID != "" {
		w.recoverPersonalBoss(owner)
		w.failure(owner, frame, p.CodeLineRecovering)
		return
	}
	snapshot := w.freezePersonalBossSnapshot(owner, template)
	encoded, err := json.Marshal(snapshot)
	if err != nil || owner.Isolated || snapshot.Fighter.HP <= 0 {
		w.isolateRoom(owner)
		return
	}
	run := &personalBossRun{requestID: msg.ChallengeId, auth: owner.Auth, conn: owner.Conn,
		begin: persistence.PersonalBossBegin{ChallengeID: domain.ID(), Execution: ownerExecutionEpoch.Add(1), TemplateID: template.TemplateID,
			CombatSnapshot: persistence.Hash(encoded), ExpectedRevision: owner.Role.Revision, RuleVersion: fixture.RuleVersion, ConfigHash: fixture.Hash,
			StartedAt: now.UTC().Truncate(time.Millisecond), Duration: template.Duration, SuccessCooldown: template.SuccessCooldown,
			RetryCooldown: template.RetryCooldown, BossHP: template.Monster.HP}}
	owner.personalBoss = run
	w.personalBossState(owner, run, frame.Request, bossStateBooking, 0)
	w.personalBossJob(owner, "start", run.begin.ChallengeID, run.auth, false, func() (domain.Role, persistence.PersonalBossChallenge, error) {
		return store.BeginPersonalBoss(w.e.ctx, run.auth, run.begin)
	}, func(role domain.Role, grant persistence.PersonalBossChallenge, err error) {
		if err != nil {
			w.personalBossState(owner, run, frame.Request, bossStateCancelled, persistence.Code(err))
			owner.personalBoss = nil
			return
		}
		previousPrivilege := owner.Role.PrivilegeUntil
		owner.Role = role
		w.acceptRolePrivilege(owner, time.Now(), previousPrivilege)
		w.installPersonalBoss(owner, run, template, snapshot, grant, time.Now())
	})
}

// pendingPersonalBossTransfer 确保之前已经受理的瞬移先完成，再建立双场景观察关系。
func pendingPersonalBossTransfer(owner *room) bool {
	for _, world := range owner.worlds() {
		for _, cast := range world.Casts {
			if cast.Skill.Behavior == "teleport" {
				return true
			}
		}
	}
	return false
}

// personalBossJob 使用普通角色的永久操作槽，未知提交继续用捕获的原请求重试。
func (w *worker) personalBossJob(owner *room, stage, id string, auth persistence.Auth, settlement bool,
	work func() (domain.Role, persistence.PersonalBossChallenge, error), complete func(domain.Role, persistence.PersonalBossChallenge, error)) {
	key := "boss:" + stage + ":" + id
	owner.Pending, owner.PendingOperationID = true, key
	var task job
	task = job{target: w, settlement: settlement, run: func() func(*worker) {
		role, value, err := work()
		return func(w *worker) {
			if w.rooms[auth.RoleID] != owner || owner.Auth != auth || !owner.Pending || owner.PendingOperationID != key || owner.Isolated {
				return
			}
			if code := persistence.Code(err); code == p.CodeOperationPending || code == p.CodeServerBusy {
				owner.PendingRetry, owner.RetryAt = &task, time.Now().Add(250*time.Millisecond)
				return
			}
			owner.resumeAfterPending(time.Now())
			owner.PendingRetry, owner.PendingOperationID = nil, ""
			complete(role, value, err)
			w.after(owner)
		}
	}}
	if !w.submit(task) {
		owner.PendingRetry, owner.RetryAt = &task, time.Now().Add(100*time.Millisecond)
	}
}

// installPersonalBoss 校验预约身份，在同一原执行器内创建年轻的独立战斗场景。
func (w *worker) installPersonalBoss(owner *room, run *personalBossRun, template *devconfig.PersonalBossConfig, snapshot personalBossSnapshot, grant persistence.PersonalBossChallenge, now time.Time) {
	begin := run.begin
	if grant.ChallengeID != begin.ChallengeID || grant.RoleID != owner.Role.ID || grant.RunID != w.roomStore(owner).Cfg.RunID ||
		grant.Execution != begin.Execution || grant.RuleVersion != begin.RuleVersion || grant.ConfigHash != begin.ConfigHash || grant.TemplateID != begin.TemplateID ||
		grant.CombatSnapshot != begin.CombatSnapshot || grant.BossHP != begin.BossHP || grant.Outcome != 0 || !grant.StartedAt.Equal(begin.StartedAt) || !grant.ExpiresAt.Equal(begin.StartedAt.Add(begin.Duration)) {
		w.isolateRoom(owner)
		return
	}
	identity := bossChallengeIdentity{runID: grant.RunID, challengeID: grant.ChallengeID, execution: grant.Execution, ruleVersion: grant.RuleVersion, configHash: grant.ConfigHash}
	challenge, err := newBossChallenge(identity, grant.RoleID, grant.StartedAt, grant.ExpiresAt, grant.BossHP, 1)
	participant := bossParticipantGrant{challenge: identity, roleID: grant.RoleID, reservationID: grant.ReservationID, combatSnapshot: grant.CombatSnapshot}
	baseline, valid := w.nextTravelBaseline(owner.Baseline)
	if err != nil || !valid || !challenge.admit(participant, grant.StartedAt) {
		w.isolateRoom(owner)
		return
	}
	run.grant = grant
	run.battle = newPersonalBossBattle(owner, template, snapshot, challenge, participant, baseline, now)
	if run.abort {
		w.interruptPersonalBoss(owner, now)
		return
	}
	if !now.Before(grant.ExpiresAt) {
		w.stepPersonalBoss(owner, now)
		return
	}
	owner.Manual, owner.Destination, owner.PetBornReady = false, nil, nil
	owner.Navigation = navigationState{}
	if owner.Conn != nil && owner.Conn == run.conn && owner.Conn.Alive() {
		w.presentPersonalBoss(owner, run, now)
	} else {
		w.detachPersonalBoss(owner)
	}
}

// ackPersonalBoss 校验独立场景的连接、完整基线和计时后才允许任何新战斗输入。
func (w *worker) ackPersonalBoss(owner *room, run *personalBossRun, frame wire.Frame, now time.Time) {
	battle := run.battle
	var ack pb.EnterWorldAck
	if battle.Ready || battle.Conn != owner.Conn || owner.Conn != run.conn || !run.conn.Alive() || battle.bossBattle.challenge.finish != nil ||
		!now.Before(battle.AwaitUntil) || !now.Before(run.grant.ExpiresAt) || proto.Unmarshal(frame.Payload, &ack) != nil ||
		ack.SceneEpoch != battle.Scene || ack.BaselineId != battle.Baseline {
		return
	}
	if !run.started {
		if ack.ChallengeAutoBattleEnabled != nil {
			battle.Auto = *ack.ChallengeAutoBattleEnabled
		}
		battle.LastTick = now
	}
	battle.Ready, run.started = true, true
	w.snapshot(battle)
	w.modes(battle, now)
	w.assets(owner)
	w.hosting(battle, 0)
	w.sendSyncReady(battle, frame.Request, &pb.SyncReady{SceneEpoch: battle.Scene, BaselineId: battle.Baseline, NextEconomicCommandSeq: uint64(owner.Role.EconomicSeq + 1)})
	w.personalBossState(owner, run, 0, bossStateFighting, 0)
}

// freezePersonalBossSnapshot 按普通战斗同样的数值冻结玩家与可参战宠物，副本无任何资产写权限。
func (w *worker) freezePersonalBossSnapshot(owner *room, cfg *devconfig.PersonalBossConfig) personalBossSnapshot {
	snapshot := w.freezeBossCombatSnapshot(owner, &cfg.Map)
	if snapshot.Pet == nil {
		return snapshot
	}
	pos, ok := travelPetPoint(&cfg.Map, cfg.Map.PlayerSpawn, &cfg.Map.PetSpawn, []*entity{{Pos: cfg.Map.MonsterSpawns[0], HP: 1}}, 1)
	if !ok {
		snapshot.Pet = nil
		return snapshot
	}
	snapshot.Pet.Pos = pos
	return snapshot
}

// freezeBossCombatSnapshot 只冻结可参战的身体，由具体场次按真实出生和占位分配入场位置。
func (w *worker) freezeBossCombatSnapshot(owner *room, target *devconfig.MapTemplate) personalBossSnapshot {
	snapshot := personalBossSnapshot{Fighter: w.basePlayerFighter(owner), Skills: slices.Clone(owner.Role.Skills),
		SkillSlotIDs: slices.Clone(owner.Role.SkillSlotIDs), autoSkills: owner.AutoSkills}
	pet := owner.worldPet(owner.currentWorld())
	if pet == nil || pet.HP <= 0 || mapForbidsPets(target) {
		return snapshot
	}
	snapshot.Pet = &entity{Handle: petHandle, Kind: pet.Kind, Def: pet.Def, HP: pet.MaxHP, MaxHP: pet.MaxHP, MP: pet.MaxMP, MaxMP: pet.MaxMP, Life: 1}
	if pet.PetRuntime != nil {
		runtime := *pet.PetRuntime
		runtime.FollowSuppressed = false
		snapshot.Pet.PetRuntime = &runtime
	}
	return snapshot
}

// recoverPersonalBoss 用刚取得的当前会话对账旧执行者的悬置预约，只写角色 owner 槽。
func (w *worker) recoverPersonalBoss(owner *room) {
	store := w.roomStore(owner)
	if owner.Pending || owner.personalBoss != nil || owner.Role.PersonalBoss == nil || owner.Role.PersonalBoss.ActiveChallengeID == "" {
		return
	}
	id, auth := owner.Role.PersonalBoss.ActiveChallengeID, owner.Auth
	w.personalBossJob(owner, "recover", id, auth, true, func() (domain.Role, persistence.PersonalBossChallenge, error) {
		return store.RecoverPersonalBoss(w.e.ctx, auth)
	}, func(role domain.Role, _ persistence.PersonalBossChallenge, err error) {
		if err == nil {
			previousPrivilege := owner.Role.PrivilegeUntil
			owner.Role = role
			w.acceptRolePrivilege(owner, time.Now(), previousPrivilege)
			w.assets(owner)
		} else if hardAuthorityFailure(persistence.Code(err)) {
			w.isolateRoom(owner)
		}
	})
}
