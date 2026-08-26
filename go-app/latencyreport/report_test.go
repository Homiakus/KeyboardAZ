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
	if summary.Samples != 5 {
		t.Fatalf("unexpected summary sample count %d", summary.Samples)
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
		{Sequence: 1, T2HostRxNS: 100, T3SendInputNS: 150},
		{Sequence: 2},
	})
	if summary.HostDispatch.Count != 1 || summary.HostDispatch.P50 != 50*time.Nanosecond {
		t.Fatalf("unexpected host timing summary %+v", summary.HostDispatch)
	}
	if summary.FixtureE2E.Count != 0 {
		t.Fatalf("expected no fixture E2E samples, got %+v", summary.FixtureE2E)
	}
}

func TestParseCSVRejectsSchemaDrift(t *testing.T) {
	_, err := ParseCSV(strings.NewReader("sequence,bad\n1,2\n"))
	if err == nil {
		t.Fatal("expected schema validation error")
	}
}
