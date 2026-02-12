// Package lib provides shared types and utilities for the OJS conformance test runner.
package lib

import (
	"encoding/json"
	"net/http"
)

// TestCase represents a single conformance test loaded from a JSON file.
type TestCase struct {
	TestID      string   `json:"test_id"`
	Level       int      `json:"level"`
	Category    string   `json:"category"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	SpecRef     string   `json:"spec_ref"`
	Tags        []string `json:"tags"`
	Setup       *Setup   `json:"setup,omitempty"`
	Steps       []Step   `json:"steps"`
	Teardown    *Setup   `json:"teardown,omitempty"`
	FilePath    string   `json:"-"` // populated by the runner, not from JSON
}

// Setup contains optional setup or teardown configuration.
type Setup struct {
	Steps []Step `json:"steps,omitempty"`
}

// Step represents a single HTTP interaction in a test.
type Step struct {
	ID          string            `json:"id"`
	Action      string            `json:"action"`
	Intent      string            `json:"intent,omitempty"`
	Path        string            `json:"path"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        json.RawMessage   `json:"body,omitempty"`
	DelayMs     int               `json:"delay_ms,omitempty"`
	DurationMs  int               `json:"duration_ms,omitempty"`
	Assertions  *Assertions       `json:"assertions,omitempty"`
	Description string            `json:"description,omitempty"`
}

// Assertions defines expected outcomes for a step.
type Assertions struct {
	Status       json.RawMessage            `json:"status,omitempty"`
	StatusIn     []int                      `json:"status_in,omitempty"`
	Body         map[string]json.RawMessage `json:"body,omitempty"`
	BodyAbsent   []string                   `json:"body_absent,omitempty"`
	Headers      map[string]string          `json:"headers,omitempty"`
	TimingMs     *TimingAssertion           `json:"timing_ms,omitempty"`
	BodyRaw      json.RawMessage            `json:"body_raw,omitempty"`
	BodyContains []string                   `json:"body_contains,omitempty"`
}

// TimingAssertion validates response time.
type TimingAssertion struct {
	LessThan    *int `json:"less_than,omitempty"`
	GreaterThan *int `json:"greater_than,omitempty"`
	Approximate *int `json:"approximate,omitempty"`
}

// StepResult holds the result of executing a single step.
type StepResult struct {
	StepID     string              `json:"step_id"`
	StatusCode int                 `json:"status_code"`
	Headers    http.Header `json:"headers"`
	Body       json.RawMessage     `json:"body"`
	DurationMs int64               `json:"duration_ms"`
	Parsed     map[string]any      `json:"-"` // parsed JSON body
}

// TestResult holds the outcome of running a single test case.
type TestResult struct {
	TestID      string        `json:"test_id"`
	Name        string        `json:"name"`
	Level       int           `json:"level"`
	Category    string        `json:"category"`
	SpecRef     string        `json:"spec_ref"`
	Status      string        `json:"status"` // "pass", "fail", "skip", "error"
	DurationMs  int64         `json:"duration_ms"`
	Failures    []Failure     `json:"failures,omitempty"`
	StepResults []StepResult  `json:"step_results,omitempty"`
	FilePath    string        `json:"file_path"`
}

// Failure describes a single assertion failure within a test.
type Failure struct {
	StepID   string `json:"step_id"`
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Message  string `json:"message"`
}

// SuiteReport is the top-level conformance report output.
type SuiteReport struct {
	TestSuiteVersion string          `json:"test_suite_version"`
	Target           string          `json:"target"`
	RunAt            string          `json:"run_at"`
	DurationMs       int64           `json:"duration_ms"`
	RequestedLevel   int             `json:"requested_level"`
	Results          ResultsSummary  `json:"results"`
	Conformant       bool            `json:"conformant"`
	ConformantLevel  int             `json:"conformant_level"`
	Failures         []TestResult    `json:"failures,omitempty"`
	Skipped          []TestResult    `json:"skipped,omitempty"`
}

// ResultsSummary contains aggregate test results.
type ResultsSummary struct {
	Total    int                    `json:"total"`
	Passed   int                    `json:"passed"`
	Failed   int                    `json:"failed"`
	Skipped  int                    `json:"skipped"`
	Errored  int                    `json:"errored"`
	ByLevel  map[int]LevelSummary   `json:"by_level"`
}

// LevelSummary contains results for a single conformance level.
type LevelSummary struct {
	Total   int  `json:"total"`
	Passed  int  `json:"passed"`
	Failed  int  `json:"failed"`
	Skipped int  `json:"skipped"`
	Errored int  `json:"errored"`
	AllPass bool `json:"all_pass"`
}

// LevelName returns the human-readable name for a conformance level.
func LevelName(level int) string {
	switch level {
	case 0:
		return "Core"
	case 1:
		return "Reliable"
	case 2:
		return "Scheduled"
	case 3:
		return "Orchestration"
	case 4:
		return "Advanced"
	default:
		return "Unknown"
	}
}
