package main

import (
	"context"
	"encoding/json"
	"minimir/internal/game"
	"minimir/internal/transport"
	"net"
	"net/http"
	"regexp"
	"runtime"
	"runtime/metrics"
	"sync"
	"sync/atomic"
	"time"
)

type generalDiagnostics struct {
	At              time.Time                     `json:"at"`
	HeapAlloc       uint64                        `json:"heap_alloc_bytes"`
	HeapLive        uint64                        `json:"heap_live_bytes"`
	HeapInUse       uint64                        `json:"heap_inuse_bytes"`
	HeapSys         uint64                        `json:"heap_sys_bytes"`
	TotalAlloc      uint64                        `json:"total_alloc_bytes"`
	GCCount         uint32                        `json:"gc_count"`
	GCPauseNS       uint64                        `json:"gc_pause_total_ns"`
	GCCPUSeconds    float64                       `json:"gc_cpu_seconds"`
	GCPauseBuckets  []float64                     `json:"gc_pause_bounds_sec"`
	GCPauseCounts   []uint64                      `json:"gc_pause_counts"`
	Goroutines      int                           `json:"goroutines"`
	GOMAXPROCS      int                           `json:"gomaxprocs"`
	DBQueueFull     uint64                        `json:"db_queue_full"`
	DBQueueFailures map[string]uint64             `json:"db_queue_full_by_lane"`
	CloseReasons    map[string]uint64             `json:"server_close_reasons"`
	TerminalReasons map[string]uint64             `json:"server_terminal_reasons"`
	LastTickRooms   [8]int                        `json:"last_tick_rooms"`
	Rooms           game.HostingLoadRoomState     `json:"rooms"`
	RoomsByWorker   [8]game.HostingLoadRoomState  `json:"rooms_by_worker"`
	RoomsSampleAt   [8]time.Time                  `json:"rooms_sampled_at_by_worker"`
	RoomCreations   uint64                        `json:"room_creations"`
	RoomEvictions   uint64                        `json:"room_evictions"`
	MaxJobQueue     int                           `json:"max_db_queue_depth"`
	QueueDepth      game.HostingLoadQueueDepth    `json:"db_queue_depth_by_lane"`
	QueuePeaks      game.HostingLoadQueueDepth    `json:"max_db_queue_depth_by_lane"`
	QueueCapacity   game.HostingLoadQueueDepth    `json:"db_queue_capacity_by_lane"`
	TickWork        hostingLoadDurationReport     `json:"tick_work"`
	TickLag         hostingLoadDurationReport     `json:"tick_lag"`
	RewardLatency   hostingLoadDurationReport     `json:"kill_to_receipt"`
	Mongo           []hostingLoadMongoPhaseReport `json:"mongo,omitempty"`
	Interval        *hostingLoadIntervalReport    `json:"interval,omitempty"`
	LatencyQuantile string                        `json:"latency_quantiles"`
}

type hostingLoadIntervalReport struct {
	StartedAt     time.Time                   `json:"started_at"`
	EndedAt       time.Time                   `json:"ended_at"`
	TickWork      hostingLoadDurationReport   `json:"tick_work"`
	TickLag       hostingLoadDurationReport   `json:"tick_lag"`
	RewardLatency hostingLoadDurationReport   `json:"kill_to_receipt"`
	Mongo         hostingLoadMongoPhaseReport `json:"mongo"`
}

type tickMetrics struct {
	mu            sync.Mutex
	rooms         [8]game.HostingLoadRoomState
	roomsSampleAt [8]time.Time
	queueDepth    game.HostingLoadQueueDepth
	queueCapacity game.HostingLoadQueueDepth
	phase         tickWindow
	interval      tickWindow
	queueFull     map[string]uint64
}

type tickWindow struct {
	queueMax           int
	queueLaneMax       game.HostingLoadQueueDepth
	work, lag, rewards durationHistogram
}

func (w *tickWindow) observe(queued game.HostingLoadQueueDepth, work, lag time.Duration) {
	w.queueMax = max(w.queueMax, queued.Business+queued.Settlement+queued.Ticket+queued.Entry)
	w.queueLaneMax.Business = max(w.queueLaneMax.Business, queued.Business)
	w.queueLaneMax.Settlement = max(w.queueLaneMax.Settlement, queued.Settlement)
	w.queueLaneMax.Ticket = max(w.queueLaneMax.Ticket, queued.Ticket)
	w.queueLaneMax.Entry = max(w.queueLaneMax.Entry, queued.Entry)
	w.work.observe(work)
	w.lag.observe(lag)
}

func sumRoomStates(byWorker [8]game.HostingLoadRoomState) (total game.HostingLoadRoomState) {
	for _, s := range byWorker {
		total.Total += s.Total
		total.Online += s.Online
		total.OnlineAutoAlive += s.OnlineAutoAlive
		total.Managed += s.Managed
		total.Inactive += s.Inactive
		total.Joining += s.Joining
		total.Isolated += s.Isolated
		total.Pending += s.Pending
	}
	return total
}

func startDiagnostics(addr string) (func(), error) {
	m := newHostingLoadMongoMetrics()
	diagnosticMongo(m)
	ticks := &tickMetrics{queueFull: make(map[string]uint64)}
	var roomCreations, roomEvictions atomic.Uint64
	game.HostingLoadRoomCreated = func() { roomCreations.Add(1) }
	game.HostingLoadRoomEvicted = func() { roomEvictions.Add(1) }
	var closeMu sync.Mutex
	closeReasons := make(map[string]uint64)
	terminalReasons := make(map[string]uint64)
	transport.HostingLoadObserveClose = func(reason string) {
		closeMu.Lock()
		closeReasons[reason]++
		closeMu.Unlock()
	}
	transport.HostingLoadObserveTerminal = func(reason string) {
		closeMu.Lock()
		terminalReasons[reason]++
		closeMu.Unlock()
	}
	game.HostingLoadQueueFull = func(lane string) {
		ticks.mu.Lock()
		ticks.queueFull[lane]++
		ticks.mu.Unlock()
	}
	game.HostingLoadObserveReward = func(elapsed time.Duration) {
		ticks.mu.Lock()
		ticks.phase.rewards.observe(elapsed)
		ticks.interval.rewards.observe(elapsed)
		ticks.mu.Unlock()
	}
	game.HostingLoadObserveTick = func(shard int, at time.Time, rooms game.HostingLoadRoomState, roomsSampled bool, queued, capacity game.HostingLoadQueueDepth, duration, lag time.Duration) {
		ticks.mu.Lock()
		if roomsSampled {
			ticks.rooms[shard] = rooms
			ticks.roomsSampleAt[shard] = at
		}
		ticks.queueDepth = queued
		ticks.queueCapacity = capacity
		ticks.phase.observe(queued, duration, lag)
		ticks.interval.observe(queued, duration, lag)
		ticks.mu.Unlock()
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	phaseName := regexp.MustCompile(`^(ramp|hold|after)-[0-9]+$`)
	var sampleMu sync.Mutex
	snapshot := func(out http.ResponseWriter, r *http.Request) {
		var phase string
		if r.URL.Path == "/phase" {
			phase = r.URL.Query().Get("name")
			if !phaseName.MatchString(phase) {
				http.Error(out, "阶段无效", 400)
				return
			}
		}
		interval := r.URL.Path == "/soak"
		sampleMu.Lock()
		defer sampleMu.Unlock()
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		samples := []metrics.Sample{
			{Name: "/cpu/classes/gc/total:cpu-seconds"},
			{Name: "/gc/heap/live:bytes"},
			{Name: "/sched/pauses/total/gc:seconds"},
		}
		metrics.Read(samples)
		pauses := samples[2].Value.Float64Histogram()
		report := generalDiagnostics{
			At: time.Now(), HeapAlloc: mem.HeapAlloc, HeapLive: samples[1].Value.Uint64(), HeapInUse: mem.HeapInuse, HeapSys: mem.HeapSys, TotalAlloc: mem.TotalAlloc,
			GCCount: mem.NumGC, GCPauseNS: mem.PauseTotalNs, GCCPUSeconds: samples[0].Value.Float64(), GCPauseCounts: pauses.Counts,
			Goroutines: runtime.NumGoroutine(), GOMAXPROCS: runtime.GOMAXPROCS(0),
			RoomCreations: roomCreations.Load(), RoomEvictions: roomEvictions.Load(),
			LatencyQuantile: "histogram_upper_bound",
		}
		// GC直方图开头和结尾是无穷边界，JSON仅保留有限区间起点。
		report.GCPauseBuckets = pauses.Buckets[1 : len(pauses.Buckets)-1]
		report.CloseReasons = make(map[string]uint64)
		report.TerminalReasons = make(map[string]uint64)
		closeMu.Lock()
		for reason, count := range closeReasons {
			report.CloseReasons[reason] = count
		}
		for reason, count := range terminalReasons {
			report.TerminalReasons[reason] = count
		}
		closeMu.Unlock()
		ticks.mu.Lock()
		report.RoomsByWorker = ticks.rooms
		report.RoomsSampleAt = ticks.roomsSampleAt
		report.Rooms = sumRoomStates(ticks.rooms)
		for shard, roomState := range ticks.rooms {
			report.LastTickRooms[shard] = roomState.Total
		}
		window := &ticks.phase
		if interval {
			window = &ticks.interval
		}
		report.MaxJobQueue = window.queueMax
		report.QueuePeaks = window.queueLaneMax
		report.QueueDepth = ticks.queueDepth
		report.QueueCapacity = ticks.queueCapacity
		report.DBQueueFailures = make(map[string]uint64, len(ticks.queueFull))
		for lane, count := range ticks.queueFull {
			report.DBQueueFailures[lane] = count
			report.DBQueueFull += count
		}
		report.TickWork = window.work.report()
		report.TickLag = window.lag.report()
		report.RewardLatency = window.rewards.report()
		if phase != "" {
			ticks.phase = tickWindow{}
		}
		if phase != "" || interval {
			ticks.interval = tickWindow{}
		}
		ticks.mu.Unlock()
		if phase != "" {
			m.SetPhase(phase)
		}
		if interval {
			mongo := m.TakeInterval()
			report.Interval = &hostingLoadIntervalReport{
				StartedAt: mongo.StartedAt, EndedAt: mongo.EndedAt, Mongo: mongo,
				TickWork: report.TickWork, TickLag: report.TickLag, RewardLatency: report.RewardLatency,
			}
		} else {
			report.Mongo = m.Snapshot()
		}
		out.Header().Set("Content-Type", "application/json")
		json.NewEncoder(out).Encode(report)
	}
	mux.HandleFunc("/snapshot", snapshot)
	mux.HandleFunc("/metrics", snapshot)
	mux.HandleFunc("/phase", snapshot)
	mux.HandleFunc("/soak", snapshot)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	go srv.Serve(listener)
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}, nil
}
