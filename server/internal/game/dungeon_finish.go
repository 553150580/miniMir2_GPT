// 本文件确认讨伐链原终态，并在每位角色的原分片恢复普通地图观察。
package game

import (
	"log/slog"
	"sync/atomic"
	"time"

	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// dungeonFinishState 由战斗分片独占；对账和排队失败始终重试同一份结果。
type dungeonFinishState struct {
	proposal  persistence.DungeonFinish    // 首次权威终态选定后不再修改的存储提案。
	result    persistence.DungeonChallenge // 原事务已确认的永久终态。
	returns   []*dungeonOwnerReturn        // 等待各原角色分片接收的原角色快照。
	retryAt   time.Time                    // 数据库忙或重放等待的最早重投时刻。
	inFlight  bool                         // 原终态提案已有数据库任务在途。
	failed    bool                         // 已确认的不可重试错误，未向客户端公布终态。
	announced bool                         // 结算中状态只公布一次，最终结果等待持久回执。
}

// dungeonOwnerReturn 在战斗分片与角色分片之间用独立原子值确认已回场。
type dungeonOwnerReturn struct {
	role  domain.Role   // 同一场次终态事务返回的角色聚合，只用于原 owner 更新。
	state atomic.Uint32 // 0 待投递，1 已投递，2 原 owner 完成回场。
}

// persistDungeonBattleFinish 冻结一次结果，未知提交沿同一编号和时间重新核对。
func (w *worker) persistDungeonBattleFinish(arena *dungeonArena, now time.Time) {
	ledger := &arena.ledger
	if ledger.result.ChallengeID != "" {
		w.deliverDungeonReturns(arena)
		return
	}
	if arena.outcome == persistence.DungeonActive || ledger.inFlight || ledger.failed || now.Before(ledger.retryAt) {
		return
	}
	if ledger.proposal.ChallengeID == "" {
		// 未击杀首领的场次在此释放权益观察；成功前已于真实击杀时固定证据。
		w.freezeDungeonRewards(arena, time.Time{})
		ledger.proposal = persistence.DungeonFinish{ChallengeID: arena.grant.ChallengeID, ReservationID: arena.grant.ReservationID,
			Execution: arena.grant.Execution, Outcome: arena.outcome, FinishedAt: arena.finishedAt}
		if arena.outcome == persistence.DungeonSucceeded {
			ledger.proposal.RewardAt, ledger.proposal.RewardEvidence = arena.rewardAt, arena.rewardEvidence
		}
	}
	store := arena.store
	if store == nil {
		store = w.e.runStores[arena.grant.RunID]
	}
	if store == nil || store.Epoch != arena.grant.RunEpoch || store.Instance != arena.grant.OwnerInstance {
		w.acceptDungeonFinish(arena, persistence.DungeonChallenge{}, nil, persistence.Fail(p.CodeRouteChanged, "原讨伐链运行服授权不可用"))
		return
	}
	proposal := ledger.proposal
	ledger.inFlight = w.submit(job{target: w, settlement: true, run: func() func(*worker) {
		result, roles, err := store.FinishOwnedDungeon(w.e.ctx, proposal)
		return func(w *worker) { w.acceptDungeonFinish(arena, result, roles, err) }
	}})
	if !ledger.inFlight {
		ledger.retryAt = now.Add(250 * time.Millisecond)
	}
}

// acceptDungeonFinish 只在确认后投递新资产和终态，避免把网络重试误当作成功或退款。
func (w *worker) acceptDungeonFinish(arena *dungeonArena, result persistence.DungeonChallenge, roles []domain.Role, err error) {
	ledger := &arena.ledger
	ledger.inFlight = false
	code := persistence.Code(err)
	if code == p.CodeOperationPending || code == p.CodeServerBusy {
		ledger.retryAt = time.Now().Add(250 * time.Millisecond)
		return
	}
	if err != nil || !validDungeonReturn(arena.grant, ledger.proposal, result, roles) {
		ledger.failed = true
		slog.Error("讨伐链终态未取得完整原确认", "run", arena.grant.RunID, "challenge", arena.grant.ChallengeID, "code", code)
		return
	}
	ledger.result = result
	for _, role := range roles {
		ledger.returns = append(ledger.returns, &dungeonOwnerReturn{role: role})
	}
	w.deliverDungeonReturns(arena)
}

// validDungeonReturn 拒绝错场或不完整的回执。角色排序来自已确认名单。
func validDungeonReturn(grant persistence.DungeonChallenge, proposal persistence.DungeonFinish, result persistence.DungeonChallenge, roles []domain.Role) bool {
	if result.ChallengeID != grant.ChallengeID || result.ReservationID != grant.ReservationID || result.Execution != grant.Execution ||
		result.RunID != grant.RunID || result.RunEpoch != grant.RunEpoch || result.OwnerInstance != grant.OwnerInstance ||
		result.Outcome != proposal.Outcome || result.Recovered || result.Refunded != (proposal.Outcome == persistence.DungeonSystemInterrupted) ||
		result.FinishedAt.UnixMilli() != proposal.FinishedAt.UnixMilli() || optionalBossUnixMillis(result.RewardAt) != optionalBossUnixMillis(proposal.RewardAt) ||
		len(result.Members) != len(grant.Members) || len(roles) != len(grant.Members) {
		return false
	}
	for i, role := range roles {
		member := grant.Members[i]
		reported := result.Members[i]
		if role.ID != member.RoleID || role.AccountID != member.AccountID || role.Origin != member.Origin ||
			reported.RoleID != member.RoleID || reported.AccountID != member.AccountID || reported.Origin != member.Origin ||
			reported.CombatSnapshot != member.CombatSnapshot || reported.EntryDay != member.EntryDay || reported.EntriesAfter != member.EntriesAfter {
			return false
		}
	}
	return true
}

// deliverDungeonReturns 只保留尚未回场的原确认。跨分片队列满或普通资产仍在途时可重投。
func (w *worker) deliverDungeonReturns(arena *dungeonArena) {
	ledger := &arena.ledger
	result := ledger.result
	for _, pending := range ledger.returns {
		if !pending.state.CompareAndSwap(0, 1) {
			continue
		}
		target := w.e.worker(pending.role.ID)
		if !w.postWorldBoss(target, func(owner *worker) {
			if owner.returnDungeonOwner(pending.role.ID, result, pending.role) {
				pending.state.Store(2)
			} else {
				pending.state.Store(0)
			}
		}) {
			pending.state.Store(0)
		}
	}
	for _, pending := range ledger.returns {
		if pending.state.Load() != 2 {
			return
		}
	}
	if w.dungeonArenas[result.ChallengeID] == arena {
		delete(w.dungeonArenas, result.ChallengeID)
	}
}

// drainDungeonArenas 保留已裁决胜负，系统停机中断仍在加载或尚未到期的原场次。
func (w *worker) drainDungeonArenas(now time.Time) (pending bool, failed int) {
	for _, arena := range w.dungeonArenas {
		markDungeonDrainOutcome(arena, now)
		w.persistDungeonBattleFinish(arena, now)
		if arena.ledger.failed {
			failed++
		} else if w.dungeonArenas[arena.grant.ChallengeID] == arena {
			pending = true
		}
	}
	return pending, failed
}

// markDungeonDrainOutcome 只有已完成全队确认的战斗经过原截止才算正常超时。
func markDungeonDrainOutcome(arena *dungeonArena, at time.Time) {
	if arena.outcome != persistence.DungeonActive {
		return
	}
	if arena.readyToFight() && !at.Before(arena.grant.ExpiresAt) {
		arena.end(persistence.DungeonFailed, arena.grant.ExpiresAt)
		return
	}
	arena.end(persistence.DungeonSystemInterrupted, at)
}
