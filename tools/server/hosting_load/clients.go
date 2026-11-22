package main

import (
	"context"
	"errors"
	"fmt"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	"strings"
	"sync"
	"time"
)

type census struct {
	Entered         int               `json:"entered"`
	Online          int               `json:"online"`
	AutoAlive       int               `json:"auto_alive"`
	DistinctScenes  int               `json:"distinct_scenes"`
	Attacking       int               `json:"ever_attacked"`
	Settling        int               `json:"ever_settled"`
	ChangedPosition int               `json:"changed_position"`
	Attacks         uint64            `json:"attacks"`
	Settlements     uint64            `json:"settlements"`
	Deaths          uint64            `json:"deaths"`
	Disconnects     uint64            `json:"disconnects"`
	Errors          uint64            `json:"errors"`
	Heartbeats      uint64            `json:"heartbeats"`
	CloseSamples    []string          `json:"close_samples,omitempty"`
	ErrorCodes      map[uint32]uint64 `json:"error_codes,omitempty"`
}

func countBots(bots []*loadBot) (census, []loadBotSnapshot) {
	rows := make([]loadBotSnapshot, len(bots))
	count := census{ErrorCodes: make(map[uint32]uint64)}
	scenes := make(map[uint64]bool)
	for i, bot := range bots {
		if bot == nil {
			continue
		}
		b := bot.snapshot()
		rows[i] = b
		count.Entered++
		if b.Online {
			count.Online++
			scenes[b.Scene] = true
		}
		if b.Online && b.Auto && !b.Dead && b.Ready {
			count.AutoAlive++
		}
		if b.Attack+b.Cast > 0 {
			count.Attacking++
		}
		if b.Settlement > 0 {
			count.Settling++
		}
		if b.ChangedPosition > 0 {
			count.ChangedPosition++
		}
		count.Attacks += b.Attack + b.Cast
		count.Settlements += b.Settlement
		count.Deaths += b.Death
		count.Disconnects += b.CloseCount
		count.Errors += b.Errors
		count.Heartbeats += b.HeartbeatOK
		if b.CloseReason != "" && len(count.CloseSamples) < 5 {
			count.CloseSamples = append(count.CloseSamples, b.CloseReason)
		}
		for code, frequency := range b.ErrorCodes {
			count.ErrorCodes[code] += frequency
		}
	}
	count.DistinctScenes = len(scenes)
	return count, rows
}

type loadStage struct {
	target int
	warmup time.Duration
	hold   time.Duration
}

func runClients(ctx context.Context, cfg devconfig.Config, statsAddr string, a arguments) error {
	if a.parallel < 1 || a.parallel > 48 || a.seed < 129 || a.seed > 5000 || a.fullHold < 10*time.Second {
		return errors.New("压测参数超出有效范围")
	}
	store, err := persistence.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer store.Close(context.Background())
	expected := a.seed
	if a.profile == "default" {
		expected = 129
	}
	seeded := expected
	if a.profile == "soak" {
		seeded = planSoak(a).seeded
	}
	roles, err := loadIdentities(ctx, store, seeded)
	if err != nil {
		return err
	}
	bots := make([]*loadBot, expected)
	defer func() {
		for _, b := range bots {
			if b != nil {
				b.close()
			}
		}
	}()
	join := issueLoadBot(ctx, cfg, store, roles, a.profile != "prejoin")
	if a.profile == "soak" {
		return runSoak(ctx, store, statsAddr, a, roles, bots, join)
	}
	if a.profile == "default" {
		entry := admit(ctx, bots, 128, a.parallel, join)
		before, _ := countBots(bots)
		b, err := join(128)
		if b != nil {
			bots[128] = b
		}
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		result := map[string]any{"event": "default_admission", "first_128": before, "attempt_errors": entry, "connection_129": errText, "confirmed": before.Online == 128 && strings.Contains(errText, "HTTP 503")}
		return emit(result)
	}

	stages := []loadStage{
		{100, 15 * time.Second, 20 * time.Second},
		{1000, 12 * time.Second, 30 * time.Second},
		{2500, 12 * time.Second, 40 * time.Second},
		{expected, 15 * time.Second, a.fullHold},
	}
	if a.profile == "prejoin" {
		// 先完成角色入场，再打开自动战斗，将同时登录和稳态挂机分别观察。
		stages = []loadStage{{expected, 15 * time.Second, a.fullHold}}
	}
	for i, stage := range stages {
		if stage.target > expected || stage.target == expected && i < len(stages)-1 {
			continue
		}
		phase := fmt.Sprintf("hold-%d", stage.target)
		var start generalDiagnostics
		if err = statsRequest(ctx, statsAddr, fmt.Sprintf("/phase?name=ramp-%d", stage.target), &start); err != nil {
			return err
		}
		joinStart := time.Now()
		entry := admit(ctx, bots, stage.target, a.parallel, join)
		initial, _ := countBots(bots)
		if err = emit(map[string]any{"event": "admission", "target": stage.target, "seconds": time.Since(joinStart).Seconds(), "attempt_errors": entry, "current": initial}); err != nil {
			return err
		}
		if a.profile == "prejoin" {
			if initial.Online != expected {
				return fmt.Errorf("预接入 %d 个在线角色，低于目标 %d", initial.Online, expected)
			}
			failures := enableStaggered(ctx, bots)
			active, _ := countBots(bots)
			if err = emit(map[string]any{"event": "auto_activation", "target": stage.target, "census": active, "attempt_errors": failures}); err != nil {
				return err
			}
		}
		if err = sleepContext(ctx, stage.warmup); err != nil {
			return err
		}
		base, previous := countBots(bots)
		if err = statsRequest(ctx, statsAddr, "/phase?name="+phase, &start); err != nil {
			return err
		}
		started := time.Now()
		if err = emit(map[string]any{"event": "hold_start", "target": stage.target, "census": base, "server": start}); err != nil {
			return err
		}
		minOnline, minAlive := base.Online, base.AutoAlive
		for time.Since(started) < stage.hold {
			if err = sleepContext(ctx, min(10*time.Second, stage.hold-time.Since(started))); err != nil {
				return err
			}
			current, _ := countBots(bots)
			minOnline = min(minOnline, current.Online)
			minAlive = min(minAlive, current.AutoAlive)
			if err = emit(map[string]any{"event": "hold_sample", "target": stage.target, "elapsed_seconds": time.Since(started).Seconds(), "census": current}); err != nil {
				return err
			}
		}
		var finish generalDiagnostics
		if err = statsRequest(ctx, statsAddr, fmt.Sprintf("/phase?name=after-%d", stage.target), &finish); err != nil {
			return err
		}
		current, rows := countBots(bots)
		attackers, settlers := 0, 0
		for j, row := range rows {
			earlier := previous[j]
			if row.Attack+row.Cast > earlier.Attack+earlier.Cast {
				attackers++
			}
			if row.Settlement > earlier.Settlement {
				settlers++
			}
		}
		if err = emit(map[string]any{"event": "hold_end", "target": stage.target, "elapsed_seconds": time.Since(started).Seconds(),
			"minimum_online": min(minOnline, current.Online), "minimum_auto_alive": min(minAlive, current.AutoAlive), "attacking_roles": attackers,
			"settling_roles": settlers, "new_attacks": current.Attacks - base.Attacks, "new_settlements": current.Settlements - base.Settlements,
			"new_disconnects": current.Disconnects - base.Disconnects, "new_deaths": current.Deaths - base.Deaths, "census": current, "server": finish}); err != nil {
			return err
		}
	}
	return nil
}

func issueLoadBot(ctx context.Context, cfg devconfig.Config, store *persistence.Store, roles []loadIdentity, activate bool) func(int) (*loadBot, error) {
	return func(i int) (*loadBot, error) {
		q, stop := context.WithTimeout(ctx, 10*time.Second)
		ticket, err := store.IssueTicket(q, persistence.Auth{AccountID: roles[i].AccountID, AuthVersion: 1},
			&pb.RoleKey{RoleId: domain.Bytes(roles[i].ID), OriginServerId: cfg.OriginID})
		stop()
		if err != nil {
			return nil, loadTicketError{err}
		}
		return joinLoadBot(ctx, cfg.GameAddress, ticket.Ticket, activate)
	}
}

func enableStaggered(ctx context.Context, bots []*loadBot) admissionResult {
	var lock sync.Mutex
	var workers sync.WaitGroup
	var failures admissionResult
	// 将自动战斗指令均摊到25秒，避免所有房间同一帧开始战斗。
	for i, bot := range bots {
		if i != 0 && i%20 == 0 && sleepContext(ctx, 100*time.Millisecond) != nil {
			break
		}
		workers.Go(func() {
			attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := bot.enableAuto(attempt); err != nil {
				lock.Lock()
				failures.Failures++
				if len(failures.Samples) < 12 {
					failures.Samples = append(failures.Samples, err.Error())
				}
				lock.Unlock()
			}
		})
	}
	workers.Wait()
	return failures
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type admissionResult struct {
	Roles             int                       `json:"roles_attempted"`
	FirstSuccess      int                       `json:"first_success"`
	Success           int                       `json:"success"`
	Failures          int                       `json:"failures"`
	Recovered         int                       `json:"recovered"`
	TicketFailures    int                       `json:"ticket_failures"`
	TicketTimeouts    int                       `json:"ticket_timeouts"`
	TicketWriteErrors int                       `json:"ticket_write_concern_errors"`
	Confirmation      hostingLoadDurationReport `json:"confirmation_latency"`
	Samples           []string                  `json:"samples,omitempty"`
}

type loadTicketError struct{ err error }

func (e loadTicketError) Error() string { return e.err.Error() }
func (e loadTicketError) Unwrap() error { return e.err }

func admit(ctx context.Context, bots []*loadBot, goal, parallel int, join func(int) (*loadBot, error)) admissionResult {
	var workers sync.WaitGroup
	positions := make(chan int)
	var lock sync.Mutex
	result := admissionResult{}
	timings := make([]time.Duration, 0, goal)
	for range parallel {
		workers.Go(func() {
			for i := range positions {
				started := time.Now()
				for attempt := 0; attempt < 3; attempt++ {
					bot, err := join(i)
					if err == nil {
						bots[i] = bot
						lock.Lock()
						timings = append(timings, time.Since(started))
						if attempt > 0 {
							result.Recovered++
						}
						lock.Unlock()
						break
					}
					lock.Lock()
					result.Failures++
					var ticket loadTicketError
					if errors.As(err, &ticket) {
						result.TicketFailures++
						failure := classifyHostingLoadMongoError(ticket.err)
						result.TicketTimeouts += failure.Timeout
						result.TicketWriteErrors += failure.WriteConcern
					}
					if len(result.Samples) < 12 {
						result.Samples = append(result.Samples, err.Error())
					}
					lock.Unlock()
					if sleepContext(ctx, time.Duration(attempt+1)*500*time.Millisecond) != nil {
						break
					}
				}
			}
		})
	}
	attempted := 0
schedule:
	for i := 0; i < goal; i++ {
		if bots[i] != nil {
			continue
		}
		select {
		case <-ctx.Done():
			break schedule
		case positions <- i:
			attempted++
		}
	}
	close(positions)
	workers.Wait()
	result.Roles = attempted
	result.Success = len(timings)
	result.FirstSuccess = result.Success - result.Recovered
	result.Confirmation = hostingLoadDurations(timings)
	return result
}
