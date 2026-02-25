// Package main implements the OJS conformance HTTP test runner.
//
// Usage:
//
//	ojs-conformance-runner -url http://localhost:8080 -suites ./suites
//	ojs-conformance-runner -url http://localhost:8080 -suites ./suites -level 1
//	ojs-conformance-runner -url http://localhost:8080 -suites ./suites -category retry
//	ojs-conformance-runner -url http://localhost:8080 -suites ./suites -test L1-RET-001
//	ojs-conformance-runner -url http://localhost:8080 -suites ./suites -output json
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openjobspec/ojs-conformance/lib"
	"github.com/redis/go-redis/v9"
)

const (
	suiteVersion = "1.0"
	ojsMediaType = "application/openjobspec+json"
)

var templateRefPattern = regexp.MustCompile(`\{\{steps\.([^.]+)\.response\.body\.([^}]+)\}\}`)

func main() {
	var (
		baseURL      string
		suitesDir    string
		level        int
		category     string
		testID       string
		outputFormat string
		verbose      bool
		tolerancePct float64
		timeoutSec   int
		redisURL     string
	)

	flag.StringVar(&baseURL, "url", "http://localhost:8080", "Base URL of the OJS-conformant server")
	flag.StringVar(&suitesDir, "suites", "./suites", "Path to test suite directory")
	flag.IntVar(&level, "level", -1, "Filter by conformance level (0-4), -1 for all")
	flag.StringVar(&category, "category", "", "Filter by category (e.g., envelope, retry)")
	flag.StringVar(&testID, "test", "", "Run a single test by ID (e.g., L0-ENV-001)")
	flag.StringVar(&outputFormat, "output", "table", "Output format: table or json")
	flag.BoolVar(&verbose, "verbose", false, "Show detailed step results")
	flag.Float64Var(&tolerancePct, "tolerance", 50, "Timing tolerance percentage")
	flag.IntVar(&timeoutSec, "timeout", 30, "HTTP request timeout in seconds")
	flag.StringVar(&redisURL, "redis", "", "Redis URL for FLUSHDB between tests (e.g., redis://localhost:6379)")
	flag.Parse()

	// Normalize base URL
	baseURL = strings.TrimRight(baseURL, "/")

	// Load test cases
	tests, err := loadTests(suitesDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading tests: %v\n", err)
		os.Exit(2)
	}

	// Filter tests
	tests = filterTests(tests, level, category, testID)
	if len(tests) == 0 {
		fmt.Fprintln(os.Stderr, "No tests match the specified filters.")
		os.Exit(2)
	}

	// Configure HTTP client
	client := &http.Client{
		Timeout: time.Duration(timeoutSec) * time.Second,
	}

	timingCfg := lib.TimingConfig{
		TolerancePct:   tolerancePct,
		MinToleranceMs: 100,
		MaxWaitMs:      30000,
	}

	// Set up optional Redis client for test isolation
	var redisClient *redis.Client
	if redisURL != "" {
		opts, err := redis.ParseURL(redisURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing Redis URL: %v\n", err)
			os.Exit(2)
		}
		redisClient = redis.NewClient(opts)
		if err := redisClient.Ping(context.Background()).Err(); err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to Redis: %v\n", err)
			os.Exit(2)
		}
		defer redisClient.Close()
	}

	// Run tests
	suiteStart := time.Now()
	var results []lib.TestResult

	for _, tc := range tests {
		// Flush Redis between tests for isolation
		if redisClient != nil {
			redisClient.FlushDB(context.Background())
		}
		result := runTest(tc, baseURL, client, timingCfg, verbose)
		results = append(results, result)
	}

	suiteDuration := time.Since(suiteStart)

	// Build report
	report := buildReport(results, baseURL, level, suiteDuration)

	// Output results
	switch outputFormat {
	case "json":
		outputJSON(report)
	default:
		outputTable(report, results, verbose)
	}

	// Exit code
	if report.Conformant {
		os.Exit(0)
	} else {
		os.Exit(1)
	}
}

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

	// Sort by test_id for deterministic ordering
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
func runTest(tc lib.TestCase, baseURL string, client *http.Client, timingCfg lib.TimingConfig, verbose bool) lib.TestResult {
	start := time.Now()
	result := lib.TestResult{
		TestID:   tc.TestID,
		Name:     tc.Name,
		Level:    tc.Level,
		Category: tc.Category,
		SpecRef:  tc.SpecRef,
		FilePath: tc.FilePath,
	}

	// Store step results for template resolution
	stepResults := make(map[string]*lib.StepResult)

	// Run setup steps if any
	if tc.Setup != nil {
		for _, step := range tc.Setup.Steps {
			sr, failures := executeStep(step, baseURL, client, stepResults, timingCfg)
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
		sr, failures := executeStep(step, baseURL, client, stepResults, timingCfg)
		stepResults[step.ID] = sr
		result.StepResults = append(result.StepResults, *sr)

		if len(failures) > 0 {
			result.Failures = append(result.Failures, failures...)
		}
	}

	// Run teardown steps if any
	if tc.Teardown != nil {
		for _, step := range tc.Teardown.Steps {
			sr, _ := executeStep(step, baseURL, client, stepResults, timingCfg)
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

// executeStep runs a single HTTP step and evaluates its assertions.
func executeStep(step lib.Step, baseURL string, client *http.Client, stepResults map[string]*lib.StepResult, timingCfg lib.TimingConfig) (*lib.StepResult, []lib.Failure) {
	// Apply delay if specified
	if step.DelayMs > 0 {
		time.Sleep(time.Duration(step.DelayMs) * time.Millisecond)
	}

	// Handle WAIT action (pure delay, no HTTP request)
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

	// Resolve template references in body
	var body io.Reader
	if step.Body != nil {
		bodyStr := resolveTemplates(string(step.Body), stepResults)
		body = strings.NewReader(bodyStr)
	}

	// Build request
	url := baseURL + path
	req, err := http.NewRequest(step.Action, url, body)
	if err != nil {
		return &lib.StepResult{StepID: step.ID}, []lib.Failure{{
			StepID:  step.ID,
			Message: fmt.Sprintf("Failed to create request: %v", err),
		}}
	}

	// Set headers
	if step.Headers != nil {
		for k, v := range step.Headers {
			req.Header.Set(k, v)
		}
	}
	// Default content type
	if req.Header.Get("Content-Type") == "" && body != nil {
		req.Header.Set("Content-Type", ojsMediaType)
	}

	// Execute request
	reqStart := time.Now()
	resp, err := client.Do(req)
	reqDuration := time.Since(reqStart)

	if err != nil {
		return &lib.StepResult{StepID: step.ID}, []lib.Failure{{
			StepID:  step.ID,
			Message: fmt.Sprintf("HTTP request failed: %v", err),
		}}
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &lib.StepResult{StepID: step.ID}, []lib.Failure{{
			StepID:  step.ID,
			Message: fmt.Sprintf("Failed to read response body: %v", err),
		}}
	}

	// Parse response body
	var parsed map[string]any
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &parsed)
	}

	sr := &lib.StepResult{
		StepID:     step.ID,
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       json.RawMessage(respBody),
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

	// Status code assertion (supports int, string matchers, and object matchers)
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
		// Handle special top-level operators like $or and $empty
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

				// Resolve template references in assertion paths AND matchers
				resolvedPath := resolveTemplates(path, stepResults)
				resolvedMatcher := resolveMatcherTemplates(matcher, stepResults)

				val, err := lib.ResolveJSONPath(resolvedPath, sr.Parsed)
				if err != nil {
					// Check if matcher is "absent" - field not found is ok
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

	// Header assertions
	if a.Headers != nil {
		for key, expected := range a.Headers {
			actual := sr.Headers.Get(key)
			if actual != expected {
				failures = append(failures, lib.Failure{
					StepID:   step.ID,
					Field:    fmt.Sprintf("header:%s", key),
					Expected: expected,
					Actual:   actual,
					Message:  fmt.Sprintf("Expected header %q=%q, got %q", key, expected, actual),
				})
			}
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

// resolveMatcherTemplates resolves {{steps.step-id.response.body.field}} references
// within a JSON assertion matcher value.
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

// buildReport aggregates test results into a conformance report.
func buildReport(results []lib.TestResult, target string, requestedLevel int, duration time.Duration) lib.SuiteReport {
	report := lib.SuiteReport{
		TestSuiteVersion: suiteVersion,
		Target:           target,
		RunAt:            time.Now().UTC().Format(time.RFC3339),
		DurationMs:       duration.Milliseconds(),
		RequestedLevel:   requestedLevel,
		Results: lib.ResultsSummary{
			Total:   len(results),
			ByLevel: make(map[int]lib.LevelSummary),
		},
	}

	for _, r := range results {
		report.Results.Total = len(results)
		ls := report.Results.ByLevel[r.Level]
		ls.Total++

		switch r.Status {
		case "pass":
			report.Results.Passed++
			ls.Passed++
		case "fail":
			report.Results.Failed++
			ls.Failed++
			report.Failures = append(report.Failures, r)
		case "skip":
			report.Results.Skipped++
			ls.Skipped++
			report.Skipped = append(report.Skipped, r)
		case "error":
			report.Results.Errored++
			ls.Errored++
			report.Failures = append(report.Failures, r)
		}

		report.Results.ByLevel[r.Level] = ls
	}

	// Determine conformance
	report.ConformantLevel = -1
	for lvl := 0; lvl <= 4; lvl++ {
		ls, exists := report.Results.ByLevel[lvl]
		if !exists {
			continue
		}
		ls.AllPass = ls.Failed == 0 && ls.Errored == 0
		report.Results.ByLevel[lvl] = ls
		if ls.AllPass && ls.Total > 0 {
			report.ConformantLevel = lvl
		} else {
			break
		}
	}

	report.Conformant = report.Results.Failed == 0 && report.Results.Errored == 0

	return report
}

// outputJSON writes the report as JSON to stdout.
func outputJSON(report lib.SuiteReport) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(report)
}

// outputTable writes a human-readable table to stdout.
func outputTable(report lib.SuiteReport, results []lib.TestResult, verbose bool) {
	// Header
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("  OJS Conformance Test Results")
	fmt.Println("========================================")
	fmt.Printf("  Target:    %s\n", report.Target)
	fmt.Printf("  Suite:     v%s\n", report.TestSuiteVersion)
	fmt.Printf("  Run at:    %s\n", report.RunAt)
	fmt.Printf("  Duration:  %dms\n", report.DurationMs)
	fmt.Println("----------------------------------------")

	// Results table
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

		// Show failures in verbose mode or always for failed tests
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

	// Show failed test details
	if len(report.Failures) > 0 {
		fmt.Printf("  Failed Tests (%d):\n", len(report.Failures))
		for _, f := range report.Failures {
			fmt.Printf("    - %s: %s [%s]\n", f.TestID, f.Name, f.SpecRef)
		}
		fmt.Println()
	}
}

// evaluateStatusAssertion handles various status assertion formats:
// - integer: exact match (e.g., 200)
// - string: matcher like "number:range(400,422)"
// - object: {"$in": [200, 409]}
func evaluateStatusAssertion(raw json.RawMessage, actual int) error {
	// Try as integer
	var statusInt int
	if err := json.Unmarshal(raw, &statusInt); err == nil {
		if actual != statusInt {
			return fmt.Errorf("Expected status %d, got %d", statusInt, actual)
		}
		return nil
	}

	// Try as string matcher
	var statusStr string
	if err := json.Unmarshal(raw, &statusStr); err == nil {
		// Handle one_of:code1,code2,... matcher
		if strings.HasPrefix(statusStr, "one_of:") {
			codesStr := statusStr[len("one_of:"):]
			codes := strings.Split(codesStr, ",")
			for _, codeStr := range codes {
				codeStr = strings.TrimSpace(codeStr)
				code, err := strconv.Atoi(codeStr)
				if err != nil {
					return fmt.Errorf("invalid status code %q in one_of matcher", codeStr)
				}
				if actual == code {
					return nil
				}
			}
			return fmt.Errorf("expected status one of [%s], got %d", codesStr, actual)
		}
		return lib.MatchAssertion(raw, float64(actual))
	}

	// Try as object (e.g., {"$in": [200, 409]})
	var statusObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &statusObj); err == nil {
		if inRaw, ok := statusObj["$in"]; ok {
			var inList []int
			if err := json.Unmarshal(inRaw, &inList); err == nil {
				for _, s := range inList {
					if actual == s {
						return nil
					}
				}
				return fmt.Errorf("Expected status in %v, got %d", inList, actual)
			}
		}
	}

	return fmt.Errorf("Unknown status assertion format: %s", string(raw))
}

// prettyJSON formats JSON for display.
func prettyJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "    ", "  "); err != nil {
		return string(data)
	}
	return buf.String()
}

