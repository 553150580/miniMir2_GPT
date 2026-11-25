// 本文件运行个人首领的第二个格子现场，收束真实战斗结果并经原角色 owner 提交。
package game

import (
	"math"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// personalBossBattle 为挑战子房间标记隔离模式，并持有本场真正的权威首领。
type personalBossBattle struct {
	challenge *bossChallenge       // 独立场次的参与、生命和唯一终态。
	grant     bossParticipantGrant // 玩家及其已批准宠物共用的归属授权。
	fighter   devconfig.Fighter    // 已冻结的玩家装备、等级、被动属性。
	boss      *entity              // 统一战斗核心中的首领身体。
	hitSeq    uint64               // 为已经完成命中校验的事实分配单调序号。
}

// newPersonalBossBattle 用原批准快照创建没有永久写能力的临时角色与怪物实体。
func newPersonalBossBattle(owner *room, cfg *devconfig.PersonalBossConfig, snapshot personalBossSnapshot,
	challenge *bossChallenge, grant bossParticipantGrant, baseline uint64, now time.Time) *room {
	monster := &entity{Handle: 10, Kind: 3, Def: cfg.Monster.ID, Pos: cfg.Map.MonsterSpawns[0], HP: challenge.hp,
		MaxHP: challenge.hp, MP: cfg.Monster.MP, MaxMP: cfg.Monster.MP, Monster: &cfg.Monster, Life: 1}
	world := &worldState{MapID: cfg.Map.ID, BossMap: &cfg.Map, Monsters: []*entity{monster}, MonsterCount: 1, Auto: true}
	child := &room{worldState: world, runStore: owner.runStore, ExecutionEpoch: challenge.identity.execution,
		Auth: owner.Auth, Conn: owner.Conn, Scene: owner.Scene + 1, Baseline: baseline,
		AutoSkills: snapshot.autoSkills,
		Role: domain.Role{ID: owner.Role.ID, Name: owner.Role.Name, Job: owner.Role.Job, Level: owner.Role.Level, EconomicSeq: owner.Role.EconomicSeq, Revision: owner.Role.Revision, Skills: snapshot.Skills,
			HostingSeconds: owner.Role.HostingSeconds, SkillSlotIDs: snapshot.SkillSlotIDs},
		Player: entity{Handle: 1, Kind: 1, Def: owner.Role.Job, Pos: cfg.Map.PlayerSpawn, HP: snapshot.Fighter.HP, MaxHP: snapshot.Fighter.HP,
			MP: snapshot.Fighter.MP, MaxMP: snapshot.Fighter.MP, Life: 1},
		bossBattle: &personalBossBattle{challenge: challenge, grant: grant, fighter: snapshot.Fighter, boss: monster},
		AwaitUntil: minBossTime(now.Add(10*time.Second), challenge.expiresAt), LastTick: now, LastBroadcast: now}
	if snapshot.Pet != nil {
		child.Pet, child.PetWorld, child.PetLifeSequence = snapshot.Pet, child.currentWorld().Ref, snapshot.Pet.Life
	}
	return child
}

// minBossTime 返回先达到的握手期限或场次期限。
func minBossTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// suppressOrdinaryObservation 把普通挂机继续保留在线，但不向正在观察挑战的连接串发普通场景表现。
func suppressOrdinaryObservation(owner *room, messageID uint16) bool {
	if (owner.personalBoss == nil || !owner.personalBoss.shown) && (owner.worldBossObservation == nil || !owner.worldBossObservation.shown) {
		return false
	}
	switch messageID {
	case p.MsgS2CSceneBaseline, p.MsgS2CCombatImpact, p.MsgS2CAttackAccepted, p.MsgS2CCastAccepted, p.MsgS2CCastStep,
		p.MsgS2CSkillModeState, p.MsgS2CDeathNotice, p.MsgS2CEntityDisplacement, p.MsgS2CPetBorn, p.MsgS2CHostingState:
		return true
	default:
		return false
	}
}

// castRoomIntent 让真实格子施法在挑战中仅作用临时属性，永久药水消耗仍只允许原房间处理。
func (w *worker) castRoomIntent(r *room, q *pb.CastIntent, now time.Time) uint32 {
	if r.bossBattle != nil {
		return w.castDirected(r, q.SkillId, q.TargetHandle, q.InputSequence, q.Direction, now, q.TargetGrid)
	}
	return w.castWithManaRecovery(r, q, now)
}

// applyPersonalBossDamage 接收统一格子战斗已裁决的伤害，临时实体死亡不会触发普通击杀收益。
func (w *worker) applyPersonalBossDamage(r *room, from, target *entity, amount int64, now time.Time, context []hitContext) {
	battle := r.bossBattle
	at := now
	if len(context) != 0 && !context[0].At.IsZero() {
		at = context[0].At
	}
	if battle.challenge.finish != nil || at.Before(battle.challenge.lastEventAt) || !at.Before(battle.challenge.expiresAt) {
		return
	}
	switch target {
	case battle.boss:
		if from == nil || from != &r.Player && from != r.Pet || battle.hitSeq == math.MaxUint64 {
			return
		}
		battle.hitSeq++
		amount = battle.challenge.hit(bossDamageFact{grant: battle.grant, sequence: battle.hitSeq, at: at, amount: amount})
		if amount == 0 {
			return
		}
		if r.worldBossCombat != nil && battle.challenge.finish != nil {
			w.freezeWorldBossRewardEvidence(r.worldBossCombat)
		}
	case &r.Player, r.Pet:
		if from != battle.boss {
			return
		}
	default:
		return
	}
	wearStatuses(target, amount)
	target.HP = max(0, target.HP-amount)
	target.Action, target.ActionUntil = 3, now.Add(200*time.Millisecond)
	if target.HP != 0 {
		return
	}
	removeDeathStatuses(target)
	target.Action = 4
	if target == &r.Player {
		if arena := r.worldBossCombat; arena != nil {
			arena.noteRewardPresence(r.Role.ID, persistence.WorldBossPresenceDefeated, at)
		} else {
			battle.challenge.playerDefeated(battle.grant, at)
		}
	}
	if target == r.Pet && w.petRuntimeEnabled() && !w.archivePetDeath(r, r.currentWorld(), reference(r, target), now) {
		w.isolateRoom(r)
	}
	w.snapshot(r)
}

// stepPersonalBoss 每帧先推进独立过期与已受理命中，再走公共格子核心。普通 owner 仍另行 tick。
func (w *worker) stepPersonalBoss(owner *room, now time.Time) {
	run := owner.personalBoss
	if run == nil || run.battle == nil || owner.Isolated || w.e.stopped.Load() {
		return
	}
	battle := run.battle
	challenge := battle.bossBattle.challenge
	if challenge.finish == nil {
		if run.abort || battle.Isolated {
			w.interruptPersonalBoss(owner, now)
			return
		}
		if run.shown && (owner.Conn != run.conn || !run.conn.Alive()) {
			w.detachPersonalBoss(owner)
		}
		if run.shown && !battle.Ready && !now.Before(battle.AwaitUntil) && now.Before(run.grant.ExpiresAt) {
			run.conn.Close("personal boss baseline acknowledgement timeout")
			w.detachPersonalBoss(owner)
		}
		if run.started && !now.Before(battle.LastTick) {
			until := minBossTime(now, run.grant.ExpiresAt.Add(-time.Nanosecond))
			if !until.Before(battle.LastTick) {
				battle.LastTick = until
				w.expireModes(battle, until)
				changed := w.tickStatuses(battle, until)
				changed = w.tickHealing(battle, until) || changed
				if w.expirePetCorpses(battle, until) || changed {
					w.snapshot(battle)
				}
				w.advanceEvents(battle, until)
			}
		}
		challenge.expire(now)
		if run.started && challenge.finish == nil && !battle.Isolated && !now.Before(battle.LastTick) {
			w.stepGridCombat(battle, battle.BossMap, now, !battle.Ready)
		}
	}
	if challenge.finish != nil {
		w.finishPersonalBoss(owner)
	}
}

// interruptPersonalBoss 关闭旧附着受理。预约未知时先保留取消意图，待原确认后结清。
func (w *worker) interruptPersonalBoss(owner *room, now time.Time) {
	run := owner.personalBoss
	if run == nil {
		return
	}
	run.abort = true
	if run.battle == nil {
		return
	}
	challenge := run.battle.bossBattle.challenge
	if challenge.finish == nil {
		challenge.interrupt(challenge.identity, now)
	}
	w.finishPersonalBoss(owner)
}

// finishPersonalBoss 从唯一终态冻结结算证明，只在普通角色 owner 的持久事务中写入。
func (w *worker) finishPersonalBoss(owner *room) {
	if owner.Pending || owner.Isolated {
		return
	}
	run := owner.personalBoss
	result, ok := run.battle.bossBattle.challenge.proposal()
	if !ok || len(result.contributions) != 1 {
		return
	}
	grant := run.grant
	q := persistence.PersonalBossFinish{ChallengeID: grant.ChallengeID, ReservationID: grant.ReservationID, Execution: grant.Execution,
		RuleVersion: grant.RuleVersion, ConfigHash: grant.ConfigHash, Outcome: persistence.PersonalBossOutcome(result.kind),
		FinishedAt: result.finishedAt.UTC().Truncate(time.Millisecond), Damage: result.contributions[0].damage, HPRemaining: result.hpRemaining}
	store := w.roomStore(owner)
	w.personalBossJob(owner, "finish", grant.ChallengeID, run.auth, true, func() (domain.Role, persistence.PersonalBossChallenge, error) {
		return store.FinishPersonalBoss(w.e.ctx, run.auth, q)
	}, func(role domain.Role, terminal persistence.PersonalBossChallenge, err error) {
		if err != nil || terminal.ChallengeID != grant.ChallengeID || terminal.Execution != grant.Execution || terminal.Outcome != q.Outcome ||
			terminal.ReservationID != grant.ReservationID || !terminal.FinishedAt.Equal(q.FinishedAt) || terminal.Damage != q.Damage || terminal.HPRemaining != q.HPRemaining {
			w.isolateRoom(owner)
			return
		}
		previousPrivilege := owner.Role.PrivilegeUntil
		owner.Role = role
		w.acceptRolePrivilege(owner, time.Now(), previousPrivilege)
		w.restoreAfterPersonalBoss(owner, run)
	})
}

// restoreAfterPersonalBoss 仅在原永久终态得到确认后向当前客户端恢复未复制的普通现场。
func (w *worker) restoreAfterPersonalBoss(owner *room, run *personalBossRun) {
	kind := run.battle.bossBattle.challenge.finish.kind
	w.personalBossState(owner, run, 0, bossStateDefeated+uint32(kind-bossDefeated), 0)
	owner.personalBoss = nil
	if !run.shown || owner.Conn == nil || owner.Conn != run.conn || !run.conn.Alive() {
		return
	}
	baseline, ok := w.nextTravelBaseline(run.battle.Baseline)
	if !ok {
		w.isolateRoom(owner)
		return
	}
	owner.Scene, owner.Baseline = run.battle.Scene+1, baseline
	owner.Ready, owner.PetBornReady = false, nil
	owner.AwaitUntil = time.Now().Add(10 * time.Second)
	owner.InputSequence, owner.TravelHistory, owner.Travel = 0, nil, nil
	if entry := w.enterLineResult(owner); entry != nil {
		entry.ServerTransitionId = run.begin.ChallengeID
		w.send(owner, p.MsgS2CEnterLine, 0, entry)
		w.bindingState(owner, 1)
		w.baseline(owner)
	}
}
