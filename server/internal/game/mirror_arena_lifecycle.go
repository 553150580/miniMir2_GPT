// 本文件在原角色分片集成镜像预约、真实战斗、健康接回和故障收束。
package game

import (
	"log/slog"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// mirrorRun 持有挑战临时现场；连接和普通角色资产仍由原房间管理。
type mirrorRun struct {
	begin        persistence.MirrorBegin      // 首次原输入与进攻快照，写入未确认时原样重投。
	grant        persistence.MirrorChallenge  // 原预约或唯一终态的已确认回执。
	auth         persistence.Auth             // 当前永久预约绑定的已确认会话。
	request      uint32                       // 首次挑战请求号，健康换票后只推新连接可读的状态。
	cfg          *devconfig.MirrorArenaConfig // 首次原场的只读地图和规则。
	scene        uint64                       // 独立观察的场次标识，与普通地图场景代次分开。
	phase        uint32                       // 原运行阶段；零表示正在等待预约确认。
	prepareUntil time.Time                    // 首帧握手截止。
	deadline     time.Time                    // 确认真战后使用的作战截止。
	battle       *mirrorCombat                // 只在原角色分片内推进的双方格子现场。
	outcome      persistence.MirrorOutcome    // 已裁决未持久的原结果。
	finishedAt   time.Time                    // 固定终态事件时刻。
	retryAt      time.Time                    // 提交有界重试时间。
	lastFrame    time.Time                    // 当前观察的快照发送时刻。
	frameSeq     uint64                       // 原场次内单调的观察序号。
	inFlight     bool                         // 预约、接回、结算最多有一个数据库任务在途。
	draining     bool                         // 停服中不建立新现场，既定结果仍提交。
	failed       bool                         // 持久权限或事实校验出错，等待冷接管核对。
}

func (w *worker) beginMirrorArena(r *room, request uint32, offerID, challengeID string, store *persistence.Store, cfg *devconfig.MirrorArenaConfig, now time.Time) {
	if existing := r.mirrorMatch; existing != nil {
		if existing.begin.ChallengeID != challengeID || existing.begin.OfferID != offerID || existing.auth != r.Auth {
			w.sendMirrorError(r, request, p.CodeLineRecovering)
		} else if existing.grant.ChallengeID == "" {
			w.sendMirrorError(r, request, p.CodeOperationPending)
		} else {
			w.sendMirrorRun(r, request, existing)
		}
		return
	}
	if r.Pending || r.PreviewInFlight || r.Player.HP <= 0 {
		w.sendMirrorError(r, request, p.CodeLineRecovering)
		return
	}
	execution := ownerExecutionEpoch.Add(1)
	if execution == 0 {
		w.sendMirrorError(r, request, p.CodeServerBusy)
		return
	}
	run := &mirrorRun{auth: r.Auth, cfg: cfg, scene: execution, request: request,
		begin: persistence.MirrorBegin{ChallengeID: challengeID, OfferID: offerID, Execution: execution,
			ExpectedRevision: r.Role.Revision, AttackerSnapshot: w.freezeMirrorCombatSnapshot(r, &cfg.Map)}}
	r.mirrorMatch = run
	w.submitMirrorBegin(r, run, store, now)
}

func (w *worker) submitMirrorBegin(r *room, run *mirrorRun, store *persistence.Store, now time.Time) {
	auth, begin := run.auth, run.begin
	run.inFlight = w.submit(job{target: w, admission: true, run: func() func(*worker) {
		grant, err := store.BeginMirrorChallenge(w.e.ctx, auth, begin)
		return func(w *worker) {
			if !w.ownsMirrorRun(r, run) {
				return
			}
			run.inFlight = false
			code := persistence.Code(err)
			if code == p.CodeOperationPending || code == p.CodeServerBusy {
				run.retryAt = time.Now().Add(250 * time.Millisecond)
				return
			}
			if err != nil {
				if r.Auth == auth {
					w.sendMirrorError(r, run.request, code)
				}
				r.mirrorMatch = nil
				return
			}
			run.grant, run.scene = grant, grant.Execution
			if grant.Outcome != persistence.MirrorActive {
				w.completeMirrorRun(r, run)
				return
			}
			run.prepareUntil = minBossTime(grant.StartedAt.Add(devconfig.MirrorArenaReadyTimeout), grant.ExpiresAt)
			run.phase = mirrorPreparing
			if run.draining {
				run.finish(persistence.MirrorInterrupted, time.Now())
			} else if auth == r.Auth {
				w.sendMirrorRun(r, run.request, run)
			}
			run.request = 0
		}
	}})
	if !run.inFlight {
		run.retryAt = now.Add(250 * time.Millisecond)
	}
}

// stepMirrorArena 不等客户端确认普通战斗结果，也不占原普通挂机的永久任务槽。
func (w *worker) stepMirrorArena(r *room, now time.Time) {
	run := r.mirrorMatch
	if run == nil || r.Isolated || run.failed || w.e.stopped.Load() {
		return
	}
	w.advanceMirrorRun(r, run, now)
	w.commitMirrorRun(r, run, now)
}

func (w *worker) advanceMirrorRun(r *room, run *mirrorRun, now time.Time) {
	switch run.phase {
	case mirrorPreparing:
		if !now.Before(run.prepareUntil) {
			run.finish(persistence.MirrorInterrupted, run.prepareUntil)
		}
	case mirrorFighting:
		w.advanceMirrorCombat(run.battle, now)
		if result := run.battle.result; result.Cause != 0 {
			outcome := persistence.MirrorInterrupted
			switch result.Winner {
			case 1:
				outcome = persistence.MirrorWon
			case 2:
				outcome = persistence.MirrorLost
			}
			run.finish(outcome, result.FinishedAt)
			w.sendMirrorRun(r, 0, run)
		}
		if run.lastFrame.IsZero() || now.Sub(run.lastFrame) >= 100*time.Millisecond || run.phase == mirrorSettling {
			w.sendMirrorCombatFrame(r, run, now)
		}
	}
}

func (w *worker) commitMirrorRun(r *room, run *mirrorRun, now time.Time) {
	if run.inFlight || run.failed || now.Before(run.retryAt) {
		return
	}
	store := w.roomStore(r)
	switch {
	case run.grant.ChallengeID == "":
		w.submitMirrorBegin(r, run, store, now)
	case run.auth != r.Auth:
		w.resumeMirrorArena(r, run, store, now)
	case run.phase == mirrorSettling:
		w.finishMirrorArena(r, run, store, now)
	}
}

// resumeMirrorArena 保留原场次和双方临时身体，只更新预约确认的角色会话。
func (w *worker) resumeMirrorArena(r *room, run *mirrorRun, store *persistence.Store, now time.Time) {
	auth := r.Auth
	proof := persistence.MirrorResume{ChallengeID: run.grant.ChallengeID, ReservationID: run.grant.ReservationID,
		Execution: run.grant.Execution, PreviousSessionEpoch: run.auth.SessionEpoch}
	run.inFlight = w.submit(job{target: w, admission: true, run: func() func(*worker) {
		grant, err := store.ResumeMirrorChallenge(w.e.ctx, auth, proof)
		return func(w *worker) {
			if !w.ownsMirrorRun(r, run) {
				return
			}
			run.inFlight = false
			if err != nil {
				w.retryMirrorRun(r, run, auth, err)
				return
			}
			run.auth = auth
			grant.AttackerSnapshot, grant.DefenseSnapshot = run.grant.AttackerSnapshot, run.grant.DefenseSnapshot
			run.grant = grant
			if grant.Outcome != persistence.MirrorActive {
				w.completeMirrorRun(r, run)
				return
			}
			w.sendMirrorRun(r, 0, run)
		}
	}})
	if !run.inFlight {
		run.retryAt = now.Add(250 * time.Millisecond)
	}
}

func (w *worker) finishMirrorArena(r *room, run *mirrorRun, store *persistence.Store, now time.Time) {
	auth := run.auth
	proof := persistence.MirrorFinish{ChallengeID: run.grant.ChallengeID, ReservationID: run.grant.ReservationID,
		Execution: run.grant.Execution, Outcome: run.outcome, FinishedAt: run.finishedAt}
	run.inFlight = w.submit(job{target: w, settlement: true, run: func() func(*worker) {
		grant, err := store.FinishMirrorChallenge(w.e.ctx, auth, proof)
		return func(w *worker) {
			if !w.ownsMirrorRun(r, run) {
				return
			}
			run.inFlight = false
			if err != nil {
				w.retryMirrorRun(r, run, auth, err)
				return
			}
			run.grant = grant
			w.completeMirrorRun(r, run)
		}
	}})
	if !run.inFlight {
		run.retryAt = now.Add(250 * time.Millisecond)
	}
}

func (w *worker) retryMirrorRun(r *room, run *mirrorRun, auth persistence.Auth, err error) {
	code := persistence.Code(err)
	if r.Auth != auth || code == p.CodeOperationPending || code == p.CodeServerBusy {
		run.retryAt = time.Now().Add(250 * time.Millisecond)
		return
	}
	// 原结果未确认时保持独立退场状态。新权威进程仍可沿原预约做中断处理。
	run.failed = true
	w.sendMirrorError(r, 0, code)
	slog.Error("镜像竞技原场次无法确认", "challenge", run.begin.ChallengeID, "code", code)
}

func (w *worker) completeMirrorRun(r *room, run *mirrorRun) {
	run.phase = mirrorFinished
	if run.auth != r.Auth {
		// 当前房间已换为同一角色的新票。旧请求号不可跨绑定发送，但已知原终态可观察。
		run.auth, run.request = r.Auth, 0
	}
	w.sendMirrorRun(r, run.request, run)
	r.mirrorMatch = nil
}

func (w *worker) ownsMirrorRun(r *room, run *mirrorRun) bool {
	return w.rooms[r.Role.ID] == r && r.mirrorMatch == run && !r.Isolated
}

func (run *mirrorRun) finish(outcome persistence.MirrorOutcome, at time.Time) {
	if run.phase != mirrorPreparing && run.phase != mirrorFighting {
		return
	}
	run.phase, run.outcome, run.finishedAt = mirrorSettling, outcome, minBossTime(at, run.grant.ExpiresAt)
}

// drainMirrorArena 中断仍活动的原场，已经裁决的胜负沿原终态收束。
func (w *worker) drainMirrorArena(r *room, now time.Time) (pending bool, failed bool) {
	run := r.mirrorMatch
	if run == nil {
		return false, false
	}
	if run.failed || r.Isolated {
		return false, true
	}
	run.draining = true
	if run.phase == mirrorFighting && !now.Before(run.deadline) {
		w.advanceMirrorRun(r, run, run.deadline)
	}
	run.finish(persistence.MirrorInterrupted, now)
	w.commitMirrorRun(r, run, now)
	return true, false
}
