package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/openjobspec/ojs-conformance/lib"
)

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

// runTest is currently a scaffold implementation for the gRPC runner.
func runTest(tc lib.TestCase, _ *OJSClient, _ time.Duration, _ lib.TimingConfig, _ bool) lib.TestResult {
	return lib.TestResult{
		TestID:     tc.TestID,
		Name:       tc.Name,
		Level:      tc.Level,
		Category:   tc.Category,
		SpecRef:    tc.SpecRef,
		Status:     "skip",
		DurationMs: 0,
		FilePath:   tc.FilePath,
		Failures: []lib.Failure{{
			Message: "gRPC conformance runner scaffold: execution engine not implemented yet",
		}},
	}
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

func outputTable(report lib.SuiteReport, _ []lib.TestResult, verbose bool) {
	fmt.Printf("\nOJS gRPC Conformance Runner (Scaffold)\n")
	fmt.Printf("Target: %s\n", report.Target)
	fmt.Printf("Total: %d | Passed: %d | Failed: %d | Skipped: %d | Errored: %d\n",
		report.Results.Total,
		report.Results.Passed,
		report.Results.Failed,
		report.Results.Skipped,
		report.Results.Errored,
	)
	if verbose {
		fmt.Println("\nNote: all tests are currently marked as skipped while the gRPC execution engine is under development.")
	}
}
