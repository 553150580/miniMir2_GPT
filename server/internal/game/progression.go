// 本文件核对经验和装备磨损回执，在可验证的原角色生命上应用升级恢复并推送经验结果。
package game

import (
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// finishSettlementReceipt 核对来源回执，再协调收益、升级恢复和客户端同步。
// w 是调用该方法的房间执行器实例，b 记录b，使用结算会话绑定表示，q 记录结算，role 记录角色，receipt 记录回执，使用操作表示。
// err 保存当前操作的失败原因，供调用方检查或返回，retry 记录重试，使用任务或职业编号表示，finished 记录“finished”，使用供当前流程调用的函数表示。
func (w *worker) finishSettlementReceipt(b settlementBinding, q persistence.Settlement, role domain.Role, receipt persistence.Operation, err error, retry *job, finished func(persistence.Operation)) {
	// r 保存当前处理所用的房间。
	r := b.room
	// owned 保存根据房间集合中的指定项与r的计算结果与授权与授权的计算结果的计算结果与已隔离的计算结果与待确认的计算结果与结算代次与代次的计算结果的计算结果与待确认操作标识与标识的计算结果的计算结果与待确认“fingerprint”与“fingerprint”的计算结果计算的“owned”。
	owned := w.rooms[b.auth.RoleID] == r && r.Auth == b.auth && !r.Isolated && r.Pending && r.SettlementGeneration == b.generation && r.PendingOperationID == b.id && r.PendingFingerprint == b.fingerprint
	if owned && err == nil && !confirmedWear(q, receipt) {
		err = persistence.Fail(p.CodeOperationConflict, "磨损确认回执与原事实不一致")
	}
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

// experienceReceipt 提取与原来源和规则一致的已确认经验详情。
// q 记录结算，op 记录操作。
func experienceReceipt(q persistence.Settlement, op persistence.Operation) (*persistence.ExperienceGrantReceipt, bool) {
	if op.Kind != "settlement" || op.SourceSeq != q.SourceSeq || op.Settlement == nil || op.Settlement.RuleVersion < 20 || op.Settlement.Experience == nil {
		return nil, false
	}
	// g 保存经验，用于当前步骤中的g。
	g := op.Settlement.Experience
	if g.RawExperience != q.Experience || g.RawExperience < 0 || g.ActualGranted < 0 || g.ActualGranted > g.RawExperience || g.ToPool < 0 || g.ToPool > g.ActualGranted || g.ToProgress != g.ActualGranted-g.ToPool || g.Rejected != g.RawExperience-g.ActualGranted || g.LevelAfter < g.LevelBefore {
		return nil, false
	}
	return g, true
}

// applyExperienceRecovery 只把升级恢复作用到原世界中仍存活的原出生角色。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，b 记录b，使用结算会话绑定表示，q 记录结算，op 记录操作。
func (w *worker) applyExperienceRecovery(r *room, b settlementBinding, q persistence.Settlement, op persistence.Operation) {
	// g 保存由经验回执取得的g，ok 表明查询、校验或类型转换是否成功。
	g, ok := experienceReceipt(q, op)
	if !ok || op.RoleID != r.Role.ID {
		w.isolateRoom(r)
		return
	}
	if op.Revision != r.Role.Revision || b.sourceBefore >= q.SourceSeq || g.ActualGranted <= 0 || g.LevelAfter <= g.LevelBefore {
		return
	}
	// body 保存由“resolve”取得的消息正文。
	body := w.resolve(r, b.progressionActor)
	if body != &r.Player || body.HP <= 0 || r.world(b.progressionActor.World) != r.currentWorld() {
		return
	}
	// 仅原出生真正的新奖励升级恢复；不改Life、动作、CD、导航或死亡身份。
	body.HP, body.MP = body.MaxHP, body.MaxMP
}

// experienceGrantResult 向当前可观察连接发送最新确认的经验和成长结果。
// w 是调用该方法的房间执行器实例，r 指向当前执行器拥有并更新的目标房间，b 记录b，使用结算会话绑定表示，q 记录结算，op 记录操作。
func (w *worker) experienceGrantResult(r *room, b settlementBinding, q persistence.Settlement, op persistence.Operation) {
	if w.e.Store.Cfg.Fixture.RuleVersion < 20 || r.Conn == nil || !r.Ready || r.Isolated || op.RoleID != r.Role.ID || op.Revision != r.Role.Revision || b.sourceBefore >= q.SourceSeq {
		return
	}
	// g 保存由经验回执取得的g，ok 表明查询、校验或类型转换是否成功。
	g, ok := experienceReceipt(q, op)
	if !ok || g.RawExperience == 0 && g.MigrationBefore == nil {
		return
	}
	// state 保存由成长状态取得的状态，err 接收成长状态过程中可能产生的错误，供后续失败分支处理。
	state, err := persistence.ProgressionState(r.Role, w.e.Store.Cfg.Fixture)
	if err != nil {
		w.isolateRoom(r)
		return
	}
	// out 保存当前处理所用的输出。
	out := &pb.ExperienceGrantResult{Role: r.Role.Summary().Key, RoleRevision: uint64(op.Revision), SourceSeq: uint64(op.SourceSeq), CurveVersion: g.CurveVersion, RawExperience: g.RawExperience, AcceptedExperience: g.ActualGranted, PooledExperience: g.ToPool, RejectedExperience: g.Rejected, LevelBefore: g.LevelBefore, LevelAfter: g.LevelAfter, State: state, RuleVersion: op.Settlement.RuleVersion}
	if g.MigrationBefore != nil {
		out.Migrated = true
		out.LegacyLevelBefore = g.MigrationBefore.Level
		out.LegacyLevelAfter = g.MigrationAfter.Level
	}
	w.send(r, p.MsgS2CExperienceGrantResult, 0, out)
}
