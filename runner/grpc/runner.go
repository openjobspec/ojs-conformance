package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/openjobspec/ojs-conformance/lib"
	"google.golang.org/grpc/codes"
)

var templateRefPattern = regexp.MustCompile(`\{\{steps\.([^.]+)\.response\.body\.([^}]+)\}\}`)

// loadTests recursively loads all JSON test files from a directory.
func loadTests(dir string) ([]lib.TestCase, error) {
	var tests []lib.TestCase

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		var tc lib.TestCase
		if err := json.Unmarshal(data, &tc); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		tc.FilePath = path
		tests = append(tests, tc)
		return nil
	})

	sort.Slice(tests, func(i, j int) bool {
		return tests[i].TestID < tests[j].TestID
	})

	return tests, err
}

// filterTests applies level, category, and test ID filters.
func filterTests(tests []lib.TestCase, level int, category, testID string) []lib.TestCase {
	var filtered []lib.TestCase
	for _, tc := range tests {
		if level >= 0 && tc.Level != level {
			continue
		}
		if category != "" && tc.Category != category {
			continue
		}
		if testID != "" && tc.TestID != testID {
			continue
		}
		filtered = append(filtered, tc)
	}
	return filtered
}

// runTest executes a single test case and returns the result.
func runTest(tc lib.TestCase, client *OJSClient, rpcTimeout time.Duration, timingCfg lib.TimingConfig, verbose bool) lib.TestResult {
	start := time.Now()
	result := lib.TestResult{
		TestID:   tc.TestID,
		Name:     tc.Name,
		Level:    tc.Level,
		Category: tc.Category,
		SpecRef:  tc.SpecRef,
		FilePath: tc.FilePath,
	}

	stepResults := make(map[string]*lib.StepResult)

	// Run setup steps
	if tc.Setup != nil {
		for _, step := range tc.Setup.Steps {
			sr, failures := executeStep(step, client, rpcTimeout, stepResults, timingCfg)
			stepResults[step.ID] = sr
			if len(failures) > 0 {
				result.Status = "error"
				result.Failures = append(result.Failures, lib.Failure{
					StepID:  step.ID,
					Message: fmt.Sprintf("Setup step failed: %s", failures[0].Message),
				})
				result.DurationMs = time.Since(start).Milliseconds()
				return result
			}
		}
	}

	// Run test steps
	for _, step := range tc.Steps {
		sr, failures := executeStep(step, client, rpcTimeout, stepResults, timingCfg)
		stepResults[step.ID] = sr
		result.StepResults = append(result.StepResults, *sr)
		if len(failures) > 0 {
			result.Failures = append(result.Failures, failures...)
		}
	}

	// Run teardown steps
	if tc.Teardown != nil {
		for _, step := range tc.Teardown.Steps {
			sr, _ := executeStep(step, client, rpcTimeout, stepResults, timingCfg)
			stepResults[step.ID] = sr
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	if len(result.Failures) > 0 {
		result.Status = "fail"
	} else {
		result.Status = "pass"
	}
	return result
}

// executeStep runs a single gRPC step and evaluates its assertions.
func executeStep(step lib.Step, client *OJSClient, rpcTimeout time.Duration, stepResults map[string]*lib.StepResult, timingCfg lib.TimingConfig) (*lib.StepResult, []lib.Failure) {
	// Apply delay if specified
	if step.DelayMs > 0 {
		time.Sleep(time.Duration(step.DelayMs) * time.Millisecond)
	}

	// Handle WAIT action (pure delay, no RPC)
	if strings.EqualFold(step.Action, "WAIT") {
		waitMs := step.DurationMs
		if waitMs <= 0 {
			waitMs = step.DelayMs
		}
		if waitMs > 0 {
			time.Sleep(time.Duration(waitMs) * time.Millisecond)
		}
		return &lib.StepResult{StepID: step.ID}, nil
	}

	// Resolve template references in path
	path := resolveTemplates(step.Path, stepResults)

	// Resolve template references in body and parse to map
	var body map[string]any
	if step.Body != nil {
		bodyStr := resolveTemplates(string(step.Body), stepResults)
		_ = json.Unmarshal([]byte(bodyStr), &body)
	}

	// Resolve HTTP action+path to gRPC method
	method := ResolveRoute(step.Action, path)
	if method == "" {
		return &lib.StepResult{StepID: step.ID}, []lib.Failure{{
			StepID:  step.ID,
			Message: fmt.Sprintf("Cannot resolve gRPC method for %s %s", step.Action, path),
		}}
	}

	// Handle PauseOrResumeQueue disambiguation
	if method == "PauseOrResumeQueue" {
		if strings.HasSuffix(path, "/pause") {
			method = "PauseQueue"
		} else if strings.HasSuffix(path, "/resume") {
			method = "ResumeQueue"
		}
	}

	// Execute RPC
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	reqStart := time.Now()
	rpcResult, err := client.CallRPC(ctx, method, path, body)
	reqDuration := time.Since(reqStart)

	if err != nil {
		return &lib.StepResult{StepID: step.ID}, []lib.Failure{{
			StepID:  step.ID,
			Message: fmt.Sprintf("gRPC call failed: %v", err),
		}}
	}

	// Build step result
	var parsed map[string]any
	if len(rpcResult.ResponseJSON) > 0 {
		_ = json.Unmarshal(rpcResult.ResponseJSON, &parsed)
	}

	// Determine the HTTP status code equivalent.
	// Some RPCs override the default gRPC-to-HTTP mapping (e.g., Enqueue returns
	// 201 Created on success instead of 200 OK).
	httpStatus := GRPCCodeToHTTPStatus(rpcResult.GRPCCode)
	if rpcResult.HTTPStatusOverride > 0 && rpcResult.GRPCCode == codes.OK {
		httpStatus = rpcResult.HTTPStatusOverride
	}

	sr := &lib.StepResult{
		StepID:     step.ID,
		StatusCode: httpStatus,
		Body:       json.RawMessage(rpcResult.ResponseJSON),
		DurationMs: reqDuration.Milliseconds(),
		Parsed:     parsed,
	}

	// Evaluate assertions
	var failures []lib.Failure
	if step.Assertions != nil {
		failures = evaluateAssertions(step, sr, stepResults, timingCfg)
	}

	return sr, failures
}

// evaluateAssertions checks all assertions for a step result.
func evaluateAssertions(step lib.Step, sr *lib.StepResult, stepResults map[string]*lib.StepResult, timingCfg lib.TimingConfig) []lib.Failure {
	var failures []lib.Failure
	a := step.Assertions

	// Status code assertion
	if len(a.Status) > 0 {
		if err := evaluateStatusAssertion(a.Status, sr.StatusCode); err != nil {
			failures = append(failures, lib.Failure{
				StepID:   step.ID,
				Field:    "status",
				Expected: string(a.Status),
				Actual:   fmt.Sprintf("%d", sr.StatusCode),
				Message:  err.Error(),
			})
		}
	}

	// Status code in list assertion
	if len(a.StatusIn) > 0 {
		found := false
		for _, s := range a.StatusIn {
			if sr.StatusCode == s {
				found = true
				break
			}
		}
		if !found {
			failures = append(failures, lib.Failure{
				StepID:   step.ID,
				Field:    "status",
				Expected: fmt.Sprintf("one of %v", a.StatusIn),
				Actual:   fmt.Sprintf("%d", sr.StatusCode),
				Message:  fmt.Sprintf("Expected status in %v, got %d", a.StatusIn, sr.StatusCode),
			})
		}
	}

	// Body assertions using JSON path
	if a.Body != nil {
		// Handle special top-level $or operator
		if orRaw, ok := a.Body["$or"]; ok {
			var alternatives []json.RawMessage
			if json.Unmarshal(orRaw, &alternatives) == nil {
				matched := false
				for _, alt := range alternatives {
					var altBody map[string]json.RawMessage
					if json.Unmarshal(alt, &altBody) == nil {
						altFailed := false
						for p, m := range altBody {
							resolvedMatcher := resolveMatcherTemplates(m, stepResults)
							val, err := lib.ResolveJSONPath(p, sr.Parsed)
							if err != nil || lib.MatchAssertion(resolvedMatcher, val) != nil {
								altFailed = true
								break
							}
						}
						if !altFailed {
							matched = true
							break
						}
					}
				}
				if !matched {
					failures = append(failures, lib.Failure{
						StepID:  step.ID,
						Field:   "$or",
						Message: "No $or alternative matched",
					})
				}
			}
		}

		if sr.Parsed != nil {
			for path, matcher := range a.Body {
				// Skip special top-level operators (but NOT $.path expressions)
				if strings.HasPrefix(path, "$") && !strings.HasPrefix(path, "$.") {
					continue
				}

				resolvedPath := resolveTemplates(path, stepResults)
				resolvedMatcher := resolveMatcherTemplates(matcher, stepResults)

				val, err := lib.ResolveJSONPath(resolvedPath, sr.Parsed)
				if err != nil {
					var matcherStr string
					if json.Unmarshal(resolvedMatcher, &matcherStr) == nil && matcherStr == "absent" {
						continue
					}
					failures = append(failures, lib.Failure{
						StepID:  step.ID,
						Field:   path,
						Message: fmt.Sprintf("Failed to resolve path %q: %v", path, err),
					})
					continue
				}

				if err := lib.MatchAssertion(resolvedMatcher, val); err != nil {
					actualStr := "null"
					if val != nil {
						b, _ := json.Marshal(val)
						actualStr = string(b)
					}
					failures = append(failures, lib.Failure{
						StepID:   step.ID,
						Field:    path,
						Expected: string(resolvedMatcher),
						Actual:   actualStr,
						Message:  fmt.Sprintf("Assertion failed at %q: %v", path, err),
					})
				}
			}
		}
	}

	// Body absent assertions
	for _, path := range a.BodyAbsent {
		val, _ := lib.ResolveJSONPath(path, sr.Parsed)
		if val != nil {
			failures = append(failures, lib.Failure{
				StepID:  step.ID,
				Field:   path,
				Message: fmt.Sprintf("Expected field %q to be absent", path),
			})
		}
	}

	// Timing assertions
	if a.TimingMs != nil {
		if a.TimingMs.LessThan != nil {
			if sr.DurationMs >= int64(*a.TimingMs.LessThan) {
				failures = append(failures, lib.Failure{
					StepID:   step.ID,
					Field:    "timing",
					Expected: fmt.Sprintf("< %dms", *a.TimingMs.LessThan),
					Actual:   fmt.Sprintf("%dms", sr.DurationMs),
					Message:  fmt.Sprintf("Expected response in < %dms, took %dms", *a.TimingMs.LessThan, sr.DurationMs),
				})
			}
		}
		if a.TimingMs.GreaterThan != nil {
			if sr.DurationMs <= int64(*a.TimingMs.GreaterThan) {
				failures = append(failures, lib.Failure{
					StepID:   step.ID,
					Field:    "timing",
					Expected: fmt.Sprintf("> %dms", *a.TimingMs.GreaterThan),
					Actual:   fmt.Sprintf("%dms", sr.DurationMs),
					Message:  fmt.Sprintf("Expected response in > %dms, took %dms", *a.TimingMs.GreaterThan, sr.DurationMs),
				})
			}
		}
		if a.TimingMs.Approximate != nil {
			if err := timingCfg.AssertApproximateMs(float64(*a.TimingMs.Approximate), float64(sr.DurationMs)); err != nil {
				failures = append(failures, lib.Failure{
					StepID:  step.ID,
					Field:   "timing",
					Message: err.Error(),
				})
			}
		}
	}

	// Body contains assertions (substring check on raw body)
	for _, substr := range a.BodyContains {
		if !strings.Contains(string(sr.Body), substr) {
			failures = append(failures, lib.Failure{
				StepID:   step.ID,
				Field:    "body_contains",
				Expected: fmt.Sprintf("body containing %q", substr),
				Message:  fmt.Sprintf("Response body does not contain %q", substr),
			})
		}
	}

	return failures
}

// resolveTemplates replaces {{steps.step-id.response.body.field}} references.
func resolveTemplates(input string, stepResults map[string]*lib.StepResult) string {
	return templateRefPattern.ReplaceAllStringFunc(input, func(match string) string {
		parts := templateRefPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		stepID := parts[1]
		fieldPath := parts[2]

		sr, ok := stepResults[stepID]
		if !ok || sr.Parsed == nil {
			return match
		}

		val, err := lib.ResolveJSONPath(fieldPath, sr.Parsed)
		if err != nil || val == nil {
			return match
		}

		switch v := val.(type) {
		case string:
			return v
		case float64:
			if v == float64(int64(v)) {
				return fmt.Sprintf("%d", int64(v))
			}
			return fmt.Sprintf("%v", v)
		default:
			b, _ := json.Marshal(v)
			return string(b)
		}
	})
}

// resolveMatcherTemplates resolves template references within a JSON assertion matcher value.
func resolveMatcherTemplates(matcher json.RawMessage, stepResults map[string]*lib.StepResult) json.RawMessage {
	s := string(matcher)
	if !strings.Contains(s, "{{steps.") {
		return matcher
	}
	resolved := resolveTemplates(s, stepResults)
	if resolved != s {
		return json.RawMessage(resolved)
	}
	return matcher
}

func buildReport(results []lib.TestResult, target string, requestedLevel int, duration time.Duration) lib.SuiteReport {
	summary := lib.ResultsSummary{ByLevel: map[int]lib.LevelSummary{}}
	report := lib.SuiteReport{
		TestSuiteVersion: suiteVersion,
		Target:           target,
		RunAt:            time.Now().UTC().Format(time.RFC3339),
		DurationMs:       duration.Milliseconds(),
		RequestedLevel:   requestedLevel,
		Results:          summary,
		Conformant:       false,
		ConformantLevel:  -1,
	}

	for _, tr := range results {
		summary.Total++
		levelSummary := summary.ByLevel[tr.Level]
		levelSummary.Total++

		switch tr.Status {
		case "pass":
			summary.Passed++
			levelSummary.Passed++
		case "skip":
			summary.Skipped++
			levelSummary.Skipped++
			report.Skipped = append(report.Skipped, tr)
		case "error":
			summary.Errored++
			levelSummary.Errored++
			report.Failures = append(report.Failures, tr)
		default:
			summary.Failed++
			levelSummary.Failed++
			report.Failures = append(report.Failures, tr)
		}

		levelSummary.AllPass = levelSummary.Total > 0 && levelSummary.Passed == levelSummary.Total
		summary.ByLevel[tr.Level] = levelSummary
	}

	report.Results = summary
	if summary.Total > 0 && summary.Passed == summary.Total {
		report.Conformant = true
	}

	for level := 0; level <= 4; level++ {
		ls, ok := summary.ByLevel[level]
		if !ok || !ls.AllPass {
			break
		}
		report.ConformantLevel = level
	}

	return report
}

func outputTable(report lib.SuiteReport, results []lib.TestResult, verbose bool) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  OJS gRPC Conformance Test Results")
	fmt.Println("========================================")
	fmt.Printf("  Target:    %s\n", report.Target)
	fmt.Printf("  Suite:     v%s\n", report.TestSuiteVersion)
	fmt.Printf("  Run at:    %s\n", report.RunAt)
	fmt.Printf("  Duration:  %dms\n", report.DurationMs)
	fmt.Println("----------------------------------------")

	fmt.Println()
	fmt.Printf("  %-14s %-40s %-8s %s\n", "TEST ID", "NAME", "STATUS", "DURATION")
	fmt.Printf("  %-14s %-40s %-8s %s\n", strings.Repeat("-", 14), strings.Repeat("-", 40), strings.Repeat("-", 8), strings.Repeat("-", 10))

	for _, r := range results {
		status := r.Status
		switch status {
		case "pass":
			status = "PASS"
		case "fail":
			status = "FAIL"
		case "skip":
			status = "SKIP"
		case "error":
			status = "ERR"
		}

		name := r.Name
		if len(name) > 40 {
			name = name[:37] + "..."
		}

		fmt.Printf("  %-14s %-40s %-8s %dms\n", r.TestID, name, status, r.DurationMs)

		if r.Status == "fail" || r.Status == "error" {
			for _, f := range r.Failures {
				fmt.Printf("    -> [%s] %s\n", f.StepID, f.Message)
				if verbose && f.Expected != "" {
					fmt.Printf("       Expected: %s\n", f.Expected)
					fmt.Printf("       Actual:   %s\n", f.Actual)
				}
			}
		}
	}

	// Level summary
	fmt.Println()
	fmt.Println("  Level Summary:")
	fmt.Printf("  %-8s %-15s %6s %6s %6s %6s %8s\n", "LEVEL", "NAME", "TOTAL", "PASS", "FAIL", "SKIP", "STATUS")
	fmt.Printf("  %-8s %-15s %6s %6s %6s %6s %8s\n", "-----", "----", "-----", "----", "----", "----", "------")

	for lvl := 0; lvl <= 4; lvl++ {
		ls, exists := report.Results.ByLevel[lvl]
		if !exists {
			continue
		}
		status := "PASS"
		if !ls.AllPass {
			status = "FAIL"
		}
		fmt.Printf("  %-8d %-15s %6d %6d %6d %6d %8s\n",
			lvl, lib.LevelName(lvl), ls.Total, ls.Passed, ls.Failed, ls.Skipped, status)
	}

	// Summary
	fmt.Println()
	fmt.Println("  ----------------------------------------")
	fmt.Printf("  Total: %d | Passed: %d | Failed: %d | Skipped: %d | Errored: %d\n",
		report.Results.Total, report.Results.Passed, report.Results.Failed,
		report.Results.Skipped, report.Results.Errored)

	if report.Conformant {
		fmt.Printf("  Result: CONFORMANT (Level %d - %s)\n", report.ConformantLevel, lib.LevelName(report.ConformantLevel))
	} else {
		if report.ConformantLevel >= 0 {
			fmt.Printf("  Result: PARTIAL CONFORMANCE (Level %d - %s)\n", report.ConformantLevel, lib.LevelName(report.ConformantLevel))
		} else {
			fmt.Println("  Result: NOT CONFORMANT")
		}
	}
	fmt.Println("========================================")
	fmt.Println()

	if len(report.Failures) > 0 {
		fmt.Printf("  Failed Tests (%d):\n", len(report.Failures))
		for _, f := range report.Failures {
			fmt.Printf("    - %s: %s [%s]\n", f.TestID, f.Name, f.SpecRef)
		}
		fmt.Println()
	}
}
