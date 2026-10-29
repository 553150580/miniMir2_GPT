package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

func (w *worker) finishSettlementReceipt(b settlementBinding, q persistence.Settlement, role domain.Role, receipt persistence.Operation, err error, retry *job, finished func(persistence.Operation)) {
	r := b.room
	owned := w.rooms[b.auth.RoleID] == r && r.Auth == b.auth && !r.Isolated && r.Pending && r.SettlementGeneration == b.generation && r.PendingOperationID == b.id && r.PendingFingerprint == b.fingerprint
	w.finishSettlement(b, q, role, err, retry, func() {
		if w.e.Store.Cfg.Fixture.RuleVersion >= 20 {
			w.applyExperienceRecovery(r, b, q, receipt)
		}
		if !r.Isolated && finished != nil {
			finished(receipt)
		}
	})
	if owned && err == nil && !r.Isolated {
		w.equipmentRecycleResult(r, receipt)
		w.experienceGrantResult(r, b, q, receipt)
	}
}

func experienceReceipt(q persistence.Settlement, op persistence.Operation) (*persistence.ExperienceGrantReceipt, bool) {
	if op.Kind != "settlement" || op.SourceSeq != q.SourceSeq || op.Settlement == nil || op.Settlement.RuleVersion < 20 || op.Settlement.Experience == nil {
		return nil, false
	}
	g := op.Settlement.Experience
	if g.RawExperience != q.Experience || g.RawExperience < 0 || g.ActualGranted < 0 || g.ActualGranted > g.RawExperience || g.ToPool < 0 || g.ToPool > g.ActualGranted || g.ToProgress != g.ActualGranted-g.ToPool || g.Rejected != g.RawExperience-g.ActualGranted || g.LevelAfter < g.LevelBefore {
		return nil, false
	}
	return g, true
}

func (w *worker) applyExperienceRecovery(r *room, b settlementBinding, q persistence.Settlement, op persistence.Operation) {
	g, ok := experienceReceipt(q, op)
	if !ok || op.RoleID != r.Role.ID {
		w.isolateRoom(r)
		return
	}
	if op.Revision != r.Role.Revision || b.sourceBefore >= q.SourceSeq || g.ActualGranted <= 0 || g.LevelAfter <= g.LevelBefore {
		return
	}
	body := w.resolve(r, b.progressionActor)
	if body != &r.Player || body.HP <= 0 || r.world(b.progressionActor.World) != r.currentWorld() {
		return
	}
	// 仅原出生真正的新奖励升级恢复；不改Life、动作、CD、导航或死亡身份。
	body.HP, body.MP = body.MaxHP, body.MaxMP
}

func (w *worker) experienceGrantResult(r *room, b settlementBinding, q persistence.Settlement, op persistence.Operation) {
	if w.e.Store.Cfg.Fixture.RuleVersion < 20 || r.Conn == nil || !r.Ready || r.Isolated || op.RoleID != r.Role.ID || op.Revision != r.Role.Revision || b.sourceBefore >= q.SourceSeq {
		return
	}
	g, ok := experienceReceipt(q, op)
	if !ok || g.RawExperience == 0 && g.MigrationBefore == nil {
		return
	}
	state, err := persistence.ProgressionState(r.Role, w.e.Store.Cfg.Fixture)
	if err != nil {
		w.isolateRoom(r)
		return
	}
	out := &pb.ExperienceGrantResult{Role: r.Role.Summary().Key, RoleRevision: uint64(op.Revision), SourceSeq: uint64(op.SourceSeq), CurveVersion: g.CurveVersion, RawExperience: g.RawExperience, AcceptedExperience: g.ActualGranted, PooledExperience: g.ToPool, RejectedExperience: g.Rejected, LevelBefore: g.LevelBefore, LevelAfter: g.LevelAfter, State: state, RuleVersion: op.Settlement.RuleVersion}
	if g.MigrationBefore != nil {
		out.Migrated = true
		out.LegacyLevelBefore = g.MigrationBefore.Level
		out.LegacyLevelAfter = g.MigrationAfter.Level
	}
	w.send(r, p.MsgS2CExperienceGrantResult, 0, out)
}
