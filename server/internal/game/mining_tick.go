// 本文件用矿区时钟冻结一次挖矿事件，沿原来源流水确认工具耗量、离线扣时和矿石。
package game

import (
	"math"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// stepMining 独占矿区动作时间。离线时只为具备工具和余额的完整判定扣费。
func (w *worker) stepMining(r *room, now time.Time) {
	state := &r.mining
	f := &w.roomStore(r).Cfg.Fixture
	layer, allowed := f.MiningLayer(r.MapID)
	if !allowed {
		w.isolateRoom(r)
		return
	}
	if !state.active {
		if r.Conn != nil && r.Conn.Alive() && r.Ready {
			w.stepGridCombat(r, &layer.Map, now, false)
		}
		return
	}
	clearFinishedAction(&r.Player, now)
	if r.Conn != nil && (!r.Ready || !r.Conn.Alive()) {
		return
	}
	if r.Player.HP <= 0 || r.Role.Level < layer.MinimumLevel || r.Role.SourceSeq == math.MaxInt64 ||
		state.attempts == math.MaxUint64 || state.successes == math.MaxUint64 {
		w.stopMining(r, p.CodeLineForbidden)
		return
	}
	tool, code := miningToolAvailable(r.Role, f, r.MapID, state.toolID)
	if code != 0 {
		w.stopMining(r, code)
		return
	}
	interval := time.Duration(f.Mining.IntervalMS) * time.Millisecond
	var session string
	var seconds int64
	if r.Conn == nil {
		seconds = f.Mining.IntervalMS / 1000
		if r.HostSession == "" || r.Role.HostingSeconds < seconds {
			w.stopMining(r, p.CodeVendorFunds)
			return
		}
		session = r.HostSession
	}
	// 健康重连后从已确认的新附着起算；离线换边也重新计满一轮。
	if state.startedAt.IsZero() {
		state.startedAt = now
		return
	}
	if now.Before(state.startedAt.Add(interval)) {
		return
	}
	q := persistence.MiningAttemptRequest{EventID: domain.ID(), SourceSeq: r.Role.SourceSeq + 1,
		ExpectedRevision: r.Role.Revision, RuleVersion: f.RuleVersion, ConfigHash: f.Hash, MapID: r.MapID,
		ToolID: tool.ID, ToolVersion: tool.Version, StartedAt: state.startedAt, At: now,
		HostingSession: session, HostSeconds: seconds}
	w.submitMining(r, q)
}

// stopMining 停止新增轮次，已提交的事实仍由原回执收尾。
func (w *worker) stopMining(r *room, code uint32) {
	r.mining.active, r.mining.startedAt, r.mining.code = false, time.Time{}, code
	w.sendMiningState(r, 0, code)
}

// submitMining 冻结原场景、账号与一次事件。队列满或提交待确认都保留同一事件编号。
func (w *worker) submitMining(r *room, q persistence.MiningAttemptRequest) {
	store, auth, scene := w.roomStore(r), r.Auth, r.Scene
	r.Pending = true
	var work job
	work = job{target: w, settlement: true, run: func() func(*worker) {
		role, receipt, err := store.ResolveMiningAttempt(w.e.ctx, auth, q)
		return func(owner *worker) {
			if owner.rooms[auth.RoleID] != r || r.Isolated || !r.Pending || r.Scene != scene || r.Auth != auth {
				return
			}
			owner.finishMining(r, q, role, receipt, err, &work, time.Now())
		}
	}}
	if !w.submit(work) {
		r.PendingRetry, r.RetryAt = &work, time.Now().Add(100*time.Millisecond)
	}
}

// finishMining 投影原持久结果。当前背包、工具、时长和回执都使用同一次提交。
func (w *worker) finishMining(r *room, q persistence.MiningAttemptRequest, role domain.Role, receipt persistence.Operation, err error, retry *job, now time.Time) {
	code := persistence.Code(err)
	if code == p.CodeOperationPending || code == p.CodeServerBusy {
		r.PendingRetry, r.RetryAt = retry, now.Add(500*time.Millisecond)
		return
	}
	r.PendingRetry = nil
	r.resumeAfterPending(now)
	if hardAuthorityFailure(code) {
		w.isolateRoom(r)
		return
	}
	if err != nil {
		w.stopMining(r, code)
		w.after(r)
		return
	}
	result := receipt.Mining
	if result == nil || result.EventID != q.EventID || result.MapID != q.MapID || result.ToolID != q.ToolID ||
		receipt.RoleID != r.Role.ID || receipt.SourceSeq != q.SourceSeq || role.SourceSeq != q.SourceSeq || role.Revision != receipt.Revision {
		w.isolateRoom(r)
		return
	}
	r.Role = role
	state := &r.mining
	state.attempts++
	if result.Ore != nil {
		state.successes++
		state.lastOre = result.Ore
	}
	// 每次确认后才开始下一轮，等待事务的时间不会被补跑。
	state.startedAt, state.code = time.Time{}, 0
	if state.active {
		switch {
		case result.ToolExhausted:
			state.active, state.code = false, p.CodeEquipmentChanged
		case len(role.Items) >= role.Capacity:
			state.active, state.code = false, p.CodeEquipmentBagFull
		case result.HostSeconds > 0 && role.HostingSeconds < result.HostSeconds:
			state.active, state.code = false, p.CodeVendorFunds
		}
	}
	w.assets(r)
	w.sendMiningState(r, 0, state.code)
	w.after(r)
}

// miningMap 表示配置正式登记在独立矿区目录中的小地图。
func miningMap(f *devconfig.Fixture, id uint64) bool {
	_, ok := f.MiningLayer(id)
	return ok
}
