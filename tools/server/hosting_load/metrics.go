package main

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

type hostingLoadMongoMetrics struct {
	mu        sync.Mutex
	completed []hostingLoadMongoPhaseReport
	current   hostingLoadMongoPhase
	interval  hostingLoadMongoPhase
}

type hostingLoadMongoPhase struct {
	name         string
	startedAt    time.Time
	commit       hostingLoadMongoSamples
	otherWrites  hostingLoadMongoSamples
	transactions hostingLoadMongoSamples
}

type hostingLoadMongoSamples struct {
	confirmed durationHistogram
	failures  hostingLoadMongoFailures
}

type hostingLoadMongoFailures struct {
	Total        int `json:"total"`
	Timeout      int `json:"timeout"`
	WriteConcern int `json:"write_concern"`
	Unknown      int `json:"unknown_commit_result"`
	Transient    int `json:"transient_transaction"`
	Network      int `json:"network"`
	Pending      int `json:"operation_pending"`
}

type hostingLoadDurationReport struct {
	Count     int     `json:"count"`
	Over500MS int     `json:"over_500_ms"`
	P50MS     float64 `json:"p50_ms,omitempty"`
	P95MS     float64 `json:"p95_ms,omitempty"`
	P99MS     float64 `json:"p99_ms,omitempty"`
	MaxMS     float64 `json:"max_ms,omitempty"`
	MeanMS    float64 `json:"mean_ms,omitempty"`
}

type hostingLoadMongoSampleReport struct {
	Confirmed hostingLoadDurationReport `json:"confirmed"`
	Failures  hostingLoadMongoFailures  `json:"failures"`
}

type hostingLoadMongoPhaseReport struct {
	Phase        string                       `json:"phase"`
	StartedAt    time.Time                    `json:"started_at"`
	EndedAt      time.Time                    `json:"ended_at"`
	Commit       hostingLoadMongoSampleReport `json:"commit_transaction"`
	OtherWrites  hostingLoadMongoSampleReport `json:"other_write_commands"`
	Transactions hostingLoadMongoSampleReport `json:"application_transactions"`
}

func newHostingLoadMongoMetrics() *hostingLoadMongoMetrics {
	metrics := &hostingLoadMongoMetrics{}
	metrics.SetPhase("startup")
	return metrics
}

func (m *hostingLoadMongoMetrics) SetPhase(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if !m.current.startedAt.IsZero() {
		if m.current.name == name {
			return
		}
		m.completed = append(m.completed, m.current.report(now))
	}
	m.current = hostingLoadMongoPhase{name: name, startedAt: now}
	m.interval = hostingLoadMongoPhase{name: name, startedAt: now}
}

func (m *hostingLoadMongoMetrics) Monitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Succeeded: func(_ context.Context, e *event.CommandSucceededEvent) {
			if !hostingLoadWriteCommand(e.CommandName) {
				return
			}
			outcome := classifyHostingLoadMongoReply(e.Reply)
			m.recordCommand(e.CommandName, e.Duration, outcome)
		},
		Failed: func(_ context.Context, e *event.CommandFailedEvent) {
			if hostingLoadWriteCommand(e.CommandName) {
				m.recordCommand(e.CommandName, e.Duration, classifyHostingLoadMongoError(e.Failure))
			}
		},
	}
}

func hostingLoadWriteCommand(command string) bool {
	switch command {
	case "commitTransaction", "insert", "update", "delete", "findAndModify", "bulkWrite":
		return true
	}
	return false
}

func classifyHostingLoadMongoReply(reply bson.Raw) hostingLoadMongoFailures {
	var result hostingLoadMongoFailures
	// 驱动在顶层 ok=1 时仍会发布成功事件，须另行核对写入确认和写错误。
	if writeConcern, ok := reply.Lookup("writeConcernError").DocumentOK(); ok {
		result.Total = 1
		result.WriteConcern = 1
		code, _ := writeConcern.Lookup("code").AsInt64OK()
		wtimeout, _ := writeConcern.Lookup("errInfo", "wtimeout").BooleanOK()
		if code == 64 || code == 50 || wtimeout {
			result.Timeout = 1
		}
	}
	if writeErrors, ok := reply.Lookup("writeErrors").ArrayOK(); ok && len(writeErrors) > 5 {
		result.Total = 1
	}
	// bulkWrite 的逐项错误会随成功命令的结果游标返回。
	if count, ok := reply.Lookup("nErrors").AsInt64OK(); ok && count > 0 {
		result.Total = 1
		if batch, ok := reply.Lookup("cursor", "firstBatch").ArrayOK(); ok {
			values, _ := batch.Values()
			for _, value := range values {
				doc, _ := value.DocumentOK()
				code, _ := doc.Lookup("code").AsInt64OK()
				switch code {
				case 50, 64, 262:
					result.Timeout = 1
				case 112:
					result.Transient = 1
				}
			}
		}
	}
	if labels, ok := reply.Lookup("errorLabels").ArrayOK(); ok {
		values, _ := labels.Values()
		for _, value := range values {
			label, _ := value.StringValueOK()
			if label == "UnknownTransactionCommitResult" {
				result.Unknown = 1
			}
			if label == "TransientTransactionError" {
				result.Transient = 1
			}
		}
	}
	return result
}

func classifyHostingLoadMongoError(err error) hostingLoadMongoFailures {
	if err == nil {
		return hostingLoadMongoFailures{}
	}
	result := hostingLoadMongoFailures{Total: 1}
	var internal driver.Error
	if mongo.IsTimeout(err) || errors.As(err, &internal) && (internal.Code == 50 || internal.Code == 64 || internal.Code == 262) {
		result.Timeout = 1
	}
	if mongo.IsNetworkError(err) {
		result.Network = 1
	}
	var labeled mongo.LabeledError
	if errors.As(err, &labeled) {
		if labeled.HasErrorLabel("UnknownTransactionCommitResult") {
			result.Unknown = 1
		}
		if labeled.HasErrorLabel("TransientTransactionError") {
			result.Transient = 1
		}
	}
	var write mongo.WriteException
	var driverWrite driver.WriteCommandError
	if errors.As(err, &write) && write.WriteConcernError != nil {
		result.WriteConcern = 1
		if write.WriteConcernError.Code == 64 {
			result.Timeout = 1
		}
	}
	if errors.As(err, &driverWrite) && driverWrite.WriteConcernError != nil {
		result.WriteConcern = 1
		if code := driverWrite.WriteConcernError.Code; code == 64 || code == 50 {
			result.Timeout = 1
		}
	}
	var fault *persistence.Fault
	if errors.As(err, &fault) && fault.Code == p.CodeOperationPending {
		result.Pending = 1
	}
	return result
}

func (m *hostingLoadMongoMetrics) recordCommand(command string, elapsed time.Duration, failure hostingLoadMongoFailures) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if command == "commitTransaction" {
		m.current.commit.record(elapsed, failure)
		m.interval.commit.record(elapsed, failure)
		return
	}
	m.current.otherWrites.record(elapsed, failure)
	m.interval.otherWrites.record(elapsed, failure)
}

func (m *hostingLoadMongoMetrics) RecordTransaction(elapsed time.Duration, err error) {
	failure := classifyHostingLoadMongoError(err)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current.transactions.record(elapsed, failure)
	m.interval.transactions.record(elapsed, failure)
}

func (s *hostingLoadMongoSamples) record(elapsed time.Duration, failure hostingLoadMongoFailures) {
	if failure.Total == 0 {
		s.confirmed.observe(elapsed)
		return
	}
	s.failures.Total += failure.Total
	s.failures.Timeout += failure.Timeout
	s.failures.WriteConcern += failure.WriteConcern
	s.failures.Unknown += failure.Unknown
	s.failures.Transient += failure.Transient
	s.failures.Network += failure.Network
	s.failures.Pending += failure.Pending
}

func (m *hostingLoadMongoMetrics) Snapshot() []hostingLoadMongoPhaseReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	report := make([]hostingLoadMongoPhaseReport, len(m.completed), len(m.completed)+1)
	copy(report, m.completed)
	return append(report, m.current.report(time.Now()))
}

func (m *hostingLoadMongoMetrics) TakeInterval() hostingLoadMongoPhaseReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	report := m.interval.report(now)
	m.interval = hostingLoadMongoPhase{name: m.current.name, startedAt: now}
	return report
}

func (p *hostingLoadMongoPhase) report(endedAt time.Time) hostingLoadMongoPhaseReport {
	return hostingLoadMongoPhaseReport{
		Phase: p.name, StartedAt: p.startedAt, EndedAt: endedAt,
		Commit: p.commit.report(), OtherWrites: p.otherWrites.report(), Transactions: p.transactions.report(),
	}
}

func (s *hostingLoadMongoSamples) report() hostingLoadMongoSampleReport {
	return hostingLoadMongoSampleReport{Confirmed: s.confirmed.report(), Failures: s.failures}
}

func hostingLoadDurations(samples []time.Duration) hostingLoadDurationReport {
	count := len(samples)
	if count == 0 {
		return hostingLoadDurationReport{}
	}
	slices.Sort(samples)
	totalMS := float64(0)
	over500 := 0
	for _, sample := range samples {
		totalMS += float64(sample) / float64(time.Millisecond)
		if sample > durationThreshold {
			over500++
		}
	}
	percentile := func(p int) float64 {
		return float64(samples[(count*p+99)/100-1]) / float64(time.Millisecond)
	}
	return hostingLoadDurationReport{
		Count: count, Over500MS: over500, P50MS: percentile(50), P95MS: percentile(95), P99MS: percentile(99),
		MaxMS: percentile(100), MeanMS: totalMS / float64(count),
	}
}
