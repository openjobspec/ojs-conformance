package lib

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const (
	// DefaultTimingTolerancePct is the default tolerance for approximate timing assertions.
	DefaultTimingTolerancePct = 50
)

var (
	uuidPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	uuidV7Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	// RFC 3339 pattern (simplified, doesn't validate all edge cases)
	datetimePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)
	rangePattern    = regexp.MustCompile(`^number:range\((-?\d+(?:\.\d+)?),\s*(-?\d+(?:\.\d+)?)\)$`)
	lengthPattern   = regexp.MustCompile(`^array:length\((\d+)\)$`)
	approxPattern   = regexp.MustCompile(`^~(\d+(?:\.\d+)?)$`)
)

// MatchAssertion checks if a value matches an assertion matcher string.
// Returns nil if the assertion passes, or an error describing the mismatch.
func MatchAssertion(matcher json.RawMessage, actual any) error {
	// Try to unmarshal as a string matcher first
	var matcherStr string
	if err := json.Unmarshal(matcher, &matcherStr); err == nil {
		return matchStringAssertion(matcherStr, actual)
	}

	// Try as a number
	var matcherNum float64
	if err := json.Unmarshal(matcher, &matcherNum); err == nil {
		return matchNumberAssertion(matcherNum, actual)
	}

	// Try as a boolean
	var matcherBool bool
	if err := json.Unmarshal(matcher, &matcherBool); err == nil {
		actualBool, ok := actual.(bool)
		if !ok {
			return fmt.Errorf("expected boolean %v, got %T: %v", matcherBool, actual, actual)
		}
		if actualBool != matcherBool {
			return fmt.Errorf("expected %v, got %v", matcherBool, actualBool)
		}
		return nil
	}

	// Try as null
	if string(matcher) == "null" {
		if actual != nil {
			return fmt.Errorf("expected null, got %T: %v", actual, actual)
		}
		return nil
	}

	// Try as an array
	var matcherArr []json.RawMessage
	if err := json.Unmarshal(matcher, &matcherArr); err == nil {
		return matchArrayAssertion(matcherArr, actual)
	}

	// Try as an object (nested assertions)
	var matcherObj map[string]json.RawMessage
	if err := json.Unmarshal(matcher, &matcherObj); err == nil {
		return matchObjectAssertion(matcherObj, actual)
	}

	return fmt.Errorf("unknown matcher format: %s", string(matcher))
}

func matchStringAssertion(matcher string, actual any) error {
	switch matcher {
	case "any":
		// Field must exist (any value is fine)
		return nil

	case "absent":
		if actual != nil {
			return fmt.Errorf("expected field to be absent, but got %T: %v", actual, actual)
		}
		return nil

	case "string:nonempty":
		s, ok := actual.(string)
		if !ok {
			return fmt.Errorf("expected non-empty string, got %T: %v", actual, actual)
		}
		if s == "" {
			return fmt.Errorf("expected non-empty string, got empty string")
		}
		return nil

	case "string:uuid":
		s, ok := actual.(string)
		if !ok {
			return fmt.Errorf("expected UUID string, got %T: %v", actual, actual)
		}
		if !uuidPattern.MatchString(s) {
			return fmt.Errorf("expected valid UUID, got %q", s)
		}
		return nil

	case "string:uuidv7":
		s, ok := actual.(string)
		if !ok {
			return fmt.Errorf("expected UUIDv7 string, got %T: %v", actual, actual)
		}
		if !uuidV7Pattern.MatchString(s) {
			return fmt.Errorf("expected valid UUIDv7, got %q", s)
		}
		return nil

	case "string:datetime":
		s, ok := actual.(string)
		if !ok {
			return fmt.Errorf("expected datetime string, got %T: %v", actual, actual)
		}
		if !datetimePattern.MatchString(s) {
			return fmt.Errorf("expected RFC 3339 datetime, got %q", s)
		}
		return nil

	case "number:positive":
		n, ok := toFloat64(actual)
		if !ok {
			return fmt.Errorf("expected positive number, got %T: %v", actual, actual)
		}
		if n <= 0 {
			return fmt.Errorf("expected positive number, got %v", n)
		}
		return nil

	case "number:non_negative":
		n, ok := toFloat64(actual)
		if !ok {
			return fmt.Errorf("expected non-negative number, got %T: %v", actual, actual)
		}
		if n < 0 {
			return fmt.Errorf("expected non-negative number, got %v", n)
		}
		return nil

	case "array:nonempty":
		arr, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("expected non-empty array, got %T: %v", actual, actual)
		}
		if len(arr) == 0 {
			return fmt.Errorf("expected non-empty array, got empty array")
		}
		return nil

	case "array:empty":
		arr, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("expected empty array, got %T: %v", actual, actual)
		}
		if len(arr) != 0 {
			return fmt.Errorf("expected empty array, got array with %d elements", len(arr))
		}
		return nil
	}

	// Check for number:range(a,b)
	if matches := rangePattern.FindStringSubmatch(matcher); matches != nil {
		lo, _ := strconv.ParseFloat(matches[1], 64)
		hi, _ := strconv.ParseFloat(matches[2], 64)
		n, ok := toFloat64(actual)
		if !ok {
			return fmt.Errorf("expected number in range [%v, %v], got %T: %v", lo, hi, actual, actual)
		}
		if n < lo || n > hi {
			return fmt.Errorf("expected number in range [%v, %v], got %v", lo, hi, n)
		}
		return nil
	}

	// Check for array:length(n)
	if matches := lengthPattern.FindStringSubmatch(matcher); matches != nil {
		expectedLen, _ := strconv.Atoi(matches[1])
		arr, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("expected array of length %d, got %T: %v", expectedLen, actual, actual)
		}
		if len(arr) != expectedLen {
			return fmt.Errorf("expected array of length %d, got length %d", expectedLen, len(arr))
		}
		return nil
	}

	// Check for approximate match ~value
	if matches := approxPattern.FindStringSubmatch(matcher); matches != nil {
		expected, _ := strconv.ParseFloat(matches[1], 64)
		n, ok := toFloat64(actual)
		if !ok {
			return fmt.Errorf("expected approximate number ~%v, got %T: %v", expected, actual, actual)
		}
		tolerance := expected * float64(DefaultTimingTolerancePct) / 100.0
		if math.Abs(n-expected) > tolerance {
			return fmt.Errorf("expected ~%v (tolerance %v%%), got %v (diff: %v)", expected, DefaultTimingTolerancePct, n, math.Abs(n-expected))
		}
		return nil
	}

	// Check for string:pattern(regex)
	if strings.HasPrefix(matcher, "string:pattern(") && strings.HasSuffix(matcher, ")") {
		pattern := matcher[len("string:pattern(") : len(matcher)-1]
		s, ok := actual.(string)
		if !ok {
			return fmt.Errorf("expected string matching pattern %q, got %T: %v", pattern, actual, actual)
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid regex pattern %q: %v", pattern, err)
		}
		if !re.MatchString(s) {
			return fmt.Errorf("expected string matching pattern %q, got %q", pattern, s)
		}
		return nil
	}

	// Literal string comparison
	s, ok := actual.(string)
	if !ok {
		return fmt.Errorf("expected string %q, got %T: %v", matcher, actual, actual)
	}
	if s != matcher {
		return fmt.Errorf("expected %q, got %q", matcher, s)
	}
	return nil
}

func matchNumberAssertion(expected float64, actual any) error {
	n, ok := toFloat64(actual)
	if !ok {
		return fmt.Errorf("expected number %v, got %T: %v", expected, actual, actual)
	}
	// For integers, compare exactly
	if expected == math.Trunc(expected) && n == math.Trunc(n) {
		if int64(expected) != int64(n) {
			return fmt.Errorf("expected %v, got %v", int64(expected), int64(n))
		}
		return nil
	}
	// For floats, use small epsilon
	if math.Abs(expected-n) > 1e-9 {
		return fmt.Errorf("expected %v, got %v", expected, n)
	}
	return nil
}

func matchArrayAssertion(expected []json.RawMessage, actual any) error {
	arr, ok := actual.([]any)
	if !ok {
		return fmt.Errorf("expected array, got %T: %v", actual, actual)
	}
	if len(arr) != len(expected) {
		return fmt.Errorf("expected array of length %d, got length %d", len(expected), len(arr))
	}
	for i, exp := range expected {
		if err := MatchAssertion(exp, arr[i]); err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
	}
	return nil
}

func matchObjectAssertion(expected map[string]json.RawMessage, actual any) error {
	obj, ok := actual.(map[string]any)
	if !ok {
		return fmt.Errorf("expected object, got %T: %v", actual, actual)
	}
	for key, exp := range expected {
		val, exists := obj[key]
		// Check for "absent" matcher
		var s string
		if json.Unmarshal(exp, &s) == nil && s == "absent" {
			if exists {
				return fmt.Errorf("field %q: expected absent, but field exists with value %v", key, val)
			}
			continue
		}
		if !exists {
			return fmt.Errorf("field %q: expected to exist but is missing", key)
		}
		if err := MatchAssertion(exp, val); err != nil {
			return fmt.Errorf("field %q: %w", key, err)
		}
	}
	return nil
}

// ResolveJSONPath extracts a value from a parsed JSON object using a dot-path.
// Supports JSONPath-like syntax: $.field.nested.array[0].value
func ResolveJSONPath(path string, data any) (any, error) {
	// Strip leading "$."
	if strings.HasPrefix(path, "$.") {
		path = path[2:]
	}

	parts := splitJSONPath(path)
	current := data

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Check for array index: field[0]
		if idx := strings.Index(part, "["); idx >= 0 {
			field := part[:idx]
			indexStr := part[idx+1 : len(part)-1]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil, fmt.Errorf("invalid array index in path %q: %v", part, err)
			}

			if field != "" {
				obj, ok := current.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("expected object at %q, got %T", field, current)
				}
				current = obj[field]
			}

			arr, ok := current.([]any)
			if !ok {
				return nil, fmt.Errorf("expected array at %q, got %T", part, current)
			}
			if index < 0 || index >= len(arr) {
				return nil, fmt.Errorf("array index %d out of bounds (length %d) at %q", index, len(arr), part)
			}
			current = arr[index]
		} else {
			obj, ok := current.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("expected object at %q, got %T", part, current)
			}
			val, exists := obj[part]
			if !exists {
				return nil, nil // field doesn't exist
			}
			current = val
		}
	}

	return current, nil
}

// splitJSONPath splits a dot-separated JSON path, respecting brackets.
func splitJSONPath(path string) []string {
	var parts []string
	var current strings.Builder
	depth := 0

	for _, ch := range path {
		if ch == '[' {
			depth++
			current.WriteRune(ch)
		} else if ch == ']' {
			depth--
			current.WriteRune(ch)
		} else if ch == '.' && depth == 0 {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteRune(ch)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
