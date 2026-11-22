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
	LastTickRooms   [8]int                        `json:"last_tick_rooms"`
	MaxJobQueue     int                           `json:"max_db_queue_depth"`
	QueuePeaks      game.HostingLoadQueueDepth    `json:"max_db_queue_depth_by_lane"`
	QueueCapacity   game.HostingLoadQueueDepth    `json:"db_queue_capacity_by_lane"`
	TickWork        hostingLoadDurationReport     `json:"tick_work"`
	TickLag         hostingLoadDurationReport     `json:"tick_lag"`
	RewardLatency   hostingLoadDurationReport     `json:"kill_to_receipt"`
	Mongo           []hostingLoadMongoPhaseReport `json:"mongo"`
}

type tickMetrics struct {
	mu            sync.Mutex
	rooms         [8]int
	queueMax      int
	queueLaneMax  game.HostingLoadQueueDepth
	queueCapacity game.HostingLoadQueueDepth
	work, lag     []time.Duration
	rewards       []time.Duration
	queueFull     map[string]uint64
}

func startDiagnostics(addr string) (func(), error) {
	m := newHostingLoadMongoMetrics()
	diagnosticMongo(m)
	ticks := &tickMetrics{queueFull: make(map[string]uint64)}
	var closeMu sync.Mutex
	closeReasons := make(map[string]uint64)
	transport.HostingLoadObserveClose = func(reason string) {
		closeMu.Lock()
		closeReasons[reason]++
		closeMu.Unlock()
	}
	game.HostingLoadQueueFull = func(lane string) {
		ticks.mu.Lock()
		ticks.queueFull[lane]++
		ticks.mu.Unlock()
	}
	game.HostingLoadObserveReward = func(elapsed time.Duration) {
		ticks.mu.Lock()
		ticks.rewards = append(ticks.rewards, elapsed)
		ticks.mu.Unlock()
	}
	game.HostingLoadObserveTick = func(shard, rooms int, queued, capacity game.HostingLoadQueueDepth, duration, lag time.Duration) {
		ticks.mu.Lock()
		ticks.rooms[shard] = rooms
		ticks.queueMax = max(ticks.queueMax, queued.Business+queued.Settlement+queued.Ticket+queued.Entry)
		ticks.queueLaneMax.Business = max(ticks.queueLaneMax.Business, queued.Business)
		ticks.queueLaneMax.Settlement = max(ticks.queueLaneMax.Settlement, queued.Settlement)
		ticks.queueLaneMax.Ticket = max(ticks.queueLaneMax.Ticket, queued.Ticket)
		ticks.queueLaneMax.Entry = max(ticks.queueLaneMax.Entry, queued.Entry)
		ticks.queueCapacity = capacity
		ticks.work = append(ticks.work, duration)
		ticks.lag = append(ticks.lag, lag)
		ticks.mu.Unlock()
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	snapshot := func(out http.ResponseWriter, r *http.Request) {
		phase := r.URL.Query().Get("name")
		if phase != "" && !regexp.MustCompile("^(ramp|hold|after)-[0-9]+$").MatchString(phase) {
			http.Error(out, "阶段无效", 400)
			return
		}
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
		}
		// GC直方图开头和结尾是无穷边界，JSON仅保留有限区间起点。
		report.GCPauseBuckets = pauses.Buckets[1 : len(pauses.Buckets)-1]
		report.CloseReasons = make(map[string]uint64)
		closeMu.Lock()
		for reason, count := range closeReasons {
			report.CloseReasons[reason] = count
		}
		closeMu.Unlock()
		ticks.mu.Lock()
		report.LastTickRooms = ticks.rooms
		report.MaxJobQueue = ticks.queueMax
		report.QueuePeaks = ticks.queueLaneMax
		report.QueueCapacity = ticks.queueCapacity
		report.DBQueueFailures = make(map[string]uint64, len(ticks.queueFull))
		for lane, count := range ticks.queueFull {
			report.DBQueueFailures[lane] = count
			report.DBQueueFull += count
		}
		work, lag := append([]time.Duration(nil), ticks.work...), append([]time.Duration(nil), ticks.lag...)
		rewards := append([]time.Duration(nil), ticks.rewards...)
		if phase != "" {
			ticks.work = ticks.work[:0]
			ticks.lag = ticks.lag[:0]
			ticks.rewards = ticks.rewards[:0]
			ticks.queueMax = 0
			ticks.queueLaneMax = game.HostingLoadQueueDepth{}
		}
		ticks.mu.Unlock()
		report.TickWork = hostingLoadDurations(work)
		report.TickLag = hostingLoadDurations(lag)
		report.RewardLatency = hostingLoadDurations(rewards)
		if phase != "" {
			m.SetPhase(phase)
		}
		report.Mongo = m.Snapshot()
		out.Header().Set("Content-Type", "application/json")
		json.NewEncoder(out).Encode(report)
	}
	mux.HandleFunc("/snapshot", snapshot)
	mux.HandleFunc("/phase", snapshot)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	go srv.Serve(listener)
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}, nil
}
