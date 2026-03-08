// Package lib provides shared types and utilities for the OJS conformance test runner.
package lib

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// TestCase represents a single conformance test loaded from a JSON file.
type TestCase struct {
	TestID      string          `json:"test_id"`
	Level       json.RawMessage `json:"level"`
	Category    string          `json:"category"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	SpecRef     string          `json:"spec_ref"`
	Tags        []string        `json:"tags"`
	Setup       *Setup          `json:"setup,omitempty"`
	Steps       []Step          `json:"steps"`
	Teardown    *Setup          `json:"teardown,omitempty"`
	FilePath    string          `json:"-"`

	LevelInt int `json:"-"` // parsed from Level after unmarshaling
}

// ParseLevel extracts the integer level from the raw JSON value,
// handling both int and string representations. Extension suites
// use "ext" which maps to level 99.
func (tc *TestCase) ParseLevel() (int, error) {
	if len(tc.Level) == 0 {
		return 0, nil
	}

	// Try int
	var n int
	if err := json.Unmarshal(tc.Level, &n); err == nil {
		tc.LevelInt = n
		return n, nil
	}

	// Try string
	var str string
	if err := json.Unmarshal(tc.Level, &str); err == nil {
		if str == "ext" || str == "extension" {
			tc.LevelInt = 99
			return 99, nil
		}
		n, err := strconv.Atoi(str)
		if err != nil {
			return 0, fmt.Errorf("level string %q is not a number", str)
		}
		tc.LevelInt = n
		return n, nil
	}

	return 0, fmt.Errorf("cannot parse level from %s", strings.TrimSpace(string(tc.Level)))
}

// Setup contains optional setup or teardown configuration.
type Setup struct {
	Steps []Step `json:"steps,omitempty"`
}

// Step represents a single HTTP interaction in a test.
type Step struct {
	ID           string            `json:"id"`
	Action       string            `json:"action"`
	Intent       string            `json:"intent,omitempty"`
	Path         string            `json:"path"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         json.RawMessage   `json:"body,omitempty"`
	DelayMs      int               `json:"delay_ms,omitempty"`
	DurationMs   int               `json:"duration_ms,omitempty"`
	Assertions   *Assertions       `json:"assertions,omitempty"`
	Description  string            `json:"description,omitempty"`
	ParallelWith string            `json:"parallel_with,omitempty"`
}

// Assertions defines expected outcomes for a step.
type Assertions struct {
	Status       json.RawMessage            `json:"status,omitempty"`
	StatusIn     []int                      `json:"status_in,omitempty"`
	Body         map[string]json.RawMessage `json:"body,omitempty"`
	BodyAbsent   []string                   `json:"body_absent,omitempty"`
	Headers      json.RawMessage            `json:"headers,omitempty"`
	TimingMs     *TimingAssertion           `json:"timing_ms,omitempty"`
	BodyRaw      json.RawMessage            `json:"body_raw,omitempty"`
	BodyContains []string                   `json:"body_contains,omitempty"`
}

// ParsedHeaders returns headers as simple string map, extracting $match values
// from object-format headers. This handles both:
//
//	"headers": {"Content-Type": "application/json"}                     (simple)
//	"headers": {"Content-Type": {"$match": "application/(openjobspec\\+)?json"}}  (advanced)
func (a *Assertions) ParsedHeaders() map[string]string {
	if len(a.Headers) == 0 {
		return nil
	}
	result := make(map[string]string)

	// Try simple map[string]string first
	var simple map[string]string
	if err := json.Unmarshal(a.Headers, &simple); err == nil {
		return simple
	}

	// Fall back to map[string]object with $match
	var complex map[string]json.RawMessage
	if err := json.Unmarshal(a.Headers, &complex); err == nil {
		for key, val := range complex {
			var strVal string
			if json.Unmarshal(val, &strVal) == nil {
				result[key] = strVal
				continue
			}
			var objVal map[string]string
			if json.Unmarshal(val, &objVal) == nil {
				if m, ok := objVal["$match"]; ok {
					result[key] = m
				} else if e, ok := objVal["$eq"]; ok {
					result[key] = e
				}
			}
		}
	}
	return result
}

// HeaderMatcher represents a header assertion with optional regex support.
type HeaderMatcher struct {
	Value   string
	IsRegex bool
}

// ParsedHeaderMatchers returns header matchers that distinguish exact vs regex patterns.
func (a *Assertions) ParsedHeaderMatchers() map[string]HeaderMatcher {
	if len(a.Headers) == 0 {
		return nil
	}
	result := make(map[string]HeaderMatcher)

	// Try simple map[string]string first
	var simple map[string]string
	if err := json.Unmarshal(a.Headers, &simple); err == nil {
		for k, v := range simple {
			result[k] = HeaderMatcher{Value: v, IsRegex: false}
		}
		return result
	}

	// Fall back to map[string]object with $match/$eq
	var complex map[string]json.RawMessage
	if err := json.Unmarshal(a.Headers, &complex); err == nil {
		for key, val := range complex {
			var strVal string
			if json.Unmarshal(val, &strVal) == nil {
				result[key] = HeaderMatcher{Value: strVal, IsRegex: false}
				continue
			}
			var objVal map[string]string
			if json.Unmarshal(val, &objVal) == nil {
				if m, ok := objVal["$match"]; ok {
					result[key] = HeaderMatcher{Value: m, IsRegex: true}
				} else if e, ok := objVal["$eq"]; ok {
					result[key] = HeaderMatcher{Value: e, IsRegex: false}
				}
			}
		}
	}
	return result
}
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

