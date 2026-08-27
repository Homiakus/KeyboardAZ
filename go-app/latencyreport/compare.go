package latencyreport

import (
	"fmt"
	"time"
)

// ComparisonConfig defines the promotion rule for an alternate realtime
// transport. The default policy should be conservative: correctness must be
// perfect, fixture E2E coverage complete and tail latency must not regress.
type ComparisonConfig struct {
	MinSamples        int
	MinP95Improvement float64 // fraction: 0.20 means at least 20% lower p95
	RequireFixtureE2E bool
	MaxP99Regression  float64 // fraction: 0 means candidate p99 must not increase
}

type ComparisonResult struct {
	Passed                bool     `json:"passed"`
	Failures              []string `json:"failures,omitempty"`
	BaselineTransport     string   `json:"baseline_transport"`
	CandidateTransport    string   `json:"candidate_transport"`
	BaselineSamples       int      `json:"baseline_samples"`
	CandidateSamples      int      `json:"candidate_samples"`
	FixtureP50Improvement float64  `json:"fixture_p50_improvement_fraction"`
	FixtureP95Improvement float64  `json:"fixture_p95_improvement_fraction"`
	FixtureP99Improvement float64  `json:"fixture_p99_improvement_fraction"`
}

func DefaultHIDPromotionConfig() ComparisonConfig {
	return ComparisonConfig{
		MinSamples:        10000,
		MinP95Improvement: 0.20,
		RequireFixtureE2E: true,
		MaxP99Regression:  0,
	}
}

// CompareTransportDatasets evaluates a controlled baseline/candidate pair.
// It intentionally uses fixture E2E for the transport promotion decision:
// host RX -> SendInput starts after the transport has already delivered the
// event and therefore cannot prove a CDC-vs-HID wire latency improvement.
func CompareTransportDatasets(baseline, candidate Dataset, config ComparisonConfig) ComparisonResult {
	baselineSummary := Summarize(baseline.Samples)
	candidateSummary := Summarize(candidate.Samples)
	result := ComparisonResult{
		BaselineTransport:  baseline.Transport,
		CandidateTransport: candidate.Transport,
		BaselineSamples:    baselineSummary.Samples,
		CandidateSamples:   candidateSummary.Samples,
	}
	failures := make([]string, 0, 12)

	if baseline.Transport != TransportCDCV2 {
		failures = append(failures, fmt.Sprintf("baseline transport=%q, want %s", baseline.Transport, TransportCDCV2))
	}
	if candidate.Transport != TransportHIDV3 {
		failures = append(failures, fmt.Sprintf("candidate transport=%q, want %s", candidate.Transport, TransportHIDV3))
	}
	if config.MinSamples > 0 {
		if baselineSummary.Samples < config.MinSamples {
			failures = append(failures, fmt.Sprintf("baseline samples %d < required %d", baselineSummary.Samples, config.MinSamples))
		}
		if candidateSummary.Samples < config.MinSamples {
			failures = append(failures, fmt.Sprintf("candidate samples %d < required %d", candidateSummary.Samples, config.MinSamples))
		}
	}
	appendCorrectnessFailures := func(label string, summary Summary) {
		if summary.SequenceGaps != 0 {
			failures = append(failures, fmt.Sprintf("%s sequence gaps=%d", label, summary.SequenceGaps))
		}
		if summary.SequenceDuplicates != 0 {
			failures = append(failures, fmt.Sprintf("%s sequence duplicates=%d", label, summary.SequenceDuplicates))
		}
		if summary.SequenceOutOfOrder != 0 {
			failures = append(failures, fmt.Sprintf("%s sequence out_of_order=%d", label, summary.SequenceOutOfOrder))
		}
		if summary.InvalidFixtureTiming != 0 {
			failures = append(failures, fmt.Sprintf("%s invalid fixture timing=%d", label, summary.InvalidFixtureTiming))
		}
	}
	appendCorrectnessFailures("baseline", baselineSummary)
	appendCorrectnessFailures("candidate", candidateSummary)

	if config.RequireFixtureE2E {
		if baselineSummary.FixtureE2E.Count != baselineSummary.Samples {
			failures = append(failures, fmt.Sprintf("baseline fixture E2E coverage=%d/%d", baselineSummary.FixtureE2E.Count, baselineSummary.Samples))
		}
		if candidateSummary.FixtureE2E.Count != candidateSummary.Samples {
			failures = append(failures, fmt.Sprintf("candidate fixture E2E coverage=%d/%d", candidateSummary.FixtureE2E.Count, candidateSummary.Samples))
		}
	}

	if baselineSummary.FixtureE2E.Count > 0 && candidateSummary.FixtureE2E.Count > 0 {
		result.FixtureP50Improvement = latencyImprovement(baselineSummary.FixtureE2E.P50, candidateSummary.FixtureE2E.P50)
		result.FixtureP95Improvement = latencyImprovement(baselineSummary.FixtureE2E.P95, candidateSummary.FixtureE2E.P95)
		result.FixtureP99Improvement = latencyImprovement(baselineSummary.FixtureE2E.P99, candidateSummary.FixtureE2E.P99)

		if result.FixtureP95Improvement < config.MinP95Improvement {
			failures = append(failures, fmt.Sprintf("fixture p95 improvement %.2f%% < required %.2f%%", 100*result.FixtureP95Improvement, 100*config.MinP95Improvement))
		}
		maxCandidateP99 := scaleDuration(baselineSummary.FixtureE2E.P99, 1+config.MaxP99Regression)
		if candidateSummary.FixtureE2E.P99 > maxCandidateP99 {
			failures = append(failures, fmt.Sprintf("candidate fixture p99 %s > allowed %s", candidateSummary.FixtureE2E.P99, maxCandidateP99))
		}
	} else if config.RequireFixtureE2E {
		failures = append(failures, "fixture E2E distributions are required for transport promotion")
	}

	result.Failures = failures
	result.Passed = len(failures) == 0
	return result
}

func latencyImprovement(baseline, candidate time.Duration) float64 {
	if baseline <= 0 {
		return 0
	}
	return float64(baseline-candidate) / float64(baseline)
}

func scaleDuration(value time.Duration, factor float64) time.Duration {
	if factor < 0 {
		factor = 0
	}
	return time.Duration(float64(value) * factor)
}
