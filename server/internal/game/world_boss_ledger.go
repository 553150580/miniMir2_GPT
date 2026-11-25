// 本文件将共享场次的恢复、首次预约和唯一终态通过数据库队列交回原场次分片。
package game

import (
	"log/slog"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

type worldBossLedger struct {
	store   *persistence.Store           // 当前运行服的原数据库围栏。
	loaded  bool                         // 冷启动已完成原场对账。
	loading []func(uint32)               // 等待同一次恢复查询的有界响应。
	latest  *persistence.WorldBossResult // 最近一场已经确认的永久结果。
	opening *worldBossOpening            // 同一原窗口唯一的待确认预约。
}

type worldBossOpening struct {
	spec       persistence.WorldBossSpec  // 重查未知提交时保持原执行和开场编号。
	occurrence worldBossOccurrence        // 已验证的原运营窗口。
	config     *devconfig.WorldBossConfig // 原窗口的地图与预算。
	waiting    []worldBossEnter           // 预约确认前尚未创建身体的申请。
	inFlight   bool                       // 本次原预约已提交数据库队列。
	retryAt    time.Time                  // 原预约可重新投递的时刻。
}

// withWorldBossLedger 合并同一运行服的首次对账，完成前不对外发布可入场状态。
func (w *worker) withWorldBossLedger(store *persistence.Store, complete func(uint32)) {
	if w.worldBossLedgers == nil {
		w.worldBossLedgers = make(map[uint64]*worldBossLedger)
	}
	ledger := w.worldBossLedgers[store.Cfg.RunID]
	if ledger == nil {
		ledger = &worldBossLedger{store: store}
		w.worldBossLedgers[store.Cfg.RunID] = ledger
	}
	if ledger.loaded {
		complete(0)
		return
	}
	if len(ledger.loading) >= cap(w.commands) {
		complete(p.CodeServerBusy)
		return
	}
	ledger.loading = append(ledger.loading, complete)
	if len(ledger.loading) != 1 {
		return
	}
	if !w.submit(job{target: w, settlement: true, run: func() func(*worker) {
		result, err := store.RecoverWorldBoss(w.e.ctx)
		return func(_ *worker) {
			code := persistence.Code(err)
			if code == 0 {
				ledger.loaded, ledger.latest = true, result
			}
			pending := ledger.loading
			ledger.loading = nil
			for _, send := range pending {
				send(code)
			}
		}
	}}) {
		ledger.loading = nil
		complete(p.CodeServerBusy)
	}
}

// openWorldBoss 预约成功前不创建首领或参与者。重复进场共用同一次预约。
func (w *worker) openWorldBoss(enter worldBossEnter, occurrence worldBossOccurrence, cfg *devconfig.WorldBossConfig, now time.Time) {
	ledger := w.worldBossLedgers[occurrence.runID]
	if ledger.latest != nil && ledger.latest.OccurrenceID == occurrence.id {
		w.rejectWorldBoss(enter, p.CodeLineForbidden)
		return
	}
	opening := ledger.opening
	if opening == nil {
		fixture := &ledger.store.Cfg.Fixture
		opening = &worldBossOpening{occurrence: occurrence, config: cfg,
			spec: persistence.WorldBossSpec{RunID: occurrence.runID, OccurrenceID: occurrence.id, TemplateID: cfg.TemplateID,
				Execution: ownerExecutionEpoch.Add(1), ConfigHash: fixture.Hash, RuleVersion: fixture.RuleVersion,
				OpensAt: occurrence.opensAt.UTC(), ClosesAt: occurrence.closesAt.UTC(), HPMax: cfg.Monster.HP}}
		ledger.opening = opening
	}
	if opening.occurrence.id != occurrence.id || len(opening.waiting) >= cfg.ParticipantLimit {
		w.rejectWorldBoss(enter, p.CodeLineRecovering)
		return
	}
	opening.waiting = append(opening.waiting, enter)
	w.advanceWorldBossOpening(ledger, now)
}

func (w *worker) advanceWorldBossOpening(ledger *worldBossLedger, now time.Time) {
	opening := ledger.opening
	if opening == nil || opening.inFlight || now.Before(opening.retryAt) {
		return
	}
	opening.inFlight = w.submit(job{target: w, settlement: true, run: func() func(*worker) {
		result, err := ledger.store.BeginWorldBoss(w.e.ctx, opening.spec)
		return func(w *worker) {
			opening.inFlight = false
			code := persistence.Code(err)
			if code == p.CodeOperationPending || code == p.CodeServerBusy {
				opening.retryAt = time.Now().Add(250 * time.Millisecond)
				return
			}
			ledger.opening = nil
			if err == nil {
				if result.Outcome == persistence.WorldBossActive {
					code = w.installWorldBossArena(opening, result)
				} else {
					ledger.latest = result
					code = p.CodeLineForbidden
				}
			}
			for _, enter := range opening.waiting {
				if code == 0 {
					w.reserveRecoveredWorldBoss(enter, time.Now())
				} else {
					w.rejectWorldBoss(enter, code)
				}
			}
		}
	}})
}

func (w *worker) installWorldBossArena(opening *worldBossOpening, result *persistence.WorldBossResult) uint32 {
	spec, cfg := result.WorldBossSpec, opening.config
	identity := bossChallengeIdentity{runID: spec.RunID, challengeID: spec.OccurrenceID, execution: spec.Execution, ruleVersion: spec.RuleVersion, configHash: spec.ConfigHash}
	challenge, err := newBossChallenge(identity, "", spec.OpensAt, spec.ClosesAt, spec.HPMax, cfg.ParticipantLimit)
	if err != nil || spec != opening.spec {
		return p.CodeLineForbidden
	}
	if w.worldBossArenas == nil {
		w.worldBossArenas = make(map[uint64]*worldBossArena)
	}
	boss := &entity{Handle: 10, Kind: 3, Def: cfg.Monster.ID, Monster: &cfg.Monster, Pos: cfg.Map.MonsterSpawns[0],
		HP: spec.HPMax, MaxHP: spec.HPMax, MP: cfg.Monster.MP, MaxMP: cfg.Monster.MP, Life: 1}
	w.worldBossArenas[spec.RunID] = &worldBossArena{spec: spec, occurrence: opening.occurrence, config: cfg, challenge: challenge, boss: boss,
		seats: make(map[string]*worldBossSeat), resumable: make(map[string]*worldBossSeat), names: make(map[string]string), nextHandle: 100}
	return 0
}

// persistWorldBossFinish 确认原 actor 冻结的结果。数据库未确认时停止新战斗，不宣布击杀或逃走。
func (w *worker) persistWorldBossFinish(arena *worldBossArena, now time.Time) {
	if arena.result != nil || arena.finishing || arena.finishError != 0 || now.Before(arena.finishRetryAt) {
		return
	}
	proposal, exists := arena.challenge.proposal()
	if !exists {
		return
	}
	finish := persistence.WorldBossFinish{Outcome: persistence.WorldBossOutcome(proposal.kind), FinishedAt: proposal.finishedAt,
		HPRemaining: proposal.hpRemaining, Contributions: make([]persistence.WorldBossContribution, 0, len(proposal.contributions))}
	for _, row := range proposal.contributions {
		finish.Contributions = append(finish.Contributions, persistence.WorldBossContribution{RoleID: row.grant.roleID, DisplayName: arena.names[row.grant.roleID],
			ReservationID: row.grant.reservationID, CombatSnapshot: row.grant.combatSnapshot, JoinedAt: row.joinedAt, Damage: row.damage})
	}
	ledger := w.worldBossLedgers[arena.spec.RunID]
	arena.finishing = w.submit(job{target: w, settlement: true, run: func() func(*worker) {
		result, err := ledger.store.FinishWorldBoss(w.e.ctx, arena.spec, finish)
		return func(_ *worker) {
			arena.finishing = false
			if err == nil {
				arena.result, ledger.latest = result, result
				return
			}
			code := persistence.Code(err)
			if code == p.CodeOperationPending || code == p.CodeServerBusy {
				arena.finishRetryAt = time.Now().Add(250 * time.Millisecond)
				return
			}
			arena.finishError = code
			slog.Error("世界首领终态写入被拒", "run", arena.spec.RunID, "occurrence", arena.spec.OccurrenceID, "code", code)
		}
	}})
}

// drainWorldBossLedger 结清已裁决的命中，正常维护中断尚未完结的活动场。
func (w *worker) drainWorldBossLedger(now time.Time) (pending bool, failed int) {
	for _, ledger := range w.worldBossLedgers {
		w.advanceWorldBossOpening(ledger, now)
		pending = ledger.opening != nil || len(ledger.loading) > 0 || pending
	}
	for _, arena := range w.worldBossArenas {
		if arena.challenge.finish == nil {
			arena.challenge.interrupt(arena.challenge.identity, now)
		}
		w.persistWorldBossFinish(arena, now)
		if arena.finishError != 0 {
			failed++
		} else if arena.result == nil {
			pending = true
		}
	}
	return pending, failed
}

func worldBossResultState(result *persistence.WorldBossResult, scene uint64) *pb.WorldBossState {
	ranks := worldBossResultRanks(result)
	state := &pb.WorldBossState{SceneEpoch: scene, OccurrenceId: result.OccurrenceID, TemplateId: result.TemplateID,
		State: worldBossTerminalPhase(result), OpensAtUnixMs: result.OpensAt.UnixMilli(), ExpiresAtUnixMs: result.ClosesAt.UnixMilli(), Ranks: ranks,
		LatestResult: worldBossResultPB(result, ranks)}
	if result.ProgressKnown {
		state.Hp, state.HpMax = result.HPRemaining, result.HPMax
	}
	return state
}

func (w *worker) latestWorldBoss(runID uint64) *pb.WorldBossResult {
	if ledger := w.worldBossLedgers[runID]; ledger != nil && ledger.latest != nil {
		r := ledger.latest
		return worldBossResultPB(r, worldBossResultRanks(r))
	}
	return nil
}

func worldBossResultPB(result *persistence.WorldBossResult, ranks []*pb.WorldBossRank) *pb.WorldBossResult {
	return &pb.WorldBossResult{OccurrenceId: result.OccurrenceID, TemplateId: result.TemplateID, State: worldBossTerminalPhase(result),
		FinishedAtUnixMs: result.FinishedAt.UnixMilli(), Ranks: ranks}
}

func worldBossTerminalPhase(result *persistence.WorldBossResult) uint32 {
	switch result.Outcome {
	case persistence.WorldBossKilled:
		return worldBossKilled
	case persistence.WorldBossExpired:
		return worldBossExpired
	default:
		return worldBossInterrupted
	}
}

func worldBossResultRanks(result *persistence.WorldBossResult) []*pb.WorldBossRank {
	var ranks []*pb.WorldBossRank
	for _, row := range result.Contributions {
		if row.Damage > 0 {
			ranks = append(ranks, &pb.WorldBossRank{Position: uint32(len(ranks) + 1), DisplayName: row.DisplayName, Damage: row.Damage})
		}
	}
	return ranks
}
