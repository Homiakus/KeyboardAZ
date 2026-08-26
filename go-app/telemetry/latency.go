package telemetry

import (
	"sort"
	"time"
)

// latencyWindowCapacity bounds memory while keeping enough recent samples to
// make p50/p95/p99 useful during sustained typing and HIL runs.
const latencyWindowCapacity = 2048

type durationWindow struct {
	samples [latencyWindowCapacity]time.Duration
	count   int
	next    int
}

func (w *durationWindow) add(value time.Duration) {
	if value < 0 {
		value = 0
	}
	w.samples[w.next] = value
	w.next = (w.next + 1) % len(w.samples)
	if w.count < len(w.samples) {
		w.count++
	}
}

func (w *durationWindow) percentiles() (time.Duration, time.Duration, time.Duration) {
	if w.count == 0 {
		return 0, 0, 0
	}

	values := make([]time.Duration, w.count)
	copy(values, w.samples[:w.count])
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })

	return percentile(values, 0.50), percentile(values, 0.95), percentile(values, 0.99)
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1)*p + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
