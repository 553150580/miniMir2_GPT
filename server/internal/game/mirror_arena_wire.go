// 本文件将已确认的镜像竞技业务账和临时格子观测转换成独立协议。
package game

import (
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func mirrorRecordPB(summary persistence.MirrorSummary) *pb.MirrorArenaRecord {
	return &pb.MirrorArenaRecord{ChallengeId: domain.Bytes(summary.ChallengeID), AttackerName: summary.AttackerName,
		DefenderName: summary.DefenderName, DefenseVersion: summary.DefenseVersion, Outcome: uint32(summary.Outcome),
		AwardedPoints: summary.Points, StartedAtUnixMs: optionalBossUnixMillis(summary.StartedAt), FinishedAtUnixMs: optionalBossUnixMillis(summary.FinishedAt)}
}

func mirrorLobbyState(r *room, info persistence.MirrorInfo, rule uint64) *pb.MirrorArenaState {
	out := &pb.MirrorArenaState{SceneEpoch: r.Scene, Open: !info.SeasonStart.IsZero(), RuleVersion: rule,
		SeasonStartUnixMs: optionalBossUnixMillis(info.SeasonStart), SeasonEndUnixMs: optionalBossUnixMillis(info.SeasonEnd),
		DailyLimit: info.DailyLimit, EntriesUsed: info.EntriesUsed, SeasonPoints: info.SeasonPoints, Streak: info.Streak}
	for _, offer := range info.Offers {
		out.Offers = append(out.Offers, &pb.MirrorArenaOffer{OfferId: domain.Bytes(offer.ID), DefenderName: offer.DefenderName,
			DefenderJob: offer.DefenderJob, DefenderLevel: offer.DefenderLevel, DefenseVersion: offer.DefenseVersion})
	}
	for _, record := range info.AttackHistory {
		out.AttackHistory = append(out.AttackHistory, mirrorRecordPB(record))
	}
	for _, record := range info.DefenseHistory {
		out.DefenseHistory = append(out.DefenseHistory, mirrorRecordPB(record))
	}
	if info.ActiveChallenge != nil {
		out.Challenge = mirrorRecordPB(info.ActiveChallenge.MirrorSummary)
	}
	return out
}

func mergeMirrorRun(out *pb.MirrorArenaState, run *mirrorRun) {
	out.Challenge = mirrorRecordPB(run.grant.MirrorSummary)
	out.BattleState, out.BattleSceneEpoch, out.BattleMapId = run.phase, run.scene, run.cfg.Map.ID
	deadline := run.deadline
	if deadline.IsZero() {
		deadline = run.prepareUntil
	}
	out.BattleDeadlineUnixMs = optionalBossUnixMillis(deadline)
}

func (w *worker) sendMirrorError(r *room, request, code uint32) {
	w.send(r, p.MsgS2CMirrorArenaState, request, &pb.MirrorArenaState{SceneEpoch: r.Scene, ErrorCode: code,
		RuleVersion: w.roomStore(r).Cfg.Fixture.RuleVersion})
}

func (w *worker) sendMirrorRun(r *room, request uint32, run *mirrorRun) {
	if !r.Ready || r.Auth != run.auth || r.Conn == nil || !r.Conn.Alive() {
		return
	}
	state := &pb.MirrorArenaState{SceneEpoch: r.Scene, Open: true, RuleVersion: run.grant.RuleVersion,
		SeasonStartUnixMs: optionalBossUnixMillis(run.grant.SeasonStart), SeasonEndUnixMs: optionalBossUnixMillis(run.grant.SeasonEnd),
		SeasonPoints: run.grant.SeasonPoints, Streak: run.grant.Streak}
	mergeMirrorRun(state, run)
	w.send(r, p.MsgS2CMirrorArenaState, request, state)
}

// sendMirrorCombatFrame 与普通场的实体及序号空间独立，旧会话的临时事件不在新票重播。
func (w *worker) sendMirrorCombatFrame(r *room, run *mirrorRun, now time.Time) {
	run.lastFrame = now
	observations := run.battle.drainObservations()
	if !r.Ready || r.Auth != run.auth || r.Conn == nil || !r.Conn.Alive() || run.frameSeq == math.MaxUint64 {
		return
	}
	run.frameSeq++
	frame := &pb.MirrorArenaCombatFrame{SceneEpoch: r.Scene, ChallengeId: domain.Bytes(run.grant.ChallengeID),
		BattleSceneEpoch: run.scene, Sequence: run.frameSeq, ServerUnixMs: now.UnixMilli(), Entities: w.mirrorCombatEntities(run.battle)}
	remaining := wire.MaxFrame - 32 - proto.Size(frame) // 为外层帧和后续字段标签留出空间。
	for _, event := range observations {
		raw, err := proto.Marshal(event.Message)
		if err != nil {
			continue
		}
		observation := &pb.MirrorArenaCombatObservation{MessageId: uint32(event.ID), Payload: raw}
		remaining -= proto.Size(observation) + 4
		if remaining < 0 {
			break
		}
		frame.Observations = append(frame.Observations, observation)
	}
	w.send(r, p.MsgS2CMirrorArenaCombatFrame, 0, frame)
}
