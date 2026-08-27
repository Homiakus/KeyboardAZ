package latencyreport

import (
	"strings"
	"testing"
	"time"
)

func TestParseCSVAndSummarize(t *testing.T) {
	input := strings.Join([]string{
		"sequence,t0_fixture_ns,t1_firmware_us,t2_host_rx_ns,t3_sendinput_ns,t4_fixture_ns,event_type,button,modifiers",
		"10,1000,50,10000,10100,5000,stroke,3,0",
		"11,2000,60,20000,20200,7000,stroke,4,0",
		"13,3000,70,30000,30300,9000,stroke,5,1",
		"13,4000,80,40000,40400,11000,stroke,5,1",
		"12,5000,90,50000,50500,13000,stroke,6,0",
	}, "\n") + "\n"

	samples, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(samples) != 5 {
		t.Fatalf("unexpected sample count %d", len(samples))
	}

	summary := Summarize(samples)
	if summary.Samples != 5 || summary.HostTimingExpected != 5 {
		t.Fatalf("unexpected summary coverage: %+v", summary)
	}
	if summary.SequenceGaps != 1 {
		t.Fatalf("unexpected sequence gaps %d", summary.SequenceGaps)
	}
	if summary.SequenceDuplicates != 1 {
		t.Fatalf("unexpected duplicates %d", summary.SequenceDuplicates)
	}
	if summary.SequenceOutOfOrder != 1 {
		t.Fatalf("unexpected out-of-order count %d", summary.SequenceOutOfOrder)
	}
	if summary.HostDispatch.Count != 5 || summary.HostDispatch.P50 != 300*time.Nanosecond || summary.HostDispatch.P95 != 500*time.Nanosecond {
		t.Fatalf("unexpected host distribution %+v", summary.HostDispatch)
	}
	if summary.FixtureE2E.Count != 5 || summary.FixtureE2E.P50 != 6*time.Microsecond || summary.FixtureE2E.Max != 8*time.Microsecond {
		t.Fatalf("unexpected e2e distribution %+v", summary.FixtureE2E)
	}
}

func TestSummarizeHandlesSequenceWrapWithoutFalseGap(t *testing.T) {
	summary := Summarize([]Sample{
		{Sequence: ^uint32(0)},
		{Sequence: 1},
		{Sequence: 2},
	})
	if summary.SequenceGaps != 0 || summary.SequenceOutOfOrder != 0 {
		t.Fatalf("wrap misclassified: %+v", summary)
	}
}

func TestSummarizeSkipsMissingTimingStages(t *testing.T) {
	summary := Summarize([]Sample{
		{Sequence: 1, EventType: "stroke", T2HostRxNS: 100, T3SendInputNS: 150},
		{Sequence: 2, EventType: "stroke"},
	})
	if summary.HostTimingExpected != 2 {
		t.Fatalf("unexpected expected host timing count %d", summary.HostTimingExpected)
	}
	if summary.HostDispatch.Count != 1 || summary.HostDispatch.P50 != 50*time.Nanosecond {
		t.Fatalf("unexpected host timing summary %+v", summary.HostDispatch)
	}
	if summary.FixtureE2E.Count != 0 {
		t.Fatalf("expected no fixture E2E samples, got %+v", summary.FixtureE2E)
	}
}

func TestSummarizeExcludesLanguageFromHostTimingCoverage(t *testing.T) {
	summary := Summarize([]Sample{
		{Sequence: 1, EventType: "stroke", T2HostRxNS: 100, T3SendInputNS: 150},
		{Sequence: 2, EventType: "language", T2HostRxNS: 200},
		{Sequence: 3, EventType: "tap", T2HostRxNS: 300, T3SendInputNS: 375},
	})
	if summary.Samples != 3 || summary.HostTimingExpected != 2 || summary.HostDispatch.Count != 2 {
		t.Fatalf("language event distorted host coverage: %+v", summary)
	}
	result := EvaluateGate(summary, GateConfig{RequireHostTiming: true})
	if !result.Passed {
		t.Fatalf("state-only event caused false host coverage failure: %+v", result)
	}
}

func TestExpectsHostDispatchClassifiesSemanticEvents(t *testing.T) {
	for _, eventType := range []string{"stroke", "tap", "press", "combo", "repeat", " STROKE "} {
		if !ExpectsHostDispatch(eventType) {
			t.Fatalf("expected %q to require host dispatch", eventType)
		}
	}
	for _, eventType := range []string{"language", "status", "ready", "armed", "error", ""} {
		if ExpectsHostDispatch(eventType) {
			t.Fatalf("expected %q to be state-only", eventType)
		}
	}
}

func TestSummarizeCountsInvalidClockOrdering(t *testing.T) {
	summary := Summarize([]Sample{
		{Sequence: 1, EventType: "stroke", T2HostRxNS: 200, T3SendInputNS: 100, T0FixtureNS: 400, T4FixtureNS: 300},
	})
	if summary.InvalidHostTiming != 1 || summary.InvalidFixtureTiming != 1 {
		t.Fatalf("invalid timings were hidden: %+v", summary)
	}
	if summary.HostDispatch.Count != 0 || summary.FixtureE2E.Count != 0 {
		t.Fatalf("invalid timings entered distributions: %+v", summary)
	}
}

func TestEvaluateGatePassesCompleteSeriesWithinBudgets(t *testing.T) {
	summary := Summary{
		Samples:            10000,
		HostTimingExpected: 10000,
		HostDispatch:       Distribution{Count: 10000, P95: 800 * time.Microsecond, P99: 950 * time.Microsecond},
		FixtureE2E:         Distribution{Count: 10000, P95: 7 * time.Millisecond, P99: 9 * time.Millisecond},
	}
	result := EvaluateGate(summary, GateConfig{
		MinSamples:        10000,
		RequireHostTiming: true,
		RequireFixtureE2E: true,
		MaxHostP95:        time.Millisecond,
		MaxHostP99:        2 * time.Millisecond,
		MaxFixtureP95:     8 * time.Millisecond,
		MaxFixtureP99:     10 * time.Millisecond,
	})
	if !result.Passed || len(result.Failures) != 0 {
		t.Fatalf("valid HIL series failed gate: %+v", result)
	}
}

func TestEvaluateGateExplainsCorrectnessCoverageAndLatencyFailures(t *testing.T) {
	summary := Summary{
		Samples:              9999,
		HostTimingExpected:   9999,
		SequenceGaps:         1,
		SequenceDuplicates:   2,
		SequenceOutOfOrder:   3,
		InvalidHostTiming:    1,
		InvalidFixtureTiming: 1,
		HostDispatch:         Distribution{Count: 9998, P95: 2 * time.Millisecond},
		FixtureE2E:           Distribution{Count: 9997, P99: 15 * time.Millisecond},
	}
	result := EvaluateGate(summary, GateConfig{
		MinSamples:        10000,
		RequireHostTiming: true,
		RequireFixtureE2E: true,
		MaxHostP95:        time.Millisecond,
		MaxFixtureP99:     10 * time.Millisecond,
	})
	if result.Passed {
		t.Fatal("invalid HIL series passed gate")
	}
	if len(result.Failures) < 9 {
		t.Fatalf("gate did not explain all failure classes: %+v", result.Failures)
	}
}

func TestParseCSVRejectsSchemaDriftAndInvalidSemanticFields(t *testing.T) {
	if _, err := ParseCSV(strings.NewReader("sequence,bad\n1,2\n")); err == nil {
		t.Fatal("expected schema validation error")
	}

	header := "sequence,t0_fixture_ns,t1_firmware_us,t2_host_rx_ns,t3_sendinput_ns,t4_fixture_ns,event_type,button,modifiers\n"
	cases := []string{
		"0,0,0,1,2,0,stroke,1,0\n",
		"1,0,0,1,2,0,,1,0\n",
		"1,0,0,1,2,0,stroke,22,0\n",
		"1,0,0,1,2,0,stroke,1,16\n",
		"1,-1,0,1,2,0,stroke,1,0\n",
	}
	for _, row := range cases {
		if _, err := ParseCSV(strings.NewReader(header + row)); err == nil {
			t.Fatalf("expected invalid row rejection: %q", row)
		}
	}
}
