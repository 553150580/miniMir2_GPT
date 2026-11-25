// 本文件路由讨伐场输入和基线，使原普通房间保持权益结算但不串场显示。
package game

import (
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// suppressDungeonObservation 不把普通房间的动作和画面投给正在观察同一讨伐场的连接。
func suppressDungeonObservation(r *room, id uint16) bool {
	if r.dungeonMatch == nil || !r.dungeonMatch.shown {
		return false
	}
	switch id {
	case p.MsgS2CSceneBaseline, p.MsgS2CCombatImpact, p.MsgS2CAttackAccepted, p.MsgS2CCastAccepted, p.MsgS2CCastStep,
		p.MsgS2CSkillModeState, p.MsgS2CDeathNotice, p.MsgS2CEntityDisplacement, p.MsgS2CPetBorn, p.MsgS2CHostingState:
		return true
	default:
		return false
	}
}

// dungeonBattleCommand 只有已装配的原场角色可以把受理的有限输入交给原战斗分片。
func (w *worker) dungeonBattleCommand(owner *room, cmd command) bool {
	obs := owner.dungeonMatch
	if obs == nil || !obs.shown && cmd.frame.ID == p.MsgC2SEnterWorldAck {
		return false
	}
	if !obs.shown || owner.Conn != obs.conn || owner.Auth != obs.auth || obs.revoked.Load() {
		w.failure(owner, cmd.frame, p.CodeLineRecovering)
		return true
	}
	switch cmd.frame.ID {
	case p.MsgC2SEnterWorldAck, p.MsgC2SMoveIntent, p.MsgC2SAttackIntent, p.MsgC2SCastIntent, p.MsgC2SSkillModeIntent, p.MsgC2SAutoBattle:
		if !w.postWorldBoss(obs.actor, func(actor *worker) { actor.applyDungeonBattleCommand(obs, cmd.frame, time.Now()) }) {
			w.failure(owner, cmd.frame, p.CodeServerBusy)
		}
	default:
		w.failure(owner, cmd.frame, p.CodeLineForbidden)
	}
	return true
}

// tickDungeonArenas 首先发布已持久确认的观察，再推进场次并确认原终态。
func (w *worker) tickDungeonArenas(now time.Time) {
	if len(w.dungeonArenas) == 0 || w.e.stopped.Load() {
		return
	}
	for _, arena := range w.dungeonArenas {
		if arena.outcome == persistence.DungeonActive {
			w.refreshDungeonObservers(arena, now)
			if arena.readyToFight() {
				w.stepDungeonArena(arena, now)
				stage := arena.stage + 1
				if arena.outcome == persistence.DungeonActive && stage != arena.reportedStage {
					arena.reportedStage = stage
					w.reportDungeonPhase(arena, dungeonFighting)
				}
			} else if !now.Before(minBossTime(arena.loadingDeadline(), arena.grant.ExpiresAt)) {
				// 全队尚未完成入场，系统收束此预约并按原日退款。
				arena.end(persistence.DungeonSystemInterrupted, now)
			}
		}
		if arena.outcome != persistence.DungeonActive && !arena.ledger.announced {
			arena.ledger.announced = true
			w.reportDungeonPhase(arena, dungeonSettling)
		}
		w.persistDungeonBattleFinish(arena, now)
	}
}

// readyToFight 确认整队首次加载。已确认身体的健康断线不撤销开战资格。
func (a *dungeonArena) readyToFight() bool {
	if len(a.seats) != len(a.grant.Members) || len(a.seats) == 0 {
		return false
	}
	for _, seat := range a.seats {
		if !seat.started {
			return false
		}
	}
	return true
}

// loadingDeadline 使用装配时已冻结的首次基线时限，后续换票不能延长原等待。
func (a *dungeonArena) loadingDeadline() time.Time {
	return a.lastTick.Add(10 * time.Second)
}

func (w *worker) reportDungeonPhase(arena *dungeonArena, phase uint32) {
	for roleID, obs := range arena.observations {
		if seat := arena.seats[roleID]; seat != nil && obs.acked && seat.room.Conn == obs.conn && !obs.revoked.Load() {
			w.sendDungeonBattleState(arena, seat.room, obs, phase)
		}
	}
}

// refreshDungeonObservers 在场次分片原地替换被持久新票接回的观察，原战斗身体保持唯一。
func (w *worker) refreshDungeonObservers(arena *dungeonArena, now time.Time) {
	for roleID, obs := range arena.observations {
		seat := arena.seats[roleID]
		if seat == nil {
			continue
		}
		child := seat.room
		for next := obs.next.Load(); next != nil; next = obs.next.Load() {
			obs = next
		}
		if arena.observations[roleID] != obs {
			arena.observations[roleID] = obs
			child.Conn = nil
			child.Auth, child.Scene, child.Baseline = obs.auth, obs.scene, obs.baseline
			child.InputSequence, child.Sequence = 0, 0
			child.PetBornReady = nil
			child.Role.EconomicSeq, child.Role.HostingSeconds = obs.economicSeq, obs.hostingSeconds
			if obs.assets.inventory != nil {
				child.Role.Revision = int64(obs.assets.inventory.Revision)
			}
		}
		if obs.revoked.Load() || !obs.conn.Alive() || obs.conn.Binding() != uint64(obs.auth.SessionEpoch) {
			if child.Conn != nil || child.Manual {
				child.Conn, child.Manual, child.Destination = nil, false, nil
				child.Auto = true
			}
			continue
		}
		if !obs.present.Load() || obs.sent.Load() {
			continue
		}
		// 组员可能一人已托管，发送首帧期间仍保持原自动战斗状态和原 HP／冷却。
		entry := w.enterLineResult(child)
		if entry == nil {
			arena.end(persistence.DungeonSystemInterrupted, now)
			return
		}
		child.Conn = obs.conn
		w.sendDungeonSceneTransition(child, obs.sourceScene, arena.grant.ChallengeID, entry)
		w.baseline(child)
		w.sendDungeonBattleState(arena, child, obs, dungeonLoading)
		child.Conn = nil
		obs.sent.Store(true)
	}
}

// sendDungeonSceneTransition 先授权当前连接安装这份精确基线，再投递对应的入场消息。
func (w *worker) sendDungeonSceneTransition(r *room, source uint64, challenge string, entry *pb.EnterLineResult) {
	entry.ServerTransitionId = challenge
	w.send(r, p.MsgS2CDungeonSceneTransition, 0, &pb.DungeonSceneTransition{ChallengeId: domain.Bytes(challenge), SourceSceneEpoch: source,
		TargetSceneEpoch: entry.SceneEpoch, TargetBaselineId: entry.BaselineId, TargetMapId: entry.RoomId, TargetSceneKind: entry.SceneKind})
	w.send(r, p.MsgS2CEnterLine, 0, entry)
}

func (w *worker) sendDungeonBattleState(arena *dungeonArena, child *room, obs *dungeonAttachment, phase uint32) {
	stage := uint32(arena.stage + 1)
	obs.stage.Store(stage)
	w.send(child, p.MsgS2CDungeonBattleState, 0, &pb.DungeonBattleState{ChallengeId: domain.Bytes(arena.grant.ChallengeID),
		SceneEpoch: obs.scene, Stage: stage, StageCount: uint32(len(arena.template.Stages)), ExpiresAtUnixMs: arena.grant.ExpiresAt.UnixMilli(), State: phase})
}

// applyDungeonBattleCommand 核验当前真实场次、会话和首帧，不给迟到的普通输入重新分配场景。
func (w *worker) applyDungeonBattleCommand(obs *dungeonAttachment, frame wire.Frame, now time.Time) {
	arena := w.dungeonArenas[obs.proof.ChallengeID]
	if arena == nil || arena.observations[obs.auth.RoleID] != obs || obs.revoked.Load() || !obs.sent.Load() ||
		!obs.conn.Alive() || obs.conn.Binding() != frame.Binding || frame.Binding != uint64(obs.auth.SessionEpoch) {
		return
	}
	seat := arena.seats[obs.auth.RoleID]
	if seat == nil {
		return
	}
	child := seat.room
	if frame.ID == p.MsgC2SEnterWorldAck {
		var ack pb.EnterWorldAck
		if obs.acked || arena.outcome != persistence.DungeonActive || !now.Before(arena.grant.ExpiresAt) ||
			!seat.started && !now.Before(arena.loadingDeadline()) ||
			proto.Unmarshal(frame.Payload, &ack) != nil || ack.SceneEpoch != obs.scene || ack.BaselineId != obs.baseline {
			return
		}
		if ack.ChallengeAutoBattleEnabled != nil {
			child.Auto = *ack.ChallengeAutoBattleEnabled
			if !child.Auto {
				child.AutoTarget = actorRef{}
			}
		}
		obs.acked, seat.started, child.Ready, child.Conn = true, true, true, obs.conn
		if obs.assets.inventory != nil {
			child.Role.Revision = int64(obs.assets.inventory.Revision)
		}
		w.snapshot(child)
		w.modes(child, now)
		w.sendRoleAssetView(child, obs.assets)
		w.hosting(child, 0)
		w.sendSyncReady(child, frame.Request, &pb.SyncReady{SceneEpoch: child.Scene, BaselineId: child.Baseline,
			NextEconomicCommandSeq: uint64(child.Role.EconomicSeq + 1)})
		if arena.reportedStage != 0 {
			w.sendDungeonBattleState(arena, child, obs, dungeonFighting)
		}
		return
	}
	if !obs.acked || child.Conn != obs.conn || child.Player.HP <= 0 || !arena.readyToFight() || !arena.acceptsActions(now) {
		obs.conn.Deliver(wire.Frame{ID: frame.ID, Request: frame.Request, Binding: frame.Binding, Error: p.CodeLineRecovering})
		return
	}
	if frame.ID == p.MsgC2SAutoBattle {
		w.bossAutoBattleIntent(child, frame)
		return
	}
	w.roomCommand(child, command{conn: obs.conn, frame: frame})
}
