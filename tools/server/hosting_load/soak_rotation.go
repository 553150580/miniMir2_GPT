package main

import (
	"context"
	"fmt"
	"time"
)

type soakRotation struct {
	firstChurn, firstReconnect, roomSlots, rolesSeeded int
	churnCursor, reconnectCursor, probeCursor          int
	nextIdentity                                       int
	hostRoster                                         []loadBotSnapshot
	hostCheckedAt                                      []time.Time
}

func newSoakRotation(firstChurn, firstReconnect, roomSlots, rolesSeeded int, hostRoster []loadBotSnapshot) *soakRotation {
	checked := make([]time.Time, len(hostRoster))
	for i := range checked {
		checked[i] = time.Now()
	}
	return &soakRotation{
		firstChurn: firstChurn, firstReconnect: firstReconnect, roomSlots: roomSlots, rolesSeeded: rolesSeeded, nextIdentity: roomSlots,
		hostRoster: hostRoster, hostCheckedAt: checked,
	}
}

type soakReconnectResult struct {
	PreviouslyOnline int `json:"previously_online"`
	SameScene        int `json:"same_scene"`
	SameLife         int `json:"same_life"`
	SamePosition     int `json:"same_position"`
	NewBinding       int `json:"new_binding"`
	Restored         int `json:"restored"`
	HostingDebited   int `json:"hosting_debited"`
}

func (m *soakReconnectResult) record(before, after loadBotSnapshot, hosted bool) {
	if before.Online || hosted {
		m.PreviouslyOnline++
	}
	if !after.Online {
		return
	}
	scene := before.Scene != 0 && before.Scene == after.Scene
	// 初始生命代次为0，匹配完整首基线后该值同样有效。
	life := before.Ready && after.Ready && before.Life == after.Life
	position := before.X == after.X && before.Y == after.Y
	binding := before.Binding != 0 && after.Binding > before.Binding
	debited := after.HostingRemaining < before.HostingRemaining
	if scene {
		m.SameScene++
	}
	if life {
		m.SameLife++
	}
	if position {
		m.SamePosition++
	}
	if binding {
		m.NewBinding++
	}
	if debited {
		m.HostingDebited++
	}
	if scene && life && position && binding && (!hosted || debited) {
		m.Restored++
	}
}

func (r *soakRotation) cycle(ctx context.Context, state *soakObservers, join func(int) (*loadBot, error),
	a arguments, plan soakPlan, started time.Time, cycle int) error {
	if r.nextIdentity+plan.replacements > r.rolesSeeded {
		return fmt.Errorf("换人组预置角色已用完")
	}
	churnSlots := make([]int, plan.replacements)
	for i := range churnSlots {
		churnSlots[i] = r.firstChurn + r.churnCursor%(r.firstReconnect-r.firstChurn)
		r.churnCursor++
		state.detach(churnSlots[i])
	}
	firstIdentity := r.nextIdentity
	newcomers := make([]*loadBot, len(churnSlots))
	replacement := admit(ctx, newcomers, len(newcomers), a.parallel, func(i int) (*loadBot, error) {
		return join(firstIdentity + i)
	})
	r.nextIdentity += len(newcomers)
	for i, slot := range churnSlots {
		state.identities[slot] = firstIdentity + i
		state.bots[slot] = newcomers[i]
	}

	reconnectSlots := make([]int, plan.reconnects)
	previous := make([]loadBotSnapshot, plan.reconnects)
	for i := range reconnectSlots {
		reconnectSlots[i] = r.firstReconnect + r.reconnectCursor%(r.roomSlots-r.firstReconnect)
		r.reconnectCursor++
		previous[i] = state.detach(reconnectSlots[i])
	}
	// 留出实际连接断开区间，再通过同角色新票进行接管。
	if err := sleepContext(ctx, time.Second); err != nil {
		return err
	}
	rejoined := make([]*loadBot, len(reconnectSlots))
	reconnection := admit(ctx, rejoined, len(rejoined), min(a.parallel, len(rejoined)), func(i int) (*loadBot, error) {
		return join(state.identities[reconnectSlots[i]])
	})
	var observed soakReconnectResult
	for i, slot := range reconnectSlots {
		state.bots[slot] = rejoined[i]
		var after loadBotSnapshot
		if rejoined[i] != nil {
			after = rejoined[i].snapshot()
		}
		observed.record(previous[i], after, false)
	}

	result := map[string]any{
		"event": "soak_operations", "target": a.seed, "elapsed_seconds": time.Since(started).Seconds(), "cycle": cycle,
		"replacement_admission": replacement, "reconnect_admission": reconnection, "reconnect_observation": observed,
	}
	if cycle%4 == 0 && time.Since(started) >= time.Minute {
		count := max(1, plan.offline/100)
		admission, verification, waited := r.probeHosting(ctx, state, join, a.parallel, count)
		result["hosted_probe_admission"] = admission
		result["hosted_probe_observation"] = verification
		result["hosted_probe_minimum_offline_seconds"] = waited.Seconds()
	}
	return emit(result)
}

func (r *soakRotation) probeHosting(ctx context.Context, state *soakObservers, join func(int) (*loadBot, error), parallel, count int) (
	admissionResult, soakReconnectResult, time.Duration,
) {
	slots := make([]int, count)
	waited := time.Duration(0)
	for i := range slots {
		slots[i] = r.probeCursor % len(r.hostRoster)
		r.probeCursor++
		since := time.Since(r.hostCheckedAt[slots[i]])
		if i == 0 || since < waited {
			waited = since
		}
	}
	probes := make([]*loadBot, count)
	admission := admit(ctx, probes, len(probes), min(parallel, count), func(i int) (*loadBot, error) {
		return join(state.identities[slots[i]])
	})
	var observed soakReconnectResult
	for i, slot := range slots {
		state.bots[slot] = probes[i]
		after := state.detach(slot)
		if after.Online {
			r.hostCheckedAt[slot] = time.Now()
		}
		observed.record(r.hostRoster[slot], after, true)
		if after.Online {
			r.hostRoster[slot] = after
		}
	}
	return admission, observed, waited
}
