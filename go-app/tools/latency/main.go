package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"hapticpad-go-app/latencyreport"
)

type distributionJSON struct {
	Count int     `json:"count"`
	P50US float64 `json:"p50_us"`
	P95US float64 `json:"p95_us"`
	P99US float64 `json:"p99_us"`
	MaxUS float64 `json:"max_us"`
}

type outputJSON struct {
	Samples            int              `json:"samples"`
	SequenceGaps       uint64           `json:"sequence_gaps"`
	SequenceDuplicates uint64           `json:"sequence_duplicates"`
	SequenceOutOfOrder uint64           `json:"sequence_out_of_order"`
	HostDispatch       distributionJSON `json:"host_rx_to_sendinput"`
	FixtureE2E         distributionJSON `json:"fixture_e2e"`
	GatePassed         bool             `json:"gate_passed"`
}

func main() {
	inputPath := flag.String("input", "", "HIL CSV produced according to tests/hil/latency_protocol.md")
	minSamples := flag.Int("min-samples", 10000, "minimum samples required for correctness gate")
	flag.Parse()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "-input is required")
		os.Exit(2)
	}
	file, err := os.Open(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open HIL CSV: %v\n", err)
		os.Exit(2)
	}
	defer file.Close()

	samples, err := latencyreport.ParseCSV(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse HIL CSV: %v\n", err)
		os.Exit(2)
	}
	summary := latencyreport.Summarize(samples)
	passed := summary.Samples >= *minSamples &&
		summary.SequenceGaps == 0 &&
		summary.SequenceDuplicates == 0 &&
		summary.SequenceOutOfOrder == 0

	output := outputJSON{
		Samples:            summary.Samples,
		SequenceGaps:       summary.SequenceGaps,
		SequenceDuplicates: summary.SequenceDuplicates,
		SequenceOutOfOrder: summary.SequenceOutOfOrder,
		HostDispatch:       convertDistribution(summary.HostDispatch),
		FixtureE2E:         convertDistribution(summary.FixtureE2E),
		GatePassed:         passed,
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(2)
	}
	if !passed {
		os.Exit(1)
	}
}

func convertDistribution(distribution latencyreport.Distribution) distributionJSON {
	return distributionJSON{
		Count: distribution.Count,
		P50US: durationUS(distribution.P50),
		P95US: durationUS(distribution.P95),
		P99US: durationUS(distribution.P99),
		MaxUS: durationUS(distribution.Max),
	}
}

func durationUS(value time.Duration) float64 {
	return float64(value) / float64(time.Microsecond)
}
