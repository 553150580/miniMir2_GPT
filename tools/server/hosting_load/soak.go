package main

import (
	"context"
	"fmt"
	"time"

	"minimir/internal/persistence"
)

const (
	soakInterval       = 15 * time.Second
	soakHostingBalance = int64(21600)
)

type soakPlan struct {
	offline, online, replacements, reconnects, seeded int
}

func planSoak(a arguments) soakPlan {
	offline := max(1, a.seed/10)
	replacements := max(2, a.seed/100)
	// 仅在保压窗口结束前轮换，向上取整额外预留最后一批。
	batches := int((a.fullHold + soakInterval - 1) / soakInterval)
	return soakPlan{
		offline: offline, online: a.seed - offline, replacements: replacements,
		reconnects: max(1, a.seed/500), seeded: a.seed + batches*replacements,
	}
}

type soakObservers struct {
	bots           []*loadBot
	identities     []int
	retired        census
	attacking      map[int]bool
	settling       map[int]bool
	moved          map[int]bool
	distinct       map[int]bool
	baseDeaths     uint64
	baseDropped    uint64
	baseAttacks    uint64
	baseSettled    uint64
	minOnline      int
	minAutoAlive   int
	minTotalActive int
}

func newSoakObservers(bots []*loadBot) *soakObservers {
	ids := make([]int, len(bots))
	for i := range ids {
		ids[i] = i
	}
	return &soakObservers{
		bots: bots, identities: ids, retired: census{ErrorCodes: make(map[uint32]uint64)},
		attacking: make(map[int]bool), settling: make(map[int]bool), moved: make(map[int]bool), distinct: make(map[int]bool),
	}
}

func (o *soakObservers) observe(i int, b loadBotSnapshot) {
	id := o.identities[i]
	o.distinct[id] = true
	if b.Attack+b.Cast > 0 {
		o.attacking[id] = true
	}
	if b.Settlement > 0 {
		o.settling[id] = true
	}
	if b.ChangedPosition > 0 {
		o.moved[id] = true
	}
}

func (o *soakObservers) detach(i int) loadBotSnapshot {
	if o.bots[i] == nil {
		return loadBotSnapshot{}
	}
	before := o.bots[i].snapshot()
	o.bots[i].close()
	snapshot := o.bots[i].snapshot()
	snapshot.Online = before.Online
	o.observe(i, snapshot)
	o.retired.Attacks += snapshot.Attack + snapshot.Cast
	o.retired.Settlements += snapshot.Settlement
	o.retired.Deaths += snapshot.Death
	o.retired.Disconnects += snapshot.CloseCount
	o.retired.Errors += snapshot.Errors
	o.retired.Heartbeats += snapshot.HeartbeatOK
	for code, count := range snapshot.ErrorCodes {
		o.retired.ErrorCodes[code] += count
	}
	if snapshot.CloseReason != "" && len(o.retired.CloseSamples) < 5 {
		o.retired.CloseSamples = append(o.retired.CloseSamples, snapshot.CloseReason)
	}
	o.bots[i] = nil
	return snapshot
}

func (o *soakObservers) snapshot() census {
	v, rows := countBots(o.bots)
	for i, b := range rows {
		if o.bots[i] != nil {
			o.observe(i, b)
		}
	}
	v.Entered = len(o.distinct)
	v.Attacking, v.Settling, v.ChangedPosition = len(o.attacking), len(o.settling), len(o.moved)
	v.Attacks += o.retired.Attacks
	v.Settlements += o.retired.Settlements
	v.Deaths += o.retired.Deaths
	v.Disconnects += o.retired.Disconnects
	v.Errors += o.retired.Errors
	v.Heartbeats += o.retired.Heartbeats
	for code, count := range o.retired.ErrorCodes {
		v.ErrorCodes[code] += count
	}
	for _, reason := range o.retired.CloseSamples {
		if len(v.CloseSamples) < 5 {
			v.CloseSamples = append(v.CloseSamples, reason)
		}
	}
	return v
}

func runSoak(ctx context.Context, store *persistence.Store, statsAddr string, a arguments, roles []loadIdentity, bots []*loadBot, join func(int) (*loadBot, error)) error {
	plan := planSoak(a)
	if err := emit(map[string]any{
		"event": "soak_cohorts", "target": a.seed, "online_target": plan.online, "offline_target": plan.offline,
		"replacement_batch": plan.replacements, "reconnect_batch": plan.reconnects,
		"interval_seconds": soakInterval.Seconds(), "roles_seeded": plan.seeded,
	}); err != nil {
		return err
	}
	var start generalDiagnostics
	if err := statsRequest(ctx, statsAddr, fmt.Sprintf("/phase?name=ramp-%d", a.seed), &start); err != nil {
		return err
	}
	joinStarted := time.Now()
	admission := admit(ctx, bots, a.seed, a.parallel, join)
	state := newSoakObservers(bots)
	initial := state.snapshot()
	if err := emit(map[string]any{"event": "admission", "target": a.seed, "seconds": time.Since(joinStarted).Seconds(),
		"attempt_errors": admission, "current": initial}); err != nil {
		return err
	}
	if initial.AutoAlive != a.seed {
		return fmt.Errorf("初始自动挂机只有 %d 个角色，目标 %d", initial.AutoAlive, a.seed)
	}
	if err := sleepContext(ctx, 15*time.Second); err != nil {
		return err
	}

	hostRoster := make([]loadBotSnapshot, plan.offline)
	for i := range hostRoster {
		hostRoster[i] = state.detach(i)
		if !hostRoster[i].Auto || hostRoster[i].HostingRemaining != uint32(soakHostingBalance) {
			return fmt.Errorf("托管组角色 %d 未开启自动战斗或余额不符", i)
		}
	}
	// 等待断开传到游戏服，稳态开始前不把脱离中的连接算作离线托管。
	if err := sleepContext(ctx, 3*time.Second); err != nil {
		return err
	}
	ledger := newSoakHostingLedger(roles[:plan.offline])
	if err := statsRequest(ctx, statsAddr, fmt.Sprintf("/phase?name=hold-%d", a.seed), &start); err != nil {
		return err
	}
	var startInterval generalDiagnostics
	if err := statsRequest(ctx, statsAddr, "/soak", &startInterval); err != nil {
		return err
	}
	started := time.Now()
	base, hosting, err := soakSample(ctx, store, state, ledger)
	if err != nil {
		return err
	}
	state.minOnline, state.minAutoAlive = base.Online, base.AutoAlive
	state.minTotalActive = startInterval.Rooms.OnlineAutoAlive + startInterval.Rooms.Managed
	state.baseAttacks, state.baseSettled, state.baseDeaths, state.baseDropped = base.Attacks, base.Settlements, base.Deaths, base.Disconnects
	if err = emit(map[string]any{"event": "hold_start", "target": a.seed, "census": base, "hosting": hosting, "server": start}); err != nil {
		return err
	}
	cycles := 0
	// 轮换使用前半个在线队列；另半队列专司同一角色的健康重连。
	split := plan.offline + plan.online/2
	rotation := newSoakRotation(plan.offline, split, len(bots), len(roles), hostRoster)
	for cycleAt := soakInterval; cycleAt < a.fullHold; cycleAt += soakInterval {
		if err = sleepContext(ctx, max(0, cycleAt-time.Since(started))); err != nil {
			return err
		}
		if time.Since(started) >= a.fullHold {
			break
		}
		cycles++
		// 在下一批操作前取样，避免把上一秒尚未同步的新连接当作稳态缺员。
		if cycles%2 == 0 {
			if err = emitSoakSample(ctx, statsAddr, store, state, ledger, a.seed, started); err != nil {
				return err
			}
		}
		if err = rotation.cycle(ctx, state, join, a, plan, started, cycles); err != nil {
			return err
		}
	}
	if err = sleepContext(ctx, max(0, a.fullHold-time.Since(started))); err != nil {
		return err
	}

	if err = emitSoakSample(ctx, statsAddr, store, state, ledger, a.seed, started); err != nil {
		return err
	}
	var finish generalDiagnostics
	if err = statsRequest(ctx, statsAddr, fmt.Sprintf("/phase?name=after-%d", a.seed), &finish); err != nil {
		return err
	}
	final := state.snapshot()
	if err = emit(map[string]any{"event": "hold_end", "target": a.seed, "elapsed_seconds": time.Since(started).Seconds(),
		"minimum_online": state.minOnline, "minimum_auto_alive": state.minAutoAlive, "minimum_active": state.minTotalActive,
		"attacking_roles": final.Attacking, "settling_roles": final.Settling, "new_attacks": final.Attacks - state.baseAttacks,
		"new_settlements": final.Settlements - state.baseSettled, "new_disconnects": final.Disconnects - state.baseDropped,
		"new_deaths": final.Deaths - state.baseDeaths, "census": final, "hosting": ledger.latest, "server": finish, "cycles": cycles,
	}); err != nil {
		return err
	}
	// 长测停止制造新闲置房间，保留原有在线和托管压力，观察最近一批自然超期。
	if a.fullHold >= 5*time.Minute {
		const drainPeriod = 66 * time.Second
		drainStart := time.Now()
		if err = sleepContext(ctx, drainPeriod); err != nil {
			return err
		}
		var drained generalDiagnostics
		if err = statsRequest(ctx, statsAddr, "/soak", &drained); err != nil {
			return err
		}
		settled, ledgerAfter, sampleErr := soakSample(ctx, store, state, ledger)
		if sampleErr != nil {
			return sampleErr
		}
		return emit(map[string]any{"event": "soak_drain", "target": a.seed, "seconds": time.Since(drainStart).Seconds(),
			"elapsed_seconds": time.Since(started).Seconds(), "census": settled, "hosting": ledgerAfter, "server": drained})
	}
	return nil
}

func soakSample(ctx context.Context, store *persistence.Store, state *soakObservers, ledger *soakHostingLedger) (census, soakHostingSample, error) {
	v := state.snapshot()
	hosting, err := ledger.sample(ctx, store)
	return v, hosting, err
}

func emitSoakSample(ctx context.Context, addr string, store *persistence.Store, state *soakObservers,
	ledger *soakHostingLedger, target int, started time.Time) error {
	var diagnostic generalDiagnostics
	if err := statsRequest(ctx, addr, "/soak", &diagnostic); err != nil {
		return err
	}
	v, hosting, err := soakSample(ctx, store, state, ledger)
	if err != nil {
		return err
	}
	active := diagnostic.Rooms.OnlineAutoAlive + diagnostic.Rooms.Managed
	state.minOnline = min(state.minOnline, v.Online)
	state.minAutoAlive = min(state.minAutoAlive, v.AutoAlive)
	state.minTotalActive = min(state.minTotalActive, active)
	return emit(map[string]any{"event": "soak_sample", "target": target, "elapsed_seconds": time.Since(started).Seconds(),
		"active_online": diagnostic.Rooms.OnlineAutoAlive, "active_managed": diagnostic.Rooms.Managed, "active_total": active,
		"census": v, "hosting": hosting, "server": diagnostic})
}
