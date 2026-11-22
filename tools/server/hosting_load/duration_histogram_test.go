package main

import (
	"testing"
	"time"
)

func TestDurationHistogramCountsOver500MSExactly(t *testing.T) {
	samples := make([]time.Duration, 100)
	for i := range samples[:98] {
		samples[i] = durationThreshold - time.Nanosecond
	}
	samples[98] = durationThreshold
	samples[99] = durationThreshold + time.Nanosecond

	var histogram durationHistogram
	for _, sample := range samples {
		histogram.observe(sample)
	}
	approximate, exact := histogram.report(), hostingLoadDurations(samples)
	if approximate.Count != 100 || approximate.Over500MS != 1 || exact.Over500MS != 1 {
		t.Fatalf("500 毫秒阈值统计错误：直方图 %+v，精确统计 %+v", approximate, exact)
	}
	if exact.P99MS != 500 || approximate.P99MS != approximate.MaxMS || approximate.P99MS <= exact.P99MS {
		t.Fatalf("直方图应保留现有桶上界：直方图 %+v，精确统计 %+v", approximate, exact)
	}
}
