package main

import (
	"math/bits"
	"time"
)

const (
	durationBucketBits = 5
	durationBucketSize = 1 << durationBucketBits
	durationBuckets    = 1 + 63*durationBucketSize
	durationThreshold  = 500 * time.Millisecond
)

// 每个倍增区间分成 32 个子桶，分位取桶的上界，最长运行也只使用固定空间。
// 最小区间按纳秒精确保存，最大值和均值使用观测值，独立于桶的边界。
type durationHistogram struct {
	buckets [durationBuckets]uint64
	count   uint64
	over500 uint64
	totalNS float64
	maximum time.Duration
}

func (h *durationHistogram) observe(elapsed time.Duration) {
	elapsed = max(0, elapsed)
	index := 0
	if elapsed > 0 {
		n := uint64(elapsed)
		exponent := bits.Len64(n) - 1
		base := uint64(1) << exponent
		shift := max(0, exponent-durationBucketBits)
		index = 1 + exponent*durationBucketSize + int((n-base)>>shift)
	}
	h.buckets[index]++
	h.count++
	// 直接比较观测值，避免 500 毫秒所在的子桶把两侧的样本混在一起。
	if elapsed > durationThreshold {
		h.over500++
	}
	h.totalNS += float64(elapsed)
	h.maximum = max(h.maximum, elapsed)
}

func durationBucketUpper(index int) time.Duration {
	if index == 0 {
		return 0
	}
	exponent, offset := (index-1)/durationBucketSize, (index-1)%durationBucketSize
	shift := max(0, exponent-durationBucketBits)
	return time.Duration((uint64(1) << exponent) + (uint64(offset+1) << shift) - 1)
}

func (h *durationHistogram) report() hostingLoadDurationReport {
	if h.count == 0 {
		return hostingLoadDurationReport{}
	}
	const ms = float64(time.Millisecond)
	out := hostingLoadDurationReport{
		Count: int(h.count), Over500MS: int(h.over500),
		MaxMS: float64(h.maximum) / ms, MeanMS: h.totalNS / ms / float64(h.count),
	}
	ranks := [3]uint64{(h.count*50 + 99) / 100, (h.count*95 + 99) / 100, (h.count*99 + 99) / 100}
	percentiles := [3]*float64{&out.P50MS, &out.P95MS, &out.P99MS}
	var accumulated uint64
	next := 0
	for bucket, hits := range &h.buckets {
		accumulated += hits
		for next < len(ranks) && accumulated >= ranks[next] {
			*percentiles[next] = float64(min(durationBucketUpper(bucket), h.maximum)) / ms
			next++
		}
		if next == len(ranks) {
			break
		}
	}
	return out
}
