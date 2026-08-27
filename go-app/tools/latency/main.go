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
	Transport            string           `json:"transport"`
	Samples              int              `json:"samples"`
	SequenceGaps         uint64           `json:"sequence_gaps"`
	SequenceDuplicates   uint64           `json:"sequence_duplicates"`
	SequenceOutOfOrder   uint64           `json:"sequence_out_of_order"`
	InvalidHostTiming    uint64           `json:"invalid_host_timing"`
	InvalidFixtureTiming uint64           `json:"invalid_fixture_timing"`
	HostDispatch         distributionJSON `json:"host_rx_to_sendinput"`
	FixtureE2E           distributionJSON `json:"fixture_e2e"`
	GatePassed           bool             `json:"gate_passed"`
	GateFailures         []string         `json:"gate_failures,omitempty"`
}

func main() {
	inputPath := flag.String("input", "", "HIL CSV produced according to tests/hil/latency_protocol.md")
	minSamples := flag.Int("min-samples", 10000, "minimum samples required for correctness gate")
	requireHost := flag.Bool("require-host-timing", true, "require host RX -> SendInput timing for every sample")
	requireE2E := flag.Bool("require-fixture-e2e", false, "require fixture T0 -> T4 timing for every sample")
	maxHostP95US := flag.Float64("max-host-p95-us", 1000, "maximum host RX -> SendInput p95 in microseconds; 0 disables")
	maxHostP99US := flag.Float64("max-host-p99-us", 0, "maximum host RX -> SendInput p99 in microseconds; 0 disables")
	maxE2EP95US := flag.Float64("max-e2e-p95-us", 0, "maximum fixture E2E p95 in microseconds; 0 disables")
	maxE2EP99US := flag.Float64("max-e2e-p99-us", 0, "maximum fixture E2E p99 in microseconds; 0 disables")
	flag.Parse()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "-input is required")
		os.Exit(2)
	}
	for name, value := range map[string]float64{
		"max-host-p95-us": *maxHostP95US,
		"max-host-p99-us": *maxHostP99US,
		"max-e2e-p95-us":  *maxE2EP95US,
		"max-e2e-p99-us":  *maxE2EP99US,
	} {
		if value < 0 {
			fmt.Fprintf(os.Stderr, "-%s must be >= 0\n", name)
			os.Exit(2)
		}
	}

	file, err := os.Open(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open HIL CSV: %v\n", err)
		os.Exit(2)
	}
	defer file.Close()

	dataset, err := latencyreport.ParseDatasetCSV(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse HIL CSV: %v\n", err)
		os.Exit(2)
	}
	summary := latencyreport.Summarize(dataset.Samples)
	gate := latencyreport.EvaluateGate(summary, latencyreport.GateConfig{
		MinSamples:        *minSamples,
		RequireHostTiming: *requireHost,
		RequireFixtureE2E: *requireE2E,
		MaxHostP95:        microseconds(*maxHostP95US),
		MaxHostP99:        microseconds(*maxHostP99US),
		MaxFixtureP95:     microseconds(*maxE2EP95US),
		MaxFixtureP99:     microseconds(*maxE2EP99US),
	})

	output := outputJSON{
		Transport:            dataset.Transport,
		Samples:              summary.Samples,
		SequenceGaps:         summary.SequenceGaps,
		SequenceDuplicates:   summary.SequenceDuplicates,
		SequenceOutOfOrder:   summary.SequenceOutOfOrder,
		InvalidHostTiming:    summary.InvalidHostTiming,
		InvalidFixtureTiming: summary.InvalidFixtureTiming,
		HostDispatch:         convertDistribution(summary.HostDispatch),
		FixtureE2E:           convertDistribution(summary.FixtureE2E),
		GatePassed:           gate.Passed,
		GateFailures:         gate.Failures,
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
		os.Exit(2)
	}
	if !gate.Passed {
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

func microseconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Microsecond))
}
