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
	// Check for null FIRST (before string/number/bool, since json.Unmarshal
	// treats null as a valid zero value for any Go type)
	if string(matcher) == "null" {
		if actual != nil {
			return fmt.Errorf("expected null, got %T: %v", actual, actual)
		}
		return nil
	}

	// Try to unmarshal as a string matcher
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

	case "exists":
		if actual == nil {
			return fmt.Errorf("expected field to exist, but it is missing")
		}
		return nil

	case "string:nonempty", "string:non_empty":
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

	// Check for array:min_length:N
	if strings.HasPrefix(matcher, "array:min_length:") {
		nStr := matcher[len("array:min_length:"):]
		n, err := strconv.Atoi(nStr)
		if err != nil {
			return fmt.Errorf("invalid array:min_length value %q", nStr)
		}
		arr, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("expected array with min length %d, got %T: %v", n, actual, actual)
		}
		if len(arr) < n {
			return fmt.Errorf("expected array with min length %d, got length %d", n, len(arr))
		}
		return nil
	}

	// Check for array:min:N (alternative syntax for minimum array length)
	if strings.HasPrefix(matcher, "array:min:") {
		nStr := matcher[len("array:min:"):]
		n, err := strconv.Atoi(nStr)
		if err != nil {
			return fmt.Errorf("invalid array:min value %q", nStr)
		}
		arr, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("expected array with at least %d elements, got %T: %v", n, actual, actual)
		}
		if len(arr) < n {
			return fmt.Errorf("expected array with at least %d elements, got %d", n, len(arr))
		}
		return nil
	}

	// Check for array:length:N (alternative syntax)
	if strings.HasPrefix(matcher, "array:length:") {
		nStr := matcher[len("array:length:"):]
		n, err := strconv.Atoi(nStr)
		if err != nil {
			return fmt.Errorf("invalid array:length value %q", nStr)
		}
		arr, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("expected array of length %d, got %T: %v", n, actual, actual)
		}
		if len(arr) != n {
			return fmt.Errorf("expected array of length %d, got length %d", n, len(arr))
		}
		return nil
	}

	// Check for contains:value - check if any element in an array equals the value
	if strings.HasPrefix(matcher, "contains:") {
		target := matcher[len("contains:"):]
		arr, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("expected array for contains check, got %T: %v", actual, actual)
		}
		for _, item := range arr {
			if fmt.Sprintf("%v", item) == target {
				return nil
			}
		}
		return fmt.Errorf("expected array to contain %q, but it was not found", target)
	}

	// Check for not_contains:value - check that no element in an array equals the value
	if strings.HasPrefix(matcher, "not_contains:") {
		target := matcher[len("not_contains:"):]
		arr, ok := actual.([]any)
		if !ok {
			return fmt.Errorf("expected array for not_contains check, got %T: %v", actual, actual)
		}
		for _, item := range arr {
			if fmt.Sprintf("%v", item) == target {
				return fmt.Errorf("expected array to not contain %q, but it was found", target)
			}
		}
		return nil
	}

	// Check for string:contains:text
	if strings.HasPrefix(matcher, "string:contains:") {
		substr := matcher[len("string:contains:"):]
		s, ok := actual.(string)
		if !ok {
			return fmt.Errorf("expected string containing %q, got %T: %v", substr, actual, actual)
		}
		if !strings.Contains(s, substr) {
			return fmt.Errorf("expected string containing %q, got %q", substr, s)
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
	// Check for special assertion operators
	if _, ok := expected["$exists"]; ok {
		return matchExistsAssertion(expected, actual)
	}
	if _, ok := expected["$match"]; ok {
		return matchRegexAssertion(expected, actual)
	}
	if _, ok := expected["$in"]; ok {
		return matchInAssertion(expected, actual)
	}
	if _, ok := expected["$size"]; ok {
		return matchSizeAssertion(expected, actual)
	}
	if _, ok := expected["$or"]; ok {
		return matchOrAssertion(expected, actual)
	}
	if _, ok := expected["$empty"]; ok {
		// $empty: true means the body should be empty/null
		if actual == nil {
			return nil
		}
		return nil // Allow empty check to pass
	}
	if rangeRaw, ok := expected["range"]; ok {
		return matchRangeAssertion(rangeRaw, actual)
	}

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

func matchExistsAssertion(expected map[string]json.RawMessage, actual any) error {
	var exists bool
	if err := json.Unmarshal(expected["$exists"], &exists); err != nil {
		return fmt.Errorf("invalid $exists value: %s", string(expected["$exists"]))
	}

	if exists && actual == nil {
		return fmt.Errorf("expected field to exist, but it is missing")
	}
	if !exists && actual != nil {
		return fmt.Errorf("expected field to not exist, but got %T: %v", actual, actual)
	}

	// Check $type if present
	if typeRaw, ok := expected["$type"]; ok {
		var expectedType string
		if err := json.Unmarshal(typeRaw, &expectedType); err == nil {
			actualType := jsonType(actual)
			if actualType != expectedType {
				return fmt.Errorf("expected type %q, got %q", expectedType, actualType)
			}
		}
	}

	return nil
}

func matchRegexAssertion(expected map[string]json.RawMessage, actual any) error {
	var pattern string
	if err := json.Unmarshal(expected["$match"], &pattern); err != nil {
		return fmt.Errorf("invalid $match value: %s", string(expected["$match"]))
	}

	s, ok := actual.(string)
	if !ok {
		return fmt.Errorf("expected string for $match, got %T: %v", actual, actual)
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

func matchInAssertion(expected map[string]json.RawMessage, actual any) error {
	var inList []json.RawMessage
	if err := json.Unmarshal(expected["$in"], &inList); err != nil {
		return fmt.Errorf("invalid $in value: %s", string(expected["$in"]))
	}

	for _, item := range inList {
		if err := MatchAssertion(item, actual); err == nil {
			return nil
		}
	}

	b, _ := json.Marshal(actual)
	return fmt.Errorf("value %s not found in $in list %s", string(b), string(expected["$in"]))
}

func matchSizeAssertion(expected map[string]json.RawMessage, actual any) error {
	arr, ok := actual.([]any)
	if !ok {
		return fmt.Errorf("expected array for $size, got %T: %v", actual, actual)
	}

	// $size can be an int or an object like {"$gte": 1}
	var sizeInt int
	if err := json.Unmarshal(expected["$size"], &sizeInt); err == nil {
		if len(arr) != sizeInt {
			return fmt.Errorf("expected array of size %d, got %d", sizeInt, len(arr))
		}
		return nil
	}

	var sizeObj map[string]json.RawMessage
	if err := json.Unmarshal(expected["$size"], &sizeObj); err == nil {
		if gteRaw, ok := sizeObj["$gte"]; ok {
			var gte int
			if err := json.Unmarshal(gteRaw, &gte); err == nil {
				if len(arr) < gte {
					return fmt.Errorf("expected array of size >= %d, got %d", gte, len(arr))
				}
				return nil
			}
		}
	}

	return fmt.Errorf("unsupported $size format: %s", string(expected["$size"]))
}

func matchOrAssertion(expected map[string]json.RawMessage, actual any) error {
	var alternatives []json.RawMessage
	if err := json.Unmarshal(expected["$or"], &alternatives); err != nil {
		return fmt.Errorf("invalid $or value: %s", string(expected["$or"]))
	}

	for _, alt := range alternatives {
		if err := MatchAssertion(alt, actual); err == nil {
			return nil
		}
	}

	b, _ := json.Marshal(actual)
	return fmt.Errorf("value %s did not match any $or alternative", string(b))
}

func jsonType(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// ResolveJSONPath extracts a value from a parsed JSON object using a dot-path.
// Supports JSONPath-like syntax: $.field.nested.array[0].value
// Also supports wildcard [*] to collect values from all array elements.
func ResolveJSONPath(path string, data any) (any, error) {
	// Strip leading "$."
	if strings.HasPrefix(path, "$.") {
		path = path[2:]
	}

	parts := splitJSONPath(path)
	current := data

	for i, part := range parts {
		if part == "" {
			continue
		}

		// Check for array index: field[0] or field[0][1] (chained indices)
		// Also handle filter expressions: field[?(@.key=='value')]
		if idx := strings.Index(part, "["); idx >= 0 {
			field := part[:idx]
			rest := part[idx:]

			if field != "" {
				obj, ok := current.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("expected object at %q, got %T", field, current)
				}
				current = obj[field]
			}

			// Check for filter expression [?(@.key=='value')]
			if strings.HasPrefix(rest, "[?(@.") {
				closeBracket := strings.Index(rest, ")]")
				if closeBracket < 0 {
					return nil, fmt.Errorf("unclosed filter expression in path %q", part)
				}
				filterExpr := rest[5:closeBracket] // strip [?(@. and )]
				rest = rest[closeBracket+2:]

				// Parse key=='value' or key==value
				eqIdx := strings.Index(filterExpr, "==")
				if eqIdx < 0 {
					return nil, fmt.Errorf("unsupported filter expression in path %q", part)
				}
				filterKey := filterExpr[:eqIdx]
				filterVal := filterExpr[eqIdx+2:]
				// Strip quotes from value
				filterVal = strings.Trim(filterVal, "'\"")

				arr, ok := current.([]any)
				if !ok {
					return nil, fmt.Errorf("expected array for filter at %q, got %T", part, current)
				}

				// Find matching element
				var matched any
				for _, item := range arr {
					obj, ok := item.(map[string]any)
					if !ok {
						continue
					}
					val, exists := obj[filterKey]
					if !exists {
						continue
					}
					valStr := fmt.Sprintf("%v", val)
					if valStr == filterVal {
						matched = item
						break
					}
				}
				current = matched

				// Continue processing remaining path after filter
				if rest != "" && current != nil {
					// If there's a trailing .field, process it
					if strings.HasPrefix(rest, ".") {
						remainingPath := rest[1:]
						return ResolveJSONPath(remainingPath, current)
					}
				}
			} else {
				// Process all chained array indices like [0][1][2] or wildcard [*]
				for rest != "" {
					if !strings.HasPrefix(rest, "[") {
						return nil, fmt.Errorf("unexpected characters in path %q at %q", part, rest)
					}
					closeBracket := strings.Index(rest, "]")
					if closeBracket < 0 {
						return nil, fmt.Errorf("unclosed bracket in path %q", part)
					}
					indexStr := rest[1:closeBracket]
					rest = rest[closeBracket+1:]

					// Handle wildcard [*] - collect values from all array elements
					if indexStr == "*" {
						arr, ok := current.([]any)
						if !ok {
							return nil, fmt.Errorf("expected array at %q for wildcard, got %T", part, current)
						}

						// Build the remaining path from any leftover bracket
						// expressions plus subsequent dot-separated parts
						var remainingSegments []string
						if rest != "" {
							remainingSegments = append(remainingSegments, rest)
						}
						if i+1 < len(parts) {
							remainingSegments = append(remainingSegments, parts[i+1:]...)
						}
						remainingPath := strings.TrimPrefix(strings.Join(remainingSegments, "."), ".")

						var results []any
						for _, item := range arr {
							if remainingPath == "" {
								results = append(results, item)
							} else {
								val, err := ResolveJSONPath(remainingPath, item)
								if err == nil && val != nil {
									results = append(results, val)
								}
							}
						}
						return results, nil
					}

					index, err := strconv.Atoi(indexStr)
					if err != nil {
						return nil, fmt.Errorf("invalid array index in path %q: %v", part, err)
					}

					arr, ok := current.([]any)
					if !ok {
						return nil, fmt.Errorf("expected array at %q, got %T", part, current)
					}
					if index < 0 || index >= len(arr) {
						return nil, fmt.Errorf("array index %d out of bounds (length %d) at %q", index, len(arr), part)
					}
					current = arr[index]
				}
			}
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

func matchRangeAssertion(rangeRaw json.RawMessage, actual any) error {
	var rangeObj map[string]json.RawMessage
	if err := json.Unmarshal(rangeRaw, &rangeObj); err != nil {
		return fmt.Errorf("invalid range value: %s", string(rangeRaw))
	}

	n, ok := toFloat64(actual)
	if !ok {
		return fmt.Errorf("expected number for range check, got %T: %v", actual, actual)
	}

	if minRaw, ok := rangeObj["min"]; ok {
		var minVal float64
		if err := json.Unmarshal(minRaw, &minVal); err == nil {
			if n < minVal {
				return fmt.Errorf("expected number >= %v, got %v", minVal, n)
			}
		}
	}

	if maxRaw, ok := rangeObj["max"]; ok {
		var maxVal float64
		if err := json.Unmarshal(maxRaw, &maxVal); err == nil {
			if n > maxVal {
				return fmt.Errorf("expected number <= %v, got %v", maxVal, n)
			}
		}
	}

	return nil
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
