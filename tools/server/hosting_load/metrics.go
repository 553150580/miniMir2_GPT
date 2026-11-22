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
	mu     sync.Mutex
	phases []*hostingLoadMongoPhase
}

type hostingLoadMongoPhase struct {
	name         string
	startedAt    time.Time
	endedAt      time.Time
	commit       hostingLoadMongoSamples
	otherWrites  hostingLoadMongoSamples
	transactions hostingLoadMongoSamples
}

type hostingLoadMongoSamples struct {
	confirmed []time.Duration
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
	Count  int     `json:"count"`
	P50MS  float64 `json:"p50_ms,omitempty"`
	P95MS  float64 `json:"p95_ms,omitempty"`
	P99MS  float64 `json:"p99_ms,omitempty"`
	MaxMS  float64 `json:"max_ms,omitempty"`
	MeanMS float64 `json:"mean_ms,omitempty"`
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
	if len(m.phases) != 0 {
		current := m.phases[len(m.phases)-1]
		if current.name == name {
			return
		}
		current.endedAt = now
	}
	m.phases = append(m.phases, &hostingLoadMongoPhase{name: name, startedAt: now})
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
	current := m.phases[len(m.phases)-1]
	samples := &current.otherWrites
	if command == "commitTransaction" {
		samples = &current.commit
	}
	samples.record(elapsed, failure)
}

func (m *hostingLoadMongoMetrics) RecordTransaction(elapsed time.Duration, err error) {
	failure := classifyHostingLoadMongoError(err)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.phases[len(m.phases)-1].transactions.record(elapsed, failure)
}

func (s *hostingLoadMongoSamples) record(elapsed time.Duration, failure hostingLoadMongoFailures) {
	if failure.Total == 0 {
		s.confirmed = append(s.confirmed, elapsed)
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
	snapshot := make([]hostingLoadMongoPhase, len(m.phases))
	for i, phase := range m.phases {
		snapshot[i] = *phase
		snapshot[i].commit.confirmed = slices.Clone(phase.commit.confirmed)
		snapshot[i].otherWrites.confirmed = slices.Clone(phase.otherWrites.confirmed)
		snapshot[i].transactions.confirmed = slices.Clone(phase.transactions.confirmed)
	}
	m.mu.Unlock()
	now := time.Now()
	report := make([]hostingLoadMongoPhaseReport, len(snapshot))
	for i, phase := range snapshot {
		ended := phase.endedAt
		if ended.IsZero() {
			ended = now
		}
		report[i] = hostingLoadMongoPhaseReport{
			Phase: phase.name, StartedAt: phase.startedAt, EndedAt: ended,
			Commit: phase.commit.report(), OtherWrites: phase.otherWrites.report(), Transactions: phase.transactions.report(),
		}
	}
	return report
}

func (s hostingLoadMongoSamples) report() hostingLoadMongoSampleReport {
	return hostingLoadMongoSampleReport{Confirmed: hostingLoadDurations(s.confirmed), Failures: s.failures}
}

func hostingLoadDurations(samples []time.Duration) hostingLoadDurationReport {
	count := len(samples)
	if count == 0 {
		return hostingLoadDurationReport{}
	}
	slices.Sort(samples)
	totalMS := float64(0)
	for _, sample := range samples {
		totalMS += float64(sample) / float64(time.Millisecond)
	}
	percentile := func(p int) float64 {
		return float64(samples[(count*p+99)/100-1]) / float64(time.Millisecond)
	}
	return hostingLoadDurationReport{
		Count: count, P50MS: percentile(50), P95MS: percentile(95), P99MS: percentile(99),
		MaxMS: percentile(100), MeanMS: totalMS / float64(count),
	}
}
