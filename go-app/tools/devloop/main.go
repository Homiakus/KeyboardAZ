package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type loopConfig struct {
	SchemaVersion              int                    `json:"schema_version"`
	PlanDocument               string                 `json:"plan_document"`
	ProgressDocument           string                 `json:"progress_document"`
	EdgeSpace                  string                 `json:"edge_space"`
	MaxSemanticRepairAttempts  int                    `json:"max_semantic_repair_attempts"`
	MaxInfrastructureRetries   int                    `json:"max_infrastructure_retries"`
	Autofix                    autofixPolicy          `json:"autofix"`
	Mutation                   mutationPolicy         `json:"mutation"`
	Gates                      map[string][]gateSpec  `json:"gates"`
	Stages                     []stageSpec            `json:"stages"`
}

type autofixPolicy struct {
	MaxFiles         int      `json:"max_files"`
	MaxChangedLines  int      `json:"max_changed_lines"`
	AllowedPrefixes  []string `json:"allowed_prefixes"`
	ForbiddenPrefixes []string `json:"forbidden_prefixes"`
	SafeClasses      []string `json:"safe_classes"`
}

type mutationPolicy struct {
	Tool                         string   `json:"tool"`
	Version                      string   `json:"version"`
	MinimumEfficacy              float64  `json:"minimum_efficacy"`
	MinimumMutantCoverage        float64  `json:"minimum_mutant_coverage"`
	CriticalMinimumEfficacy      float64  `json:"critical_minimum_efficacy"`
	CriticalMinimumMutantCoverage float64 `json:"critical_minimum_mutant_coverage"`
	CriticalPrefixes             []string `json:"critical_prefixes"`
}

type gateSpec struct {
	ID             string   `json:"id"`
	CWD            string   `json:"cwd"`
	Platforms      []string `json:"platforms"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Argv           []string `json:"argv"`
}

type stageSpec struct {
	ID            string   `json:"id"`
	PlanHeading   string   `json:"plan_heading"`
	Status        string   `json:"status"`
	Priority      int      `json:"priority"`
	DependsOn     []string `json:"depends_on"`
	BlockedBy     []string `json:"blocked_by"`
	EdgeScenarios []string `json:"edge_scenarios"`
}

type edgeSpace struct {
	SchemaVersion  int             `json:"schema_version"`
	CoveragePolicy coveragePolicy  `json:"coverage_policy"`
	Dimensions     []dimensionSpec `json:"dimensions"`
	Scenarios      []scenarioSpec  `json:"scenarios"`
}

type coveragePolicy struct {
	BaselineStrength     int      `json:"baseline_strength"`
	CriticalStrength     int      `json:"critical_strength"`
	ExhaustiveDimensions []string `json:"exhaustive_dimensions"`
	Rules                []string `json:"rules"`
}

type dimensionSpec struct {
	ID     string   `json:"id"`
	Values []string `json:"values"`
}

type scenarioSpec struct {
	ID         string              `json:"id"`
	Strength   int                 `json:"strength"`
	Axes       map[string][]string `json:"axes"`
	Invariants []string            `json:"invariants"`
}

type mutationReport struct {
	TestEfficacy      float64 `json:"test_efficacy"`
	MutationsCoverage float64 `json:"mutations_coverage"`
	MutantsTotal      int     `json:"mutants_total"`
	MutantsKilled     int     `json:"mutants_killed"`
	MutantsLived      int     `json:"mutants_lived"`
	MutantsNotCovered int     `json:"mutants_not_covered"`
	MutantsNotViable  int     `json:"mutants_not_viable"`
}

type edgeReport struct {
	Dimensions          int    `json:"dimensions"`
	CartesianCases      string `json:"cartesian_cases"`
	PairwiseTuples      string `json:"pairwise_tuples"`
	NamedScenarios      int    `json:"named_scenarios"`
	CriticalScenarios   int    `json:"critical_scenarios"`
	MaximumStrength     int    `json:"maximum_strength"`
}

func main() {
	command := flag.String("cmd", "validate", "validate|next|edge-report|safe-fix|gate|mutation-check|verify-diff")
	configPath := flag.String("config", "", "path to engineering/loop.json; auto-detected when empty")
	gateName := flag.String("gate", "fast", "gate profile for -cmd=gate")
	mutationPath := flag.String("mutation-report", "", "Gremlins JSON output for -cmd=mutation-check")
	criticalMutation := flag.Bool("critical", false, "use critical mutation thresholds")
	baseRef := flag.String("base", "HEAD~1", "git base ref for -cmd=verify-diff")
	flag.Parse()

	cfgPath, err := resolveConfigPath(*configPath)
	fatalIf(err)
	root := filepath.Dir(filepath.Dir(cfgPath))
	cfg, err := readJSON[loopConfig](cfgPath)
	fatalIf(err)
	edgePath := filepath.Join(root, filepath.FromSlash(cfg.EdgeSpace))
	edges, err := readJSON[edgeSpace](edgePath)
	fatalIf(err)

	switch *command {
	case "validate":
		fatalIf(validateAll(root, cfg, edges))
		fmt.Printf("engineering loop valid: %d stages, %d dimensions, %d named scenarios\n", len(cfg.Stages), len(edges.Dimensions), len(edges.Scenarios))
	case "next":
		fatalIf(validateAll(root, cfg, edges))
		stage, ok := nextStage(cfg.Stages)
		if !ok {
			fmt.Println("no unblocked executable stage")
			return
		}
		writeJSON(os.Stdout, stage)
	case "edge-report":
		fatalIf(validateAll(root, cfg, edges))
		writeJSON(os.Stdout, summarizeEdges(edges))
	case "safe-fix":
		fatalIf(validateAll(root, cfg, edges))
		fatalIf(safeFix(root, cfg.Autofix))
	case "gate":
		fatalIf(validateAll(root, cfg, edges))
		fatalIf(runGate(root, cfg, *gateName))
	case "mutation-check":
		fatalIf(validateAll(root, cfg, edges))
		if strings.TrimSpace(*mutationPath) == "" {
			fatalIf(errors.New("-mutation-report is required"))
		}
		fatalIf(checkMutation(*mutationPath, cfg.Mutation, *criticalMutation))
	case "verify-diff":
		fatalIf(validateAll(root, cfg, edges))
		fatalIf(verifyDiff(root, *baseRef, cfg.Autofix))
	default:
		fatalIf(fmt.Errorf("unknown command %q", *command))
	}
}

func resolveConfigPath(value string) (string, error) {
	candidates := []string{}
	if strings.TrimSpace(value) != "" {
		candidates = append(candidates, value)
	} else {
		candidates = append(candidates, "engineering/loop.json", "../engineering/loop.json")
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(absolute); err == nil && !info.IsDir() {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("engineering loop config not found; checked %v", candidates)
}

func readJSON[T any](path string) (T, error) {
	var result T
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("decode %s: %w", path, err)
	}
	return result, nil
}

func validateAll(root string, cfg loopConfig, edges edgeSpace) error {
	if cfg.SchemaVersion != 1 || edges.SchemaVersion != 1 {
		return fmt.Errorf("unsupported engineering schema version loop=%d edges=%d", cfg.SchemaVersion, edges.SchemaVersion)
	}
	for _, relative := range []string{cfg.PlanDocument, cfg.ProgressDocument, cfg.EdgeSpace} {
		if strings.TrimSpace(relative) == "" {
			return errors.New("engineering document path must not be empty")
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); err != nil {
			return fmt.Errorf("required engineering resource %s: %w", relative, err)
		}
	}
	if cfg.MaxSemanticRepairAttempts < 1 || cfg.MaxSemanticRepairAttempts > 5 {
		return fmt.Errorf("max semantic repair attempts must be 1..5, got %d", cfg.MaxSemanticRepairAttempts)
	}
	if cfg.MaxInfrastructureRetries < 0 || cfg.MaxInfrastructureRetries > 3 {
		return fmt.Errorf("max infrastructure retries must be 0..3, got %d", cfg.MaxInfrastructureRetries)
	}
	if cfg.Autofix.MaxFiles < 1 || cfg.Autofix.MaxChangedLines < 1 {
		return errors.New("autofix patch budgets must be positive")
	}
	if err := validateStages(cfg.Stages); err != nil {
		return err
	}
	if err := validateEdges(edges); err != nil {
		return err
	}
	scenarioIDs := make(map[string]struct{}, len(edges.Scenarios))
	for _, scenario := range edges.Scenarios {
		scenarioIDs[scenario.ID] = struct{}{}
	}
	for _, stage := range cfg.Stages {
		for _, scenario := range stage.EdgeScenarios {
			if _, ok := scenarioIDs[scenario]; !ok {
				return fmt.Errorf("stage %s references unknown edge scenario %s", stage.ID, scenario)
			}
		}
	}
	for gateName, commands := range cfg.Gates {
		if len(commands) == 0 {
			return fmt.Errorf("gate %s has no commands", gateName)
		}
		for _, gate := range commands {
			if gate.ID == "" || len(gate.Argv) == 0 {
				return fmt.Errorf("gate %s contains invalid command", gateName)
			}
			if gate.TimeoutSeconds <= 0 {
				return fmt.Errorf("gate %s/%s has non-positive timeout", gateName, gate.ID)
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(gate.CWD))); err != nil {
				return fmt.Errorf("gate %s/%s cwd: %w", gateName, gate.ID, err)
			}
		}
	}
	return nil
}

func validateStages(stages []stageSpec) error {
	if len(stages) == 0 {
		return errors.New("master plan has no stages")
	}
	allowedStatus := map[string]bool{"pending": true, "in_progress": true, "partial": true, "blocked": true, "done": true}
	byID := make(map[string]stageSpec, len(stages))
	for _, stage := range stages {
		if stage.ID == "" || stage.PlanHeading == "" {
			return errors.New("stage id and plan_heading are required")
		}
		if _, exists := byID[stage.ID]; exists {
			return fmt.Errorf("duplicate stage %s", stage.ID)
		}
		if !allowedStatus[stage.Status] {
			return fmt.Errorf("stage %s has invalid status %s", stage.ID, stage.Status)
		}
		if stage.Status == "blocked" && len(stage.BlockedBy) == 0 {
			return fmt.Errorf("blocked stage %s must name its blocker", stage.ID)
		}
		byID[stage.ID] = stage
	}
	for _, stage := range stages {
		for _, dependency := range stage.DependsOn {
			if _, ok := byID[dependency]; !ok {
				return fmt.Errorf("stage %s depends on unknown stage %s", stage.ID, dependency)
			}
		}
	}
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("cycle detected at stage %s", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range byID[id].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range byID {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func validateEdges(edges edgeSpace) error {
	if edges.CoveragePolicy.BaselineStrength < 2 || edges.CoveragePolicy.CriticalStrength < edges.CoveragePolicy.BaselineStrength {
		return errors.New("edge coverage strength policy is invalid")
	}
	dimensions := make(map[string]map[string]struct{}, len(edges.Dimensions))
	for _, dimension := range edges.Dimensions {
		if dimension.ID == "" || len(dimension.Values) < 2 {
			return fmt.Errorf("dimension %q must have at least two values", dimension.ID)
		}
		if _, exists := dimensions[dimension.ID]; exists {
			return fmt.Errorf("duplicate dimension %s", dimension.ID)
		}
		values := make(map[string]struct{}, len(dimension.Values))
		for _, value := range dimension.Values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("dimension %s contains empty value", dimension.ID)
			}
			if _, exists := values[value]; exists {
				return fmt.Errorf("dimension %s has duplicate value %s", dimension.ID, value)
			}
			values[value] = struct{}{}
		}
		dimensions[dimension.ID] = values
	}
	for _, exhaustive := range edges.CoveragePolicy.ExhaustiveDimensions {
		if _, ok := dimensions[exhaustive]; !ok {
			return fmt.Errorf("unknown exhaustive dimension %s", exhaustive)
		}
	}
	scenarioIDs := map[string]struct{}{}
	for _, scenario := range edges.Scenarios {
		if scenario.ID == "" || len(scenario.Axes) < 2 || len(scenario.Invariants) == 0 {
			return fmt.Errorf("scenario %q must contain >=2 axes and invariants", scenario.ID)
		}
		if _, exists := scenarioIDs[scenario.ID]; exists {
			return fmt.Errorf("duplicate scenario %s", scenario.ID)
		}
		scenarioIDs[scenario.ID] = struct{}{}
		if scenario.Strength < 2 || scenario.Strength > len(scenario.Axes) {
			return fmt.Errorf("scenario %s strength=%d incompatible with %d axes", scenario.ID, scenario.Strength, len(scenario.Axes))
		}
		for axis, selected := range scenario.Axes {
			values, ok := dimensions[axis]
			if !ok {
				return fmt.Errorf("scenario %s references unknown dimension %s", scenario.ID, axis)
			}
			if len(selected) == 0 {
				return fmt.Errorf("scenario %s axis %s has no selected values", scenario.ID, axis)
			}
			for _, value := range selected {
				if _, ok := values[value]; !ok {
					return fmt.Errorf("scenario %s uses unknown %s=%s", scenario.ID, axis, value)
				}
			}
		}
	}
	return nil
}

func nextStage(stages []stageSpec) (stageSpec, bool) {
	byID := make(map[string]stageSpec, len(stages))
	for _, stage := range stages {
		byID[stage.ID] = stage
	}
	candidates := make([]stageSpec, 0)
	for _, stage := range stages {
		if stage.Status != "pending" && stage.Status != "in_progress" && stage.Status != "partial" {
			continue
		}
		if len(stage.BlockedBy) != 0 {
			continue
		}
		ready := true
		for _, dependency := range stage.DependsOn {
			status := byID[dependency].Status
			if status != "done" && status != "partial" && status != "in_progress" {
				ready = false
				break
			}
		}
		if ready {
			candidates = append(candidates, stage)
		}
	}
	if len(candidates) == 0 {
		return stageSpec{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].Priority < candidates[j].Priority
	})
	return candidates[0], true
}

func summarizeEdges(edges edgeSpace) edgeReport {
	cartesian := big.NewInt(1)
	pairwise := big.NewInt(0)
	for _, dimension := range edges.Dimensions {
		cartesian.Mul(cartesian, big.NewInt(int64(len(dimension.Values))))
	}
	for i := 0; i < len(edges.Dimensions); i++ {
		for j := i + 1; j < len(edges.Dimensions); j++ {
			product := int64(len(edges.Dimensions[i].Values) * len(edges.Dimensions[j].Values))
			pairwise.Add(pairwise, big.NewInt(product))
		}
	}
	critical := 0
	maxStrength := 0
	for _, scenario := range edges.Scenarios {
		if scenario.Strength >= edges.CoveragePolicy.CriticalStrength {
			critical++
		}
		if scenario.Strength > maxStrength {
			maxStrength = scenario.Strength
		}
	}
	return edgeReport{
		Dimensions:        len(edges.Dimensions),
		CartesianCases:    cartesian.String(),
		PairwiseTuples:    pairwise.String(),
		NamedScenarios:    len(edges.Scenarios),
		CriticalScenarios: critical,
		MaximumStrength:   maxStrength,
	}
}

func safeFix(root string, policy autofixPolicy) error {
	allowed := false
	for _, class := range policy.SafeClasses {
		if class == "gofmt" {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.New("gofmt is not allowed by autofix policy")
	}
	goRoot := filepath.Join(root, "go-app")
	files := make([]string, 0, 128)
	err := filepath.WalkDir(goRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" || entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("no Go files found for safe gofmt autofix")
	}
	args := append([]string{"-w"}, files...)
	cmd := exec.Command("gofmt", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gofmt autofix: %w", err)
	}
	return nil
}

func runGate(root string, cfg loopConfig, name string) error {
	commands, ok := cfg.Gates[name]
	if !ok {
		return fmt.Errorf("unknown gate %q", name)
	}
	ran := 0
	for _, gate := range commands {
		if !supportsPlatform(gate.Platforms, runtime.GOOS) {
			fmt.Printf("SKIP %s on %s\n", gate.ID, runtime.GOOS)
			continue
		}
		ran++
		timeout := time.Duration(gate.TimeoutSeconds) * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		cmd := exec.CommandContext(ctx, gate.Argv[0], gate.Argv[1:]...)
		cmd.Dir = filepath.Join(root, filepath.FromSlash(gate.CWD))
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		fmt.Printf("RUN %s: %s\n", gate.ID, strings.Join(gate.Argv, " "))
		err := cmd.Run()
		cancel()
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("gate %s timed out after %s", gate.ID, timeout)
		}
		if err != nil {
			return fmt.Errorf("gate %s failed: %w", gate.ID, err)
		}
	}
	if ran == 0 {
		return fmt.Errorf("gate %s has no commands for platform %s", name, runtime.GOOS)
	}
	return nil
}

func supportsPlatform(platforms []string, current string) bool {
	for _, platform := range platforms {
		if platform == current {
			return true
		}
	}
	return false
}

func checkMutation(path string, policy mutationPolicy, critical bool) error {
	report, err := readJSON[mutationReport](path)
	if err != nil {
		return fmt.Errorf("mutation report missing/invalid; mutation testing may not have executed: %w", err)
	}
	if report.MutantsTotal <= 0 {
		return errors.New("mutation report contains zero mutants; test-of-tests did not execute")
	}
	efficacy := policy.MinimumEfficacy
	coverage := policy.MinimumMutantCoverage
	if critical {
		efficacy = policy.CriticalMinimumEfficacy
		coverage = policy.CriticalMinimumMutantCoverage
	}
	failures := []string{}
	if report.TestEfficacy < efficacy {
		failures = append(failures, fmt.Sprintf("test efficacy %.2f < %.2f", report.TestEfficacy, efficacy))
	}
	if report.MutationsCoverage < coverage {
		failures = append(failures, fmt.Sprintf("mutant coverage %.2f < %.2f", report.MutationsCoverage, coverage))
	}
	if report.MutantsLived > 0 {
		failures = append(failures, fmt.Sprintf("%d mutants survived", report.MutantsLived))
	}
	if len(failures) > 0 {
		return fmt.Errorf("mutation gate failed: %s", strings.Join(failures, "; "))
	}
	fmt.Printf("mutation gate passed: mutants=%d killed=%d efficacy=%.2f coverage=%.2f\n", report.MutantsTotal, report.MutantsKilled, report.TestEfficacy, report.MutationsCoverage)
	return nil
}

func verifyDiff(root, base string, policy autofixPolicy) error {
	cmd := exec.Command("git", "diff", "--numstat", base, "HEAD")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git diff --numstat %s HEAD: %w", base, err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	files := 0
	lines := 0
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 3)
		if len(parts) != 3 {
			continue
		}
		path := filepath.ToSlash(parts[2])
		for _, prefix := range policy.ForbiddenPrefixes {
			if strings.HasPrefix(path, prefix) {
				return fmt.Errorf("autofix diff touches forbidden path %s", path)
			}
		}
		allowed := false
		for _, prefix := range policy.AllowedPrefixes {
			if strings.HasPrefix(path, prefix) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("autofix diff touches path outside allowlist: %s", path)
		}
		files++
		for _, count := range parts[:2] {
			if count == "-" {
				lines += policy.MaxChangedLines + 1
				continue
			}
			value, parseErr := strconv.Atoi(count)
			if parseErr != nil {
				return parseErr
			}
			lines += value
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if files > policy.MaxFiles {
		return fmt.Errorf("autofix changes %d files; budget is %d", files, policy.MaxFiles)
	}
	if lines > policy.MaxChangedLines {
		return fmt.Errorf("autofix changes %d lines; budget is %d", lines, policy.MaxChangedLines)
	}
	fmt.Printf("autofix diff within policy: files=%d changed_lines=%d\n", files, lines)
	return nil
}

func writeJSON(output *os.File, value any) {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	fatalIf(encoder.Encode(value))
}

func fatalIf(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "devloop:", err)
	os.Exit(1)
}
