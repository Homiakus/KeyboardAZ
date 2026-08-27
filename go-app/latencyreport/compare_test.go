package latencyreport

import (
	"testing"
	"time"
)

func TestCompareTransportDatasetsPassesMeaningfulHIDImprovement(t *testing.T) {
	baseline := Dataset{Transport: TransportCDCV2, Samples: syntheticE2ESamples(10000, 10*time.Millisecond)}
	candidate := Dataset{Transport: TransportHIDV3, Samples: syntheticE2ESamples(10000, 7*time.Millisecond)}

	result := CompareTransportDatasets(baseline, candidate, DefaultHIDPromotionConfig())
	if !result.Passed {
		t.Fatalf("expected HID promotion pass: %+v", result.Failures)
	}
	if result.FixtureP95Improvement < 0.29 || result.FixtureP95Improvement > 0.31 {
		t.Fatalf("unexpected p95 improvement %.3f", result.FixtureP95Improvement)
	}
}

func TestCompareTransportDatasetsRejectsSmallGainAndTailRegression(t *testing.T) {
	baseline := Dataset{Transport: TransportCDCV2, Samples: syntheticE2ESamples(10000, 10*time.Millisecond)}
	candidateSamples := syntheticE2ESamples(10000, 9*time.Millisecond)
	candidateSamples[len(candidateSamples)-1].T4FixtureNS = int64(12 * time.Millisecond)
	candidate := Dataset{Transport: TransportHIDV3, Samples: candidateSamples}

	result := CompareTransportDatasets(baseline, candidate, DefaultHIDPromotionConfig())
	if result.Passed {
		t.Fatal("small p95 gain / tail regression unexpectedly passed")
	}
	if len(result.Failures) < 2 {
		t.Fatalf("expected p95 and p99 failures, got %v", result.Failures)
	}
}

func TestCompareTransportDatasetsRejectsWrongTransportAndCorrectnessError(t *testing.T) {
	baseline := Dataset{Transport: TransportLegacy, Samples: syntheticE2ESamples(10000, 10*time.Millisecond)}
	candidateSamples := syntheticE2ESamples(10000, 7*time.Millisecond)
	candidateSamples[1].Sequence = candidateSamples[0].Sequence
	candidate := Dataset{Transport: TransportHIDV3, Samples: candidateSamples}

	result := CompareTransportDatasets(baseline, candidate, DefaultHIDPromotionConfig())
	if result.Passed || len(result.Failures) < 2 {
		t.Fatalf("invalid comparison unexpectedly passed: %+v", result)
	}
}

func TestCompareTransportDatasetsRequiresFixtureCoverage(t *testing.T) {
	baseline := Dataset{Transport: TransportCDCV2, Samples: syntheticE2ESamples(10000, 10*time.Millisecond)}
	candidate := Dataset{Transport: TransportHIDV3, Samples: syntheticE2ESamples(10000, 7*time.Millisecond)}
	candidate.Samples[0].T0FixtureNS = 0
	candidate.Samples[0].T4FixtureNS = 0

	result := CompareTransportDatasets(baseline, candidate, DefaultHIDPromotionConfig())
	if result.Passed {
		t.Fatal("missing fixture coverage unexpectedly passed")
	}
}

func syntheticE2ESamples(count int, latency time.Duration) []Sample {
	samples := make([]Sample, count)
	for i := range samples {
		t0 := int64(time.Second) + int64(i)*int64(20*time.Millisecond)
		samples[i] = Sample{
			Sequence:    uint32(i + 1),
			T0FixtureNS: t0,
			T4FixtureNS: t0 + int64(latency),
		}
	}
	return samples
}
