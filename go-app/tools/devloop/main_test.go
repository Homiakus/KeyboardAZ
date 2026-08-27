package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateStagesRejectsDependencyCycle(t *testing.T) {
	stages := []stageSpec{
		{ID: "a", PlanHeading: "A", Status: "pending", DependsOn: []string{"b"}},
		{ID: "b", PlanHeading: "B", Status: "pending", DependsOn: []string{"a"}},
	}
	if err := validateStages(stages); err == nil {
		t.Fatal("expected dependency cycle rejection")
	}
}

func TestNextStageSkipsPhysicalBlockerAndChoosesLowestPriority(t *testing.T) {
	stages := []stageSpec{
		{ID: "blocked", PlanHeading: "blocked", Status: "partial", Priority: 1, BlockedBy: []string{"fixture"}},
		{ID: "done", PlanHeading: "done", Status: "done", Priority: 2},
		{ID: "next", PlanHeading: "next", Status: "in_progress", Priority: 3, DependsOn: []string{"done"}},
		{ID: "later", PlanHeading: "later", Status: "pending", Priority: 4, DependsOn: []string{"done"}},
	}
	got, ok := nextStage(stages)
	if !ok || got.ID != "next" {
		t.Fatalf("unexpected next stage: ok=%v stage=%+v", ok, got)
	}
}

func TestValidateEdgesRejectsUnknownScenarioValue(t *testing.T) {
	edges := edgeSpace{
		SchemaVersion: 1,
		CoveragePolicy: coveragePolicy{BaselineStrength: 2, CriticalStrength: 3},
		Dimensions: []dimensionSpec{
			{ID: "transport", Values: []string{"cdc", "hid"}},
			{ID: "state", Values: []string{"ready", "closed"}},
		},
		Scenarios: []scenarioSpec{
			{ID: "bad", Strength: 2, Axes: map[string][]string{"transport": {"wifi"}, "state": {"ready"}}, Invariants: []string{"must fail"}},
		},
	}
	if err := validateEdges(edges); err == nil {
		t.Fatal("expected unknown scenario value rejection")
	}
}

func TestSummarizeEdgesCountsCartesianAndPairwiseSpace(t *testing.T) {
	edges := edgeSpace{
		Dimensions: []dimensionSpec{
			{ID: "a", Values: []string{"1", "2"}},
			{ID: "b", Values: []string{"1", "2", "3"}},
			{ID: "c", Values: []string{"1", "2", "3", "4"}},
		},
		CoveragePolicy: coveragePolicy{CriticalStrength: 3},
		Scenarios: []scenarioSpec{{ID: "s", Strength: 3, Axes: map[string][]string{"a": {"1"}, "b": {"1"}, "c": {"1"}}}},
	}
	report := summarizeEdges(edges)
	if report.CartesianCases != "24" {
		t.Fatalf("cartesian=%s want 24", report.CartesianCases)
	}
	// 2*3 + 2*4 + 3*4 = 26.
	if report.PairwiseTuples != "26" {
		t.Fatalf("pairwise=%s want 26", report.PairwiseTuples)
	}
	if report.CriticalScenarios != 1 || report.MaximumStrength != 3 {
		t.Fatalf("unexpected scenario summary: %+v", report)
	}
}

func TestCheckMutationRejectsMissingExecutionAndSurvivors(t *testing.T) {
	temp := t.TempDir()
	path := filepath.Join(temp, "mutation.json")
	policy := mutationPolicy{MinimumEfficacy: 80, MinimumMutantCoverage: 75}

	writeMutationFixture(t, path, mutationReport{})
	if err := checkMutation(path, policy, false); err == nil {
		t.Fatal("zero-mutant report must fail")
	}

	writeMutationFixture(t, path, mutationReport{
		TestEfficacy:       99,
		MutationsCoverage: 99,
		MutantsTotal:       10,
		MutantsKilled:      9,
		MutantsLived:       1,
	})
	if err := checkMutation(path, policy, false); err == nil {
		t.Fatal("survived mutant must fail controlled test-of-tests gate")
	}
}

func TestCheckMutationPassesStrongReport(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mutation.json")
	writeMutationFixture(t, path, mutationReport{
		TestEfficacy:       100,
		MutationsCoverage: 100,
		MutantsTotal:       12,
		MutantsKilled:      12,
	})
	policy := mutationPolicy{MinimumEfficacy: 80, MinimumMutantCoverage: 75}
	if err := checkMutation(path, policy, false); err != nil {
		t.Fatalf("strong mutation report failed: %v", err)
	}
}

func writeMutationFixture(t *testing.T, path string, report mutationReport) {
	t.Helper()
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
