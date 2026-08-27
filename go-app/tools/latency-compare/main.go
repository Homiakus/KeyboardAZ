package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"hapticpad-go-app/latencyreport"
)

func main() {
	baselinePath := flag.String("baseline", "", "transport-aware CDC-v2 HIL CSV")
	candidatePath := flag.String("candidate", "", "transport-aware HID-v3 HIL CSV")
	minSamples := flag.Int("min-samples", 10000, "minimum samples required per dataset")
	minP95Improvement := flag.Float64("min-p95-improvement-percent", 20, "minimum fixture E2E p95 improvement required from HID")
	maxP99Regression := flag.Float64("max-p99-regression-percent", 0, "maximum allowed fixture E2E p99 regression")
	flag.Parse()

	if *baselinePath == "" || *candidatePath == "" {
		fmt.Fprintln(os.Stderr, "-baseline and -candidate are required")
		os.Exit(2)
	}
	if *minP95Improvement < 0 || *maxP99Regression < 0 {
		fmt.Fprintln(os.Stderr, "improvement/regression percentages must be >= 0")
		os.Exit(2)
	}

	baseline, err := readDataset(*baselinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "baseline: %v\n", err)
		os.Exit(2)
	}
	candidate, err := readDataset(*candidatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "candidate: %v\n", err)
		os.Exit(2)
	}

	config := latencyreport.DefaultHIDPromotionConfig()
	config.MinSamples = *minSamples
	config.MinP95Improvement = *minP95Improvement / 100
	config.MaxP99Regression = *maxP99Regression / 100
	result := latencyreport.CompareTransportDatasets(baseline, candidate, config)

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "write comparison: %v\n", err)
		os.Exit(2)
	}
	if !result.Passed {
		os.Exit(1)
	}
}

func readDataset(path string) (latencyreport.Dataset, error) {
	file, err := os.Open(path)
	if err != nil {
		return latencyreport.Dataset{}, err
	}
	defer file.Close()
	return latencyreport.ParseDatasetCSV(file)
}
