package lib

import (
	"encoding/json"
	"testing"
)

func raw(s string) json.RawMessage {
	return json.RawMessage(s)
}

// --- MatchAssertion: null ---

func TestMatchAssertion_Null(t *testing.T) {
	if err := MatchAssertion(raw("null"), nil); err != nil {
		t.Fatalf("expected null to match nil, got: %v", err)
	}
}

func TestMatchAssertion_NullMismatch(t *testing.T) {
	if err := MatchAssertion(raw("null"), "hello"); err == nil {
		t.Fatal("expected error when matching null against non-nil")
	}
}

// --- MatchAssertion: string matchers ---

func TestMatchStringAssertion_Any(t *testing.T) {
	for _, val := range []any{"hello", 42.0, true, nil} {
		if err := MatchAssertion(raw(`"any"`), val); err != nil {
			t.Fatalf("any should match %v, got: %v", val, err)
		}
	}
}

func TestMatchStringAssertion_Absent(t *testing.T) {
	if err := MatchAssertion(raw(`"absent"`), nil); err != nil {
		t.Fatalf("absent should match nil, got: %v", err)
	}
	if err := MatchAssertion(raw(`"absent"`), "value"); err == nil {
		t.Fatal("absent should fail when value present")
	}
}

func TestMatchStringAssertion_Exists(t *testing.T) {
	if err := MatchAssertion(raw(`"exists"`), "hello"); err != nil {
		t.Fatalf("exists should match non-nil, got: %v", err)
	}
	if err := MatchAssertion(raw(`"exists"`), nil); err == nil {
		t.Fatal("exists should fail for nil")
	}
}

func TestMatchStringAssertion_NonEmpty(t *testing.T) {
	tests := []struct {
		matcher string
		val     any
		ok      bool
	}{
		{`"string:nonempty"`, "hello", true},
		{`"string:nonempty"`, "", false},
		{`"string:nonempty"`, 42.0, false},
		{`"string:non_empty"`, "hello", true},
		{`"string:non_empty"`, "", false},
	}
	for _, tc := range tests {
		err := MatchAssertion(raw(tc.matcher), tc.val)
		if tc.ok && err != nil {
			t.Errorf("matcher=%s val=%v: unexpected error: %v", tc.matcher, tc.val, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("matcher=%s val=%v: expected error", tc.matcher, tc.val)
		}
	}
}

func TestMatchStringAssertion_UUID(t *testing.T) {
	valid := "019539a4-b68c-7def-8000-1a2b3c4d5e6f"
	invalid := "not-a-uuid"

	if err := MatchAssertion(raw(`"string:uuid"`), valid); err != nil {
		t.Fatalf("should match valid UUID, got: %v", err)
	}
	if err := MatchAssertion(raw(`"string:uuid"`), invalid); err == nil {
		t.Fatal("should reject invalid UUID")
	}
	if err := MatchAssertion(raw(`"string:uuid"`), 42.0); err == nil {
		t.Fatal("should reject non-string")
	}
}

func TestMatchStringAssertion_UUIDv7(t *testing.T) {
	valid := "019539a4-b68c-7def-8000-1a2b3c4d5e6f"
	v4 := "550e8400-e29b-41d4-a716-446655440000"

	if err := MatchAssertion(raw(`"string:uuidv7"`), valid); err != nil {
		t.Fatalf("should match valid UUIDv7, got: %v", err)
	}
	if err := MatchAssertion(raw(`"string:uuidv7"`), v4); err == nil {
		t.Fatal("should reject UUID v4")
	}
}

func TestMatchStringAssertion_Datetime(t *testing.T) {
	tests := []struct {
		val string
		ok  bool
	}{
		{"2024-01-15T10:30:00Z", true},
		{"2024-01-15T10:30:00.123Z", true},
		{"2024-01-15T10:30:00+05:30", true},
		{"2024-01-15T10:30:00.999999-08:00", true},
		{"not-a-date", false},
		{"2024-01-15", false},
	}
	for _, tc := range tests {
		err := MatchAssertion(raw(`"string:datetime"`), tc.val)
		if tc.ok && err != nil {
			t.Errorf("val=%q: unexpected error: %v", tc.val, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("val=%q: expected error", tc.val)
		}
	}
}

func TestMatchStringAssertion_NumberPositive(t *testing.T) {
	if err := MatchAssertion(raw(`"number:positive"`), 5.0); err != nil {
		t.Fatalf("5 should be positive, got: %v", err)
	}
	if err := MatchAssertion(raw(`"number:positive"`), 0.0); err == nil {
		t.Fatal("0 should not be positive")
	}
	if err := MatchAssertion(raw(`"number:positive"`), -1.0); err == nil {
		t.Fatal("-1 should not be positive")
	}
}

func TestMatchStringAssertion_NumberNonNegative(t *testing.T) {
	if err := MatchAssertion(raw(`"number:non_negative"`), 0.0); err != nil {
		t.Fatalf("0 should be non-negative, got: %v", err)
	}
	if err := MatchAssertion(raw(`"number:non_negative"`), 5.0); err != nil {
		t.Fatalf("5 should be non-negative, got: %v", err)
	}
	if err := MatchAssertion(raw(`"number:non_negative"`), -1.0); err == nil {
		t.Fatal("-1 should not be non-negative")
	}
}

func TestMatchStringAssertion_ArrayNonempty(t *testing.T) {
	if err := MatchAssertion(raw(`"array:nonempty"`), []any{1.0, 2.0}); err != nil {
		t.Fatalf("non-empty array should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`"array:nonempty"`), []any{}); err == nil {
		t.Fatal("empty array should fail")
	}
	if err := MatchAssertion(raw(`"array:nonempty"`), "not-array"); err == nil {
		t.Fatal("non-array should fail")
	}
}

func TestMatchStringAssertion_ArrayEmpty(t *testing.T) {
	if err := MatchAssertion(raw(`"array:empty"`), []any{}); err != nil {
		t.Fatalf("empty array should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`"array:empty"`), []any{1.0}); err == nil {
		t.Fatal("non-empty array should fail")
	}
}

func TestMatchStringAssertion_ArrayMinLength(t *testing.T) {
	arr := []any{1.0, 2.0, 3.0}
	if err := MatchAssertion(raw(`"array:min_length:2"`), arr); err != nil {
		t.Fatalf("length 3 >= 2 should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`"array:min_length:5"`), arr); err == nil {
		t.Fatal("length 3 < 5 should fail")
	}
}

func TestMatchStringAssertion_ArrayMin(t *testing.T) {
	arr := []any{1.0, 2.0}
	if err := MatchAssertion(raw(`"array:min:1"`), arr); err != nil {
		t.Fatalf("length 2 >= 1 should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`"array:min:3"`), arr); err == nil {
		t.Fatal("length 2 < 3 should fail")
	}
}

func TestMatchStringAssertion_ArrayLengthColon(t *testing.T) {
	arr := []any{1.0, 2.0}
	if err := MatchAssertion(raw(`"array:length:2"`), arr); err != nil {
		t.Fatalf("length 2 == 2 should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`"array:length:3"`), arr); err == nil {
		t.Fatal("length 2 != 3 should fail")
	}
}

func TestMatchStringAssertion_Contains(t *testing.T) {
	arr := []any{"alpha", "beta", "gamma"}
	if err := MatchAssertion(raw(`"contains:beta"`), arr); err != nil {
		t.Fatalf("should find beta, got: %v", err)
	}
	if err := MatchAssertion(raw(`"contains:delta"`), arr); err == nil {
		t.Fatal("should not find delta")
	}
}

func TestMatchStringAssertion_NotContains(t *testing.T) {
	arr := []any{"alpha", "beta"}
	if err := MatchAssertion(raw(`"not_contains:gamma"`), arr); err != nil {
		t.Fatalf("gamma absent should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`"not_contains:alpha"`), arr); err == nil {
		t.Fatal("alpha present should fail")
	}
}

func TestMatchStringAssertion_StringContains(t *testing.T) {
	if err := MatchAssertion(raw(`"string:contains:world"`), "hello world"); err != nil {
		t.Fatalf("should find substring, got: %v", err)
	}
	if err := MatchAssertion(raw(`"string:contains:xyz"`), "hello world"); err == nil {
		t.Fatal("should not find missing substring")
	}
}

func TestMatchStringAssertion_NumberRange(t *testing.T) {
	if err := MatchAssertion(raw(`"number:range(1, 10)"`), 5.0); err != nil {
		t.Fatalf("5 in [1,10] should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`"number:range(1, 10)"`), 15.0); err == nil {
		t.Fatal("15 not in [1,10] should fail")
	}
	if err := MatchAssertion(raw(`"number:range(1, 10)"`), 0.5); err == nil {
		t.Fatal("0.5 not in [1,10] should fail")
	}
}

func TestMatchStringAssertion_ArrayLengthParen(t *testing.T) {
	if err := MatchAssertion(raw(`"array:length(3)"`), []any{1.0, 2.0, 3.0}); err != nil {
		t.Fatalf("length 3 == 3 should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`"array:length(3)"`), []any{1.0}); err == nil {
		t.Fatal("length 1 != 3 should fail")
	}
}

func TestMatchStringAssertion_Approximate(t *testing.T) {
	// ~100 with 50% tolerance means 50-150
	if err := MatchAssertion(raw(`"~100"`), 120.0); err != nil {
		t.Fatalf("120 within ~100 (±50%%) should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`"~100"`), 200.0); err == nil {
		t.Fatal("200 outside ~100 (±50%) should fail")
	}
}

func TestMatchStringAssertion_StringPattern(t *testing.T) {
	if err := MatchAssertion(raw(`"string:pattern(^[a-z]+$)"`), "hello"); err != nil {
		t.Fatalf("should match pattern, got: %v", err)
	}
	if err := MatchAssertion(raw(`"string:pattern(^[a-z]+$)"`), "Hello123"); err == nil {
		t.Fatal("should not match pattern")
	}
	if err := MatchAssertion(raw(`"string:pattern(^[a-z]+$)"`), 42.0); err == nil {
		t.Fatal("non-string should fail")
	}
}

func TestMatchStringAssertion_LiteralString(t *testing.T) {
	if err := MatchAssertion(raw(`"active"`), "active"); err != nil {
		t.Fatalf("exact match should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`"active"`), "pending"); err == nil {
		t.Fatal("mismatched string should fail")
	}
	if err := MatchAssertion(raw(`"active"`), 42.0); err == nil {
		t.Fatal("non-string should fail")
	}
}

// --- MatchAssertion: number matchers ---

func TestMatchNumberAssertion_Integer(t *testing.T) {
	if err := MatchAssertion(raw("42"), 42.0); err != nil {
		t.Fatalf("42 == 42 should pass, got: %v", err)
	}
	if err := MatchAssertion(raw("42"), 43.0); err == nil {
		t.Fatal("42 != 43 should fail")
	}
}

func TestMatchNumberAssertion_Float(t *testing.T) {
	if err := MatchAssertion(raw("3.14"), 3.14); err != nil {
		t.Fatalf("3.14 == 3.14 should pass, got: %v", err)
	}
}

func TestMatchNumberAssertion_TypeMismatch(t *testing.T) {
	if err := MatchAssertion(raw("42"), "not-a-number"); err == nil {
		t.Fatal("string against number should fail")
	}
}

// --- MatchAssertion: boolean matchers ---

func TestMatchBooleanAssertion(t *testing.T) {
	if err := MatchAssertion(raw("true"), true); err != nil {
		t.Fatalf("true == true should pass, got: %v", err)
	}
	if err := MatchAssertion(raw("true"), false); err == nil {
		t.Fatal("true != false should fail")
	}
	if err := MatchAssertion(raw("false"), false); err != nil {
		t.Fatalf("false == false should pass, got: %v", err)
	}
	if err := MatchAssertion(raw("true"), "not-bool"); err == nil {
		t.Fatal("string against bool should fail")
	}
}

// --- MatchAssertion: array matchers ---

func TestMatchArrayAssertion(t *testing.T) {
	if err := MatchAssertion(raw(`["hello", 42]`), []any{"hello", 42.0}); err != nil {
		t.Fatalf("matching array should pass, got: %v", err)
	}
}

func TestMatchArrayAssertion_LengthMismatch(t *testing.T) {
	if err := MatchAssertion(raw(`["a", "b"]`), []any{"a"}); err == nil {
		t.Fatal("length mismatch should fail")
	}
}

func TestMatchArrayAssertion_ElementMismatch(t *testing.T) {
	if err := MatchAssertion(raw(`["a", "b"]`), []any{"a", "c"}); err == nil {
		t.Fatal("element mismatch should fail")
	}
}

func TestMatchArrayAssertion_TypeMismatch(t *testing.T) {
	if err := MatchAssertion(raw(`["a"]`), "not-array"); err == nil {
		t.Fatal("non-array should fail")
	}
}

// --- MatchAssertion: object matchers ---

func TestMatchObjectAssertion(t *testing.T) {
	matcher := raw(`{"name": "alice", "age": 30}`)
	actual := map[string]any{"name": "alice", "age": 30.0, "extra": true}
	if err := MatchAssertion(matcher, actual); err != nil {
		t.Fatalf("matching object should pass, got: %v", err)
	}
}

func TestMatchObjectAssertion_FieldMismatch(t *testing.T) {
	matcher := raw(`{"name": "alice"}`)
	actual := map[string]any{"name": "bob"}
	if err := MatchAssertion(matcher, actual); err == nil {
		t.Fatal("mismatched field should fail")
	}
}

func TestMatchObjectAssertion_MissingField(t *testing.T) {
	matcher := raw(`{"name": "alice"}`)
	actual := map[string]any{"age": 30.0}
	if err := MatchAssertion(matcher, actual); err == nil {
		t.Fatal("missing field should fail")
	}
}

func TestMatchObjectAssertion_AbsentField(t *testing.T) {
	matcher := raw(`{"deleted_at": "absent"}`)
	actual := map[string]any{"name": "alice"}
	if err := MatchAssertion(matcher, actual); err != nil {
		t.Fatalf("absent field should pass, got: %v", err)
	}

	actual2 := map[string]any{"deleted_at": "2024-01-01"}
	if err := MatchAssertion(matcher, actual2); err == nil {
		t.Fatal("present field should fail absent check")
	}
}

// --- Object assertion operators ---

func TestMatchExistsAssertion(t *testing.T) {
	if err := MatchAssertion(raw(`{"$exists": true}`), "hello"); err != nil {
		t.Fatalf("$exists:true with value should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`{"$exists": true}`), nil); err == nil {
		t.Fatal("$exists:true with nil should fail")
	}
	if err := MatchAssertion(raw(`{"$exists": false}`), nil); err != nil {
		t.Fatalf("$exists:false with nil should pass, got: %v", err)
	}
	if err := MatchAssertion(raw(`{"$exists": false}`), "hello"); err == nil {
		t.Fatal("$exists:false with value should fail")
	}
}

func TestMatchExistsAssertion_WithType(t *testing.T) {
	matcher := raw(`{"$exists": true, "$type": "string"}`)
	if err := MatchAssertion(matcher, "hello"); err != nil {
		t.Fatalf("string type check should pass, got: %v", err)
	}
	if err := MatchAssertion(matcher, 42.0); err == nil {
		t.Fatal("number against string type should fail")
	}
}

func TestMatchRegexAssertion(t *testing.T) {
	matcher := raw(`{"$match": "^email\\."}`)
	if err := MatchAssertion(matcher, "email.send"); err != nil {
		t.Fatalf("regex should match, got: %v", err)
	}
	if err := MatchAssertion(matcher, "sms.send"); err == nil {
		t.Fatal("regex should not match")
	}
	if err := MatchAssertion(matcher, 42.0); err == nil {
		t.Fatal("non-string should fail regex")
	}
}

func TestMatchInAssertion(t *testing.T) {
	matcher := raw(`{"$in": ["active", "completed", "cancelled"]}`)
	if err := MatchAssertion(matcher, "active"); err != nil {
		t.Fatalf("value in list should pass, got: %v", err)
	}
	if err := MatchAssertion(matcher, "pending"); err == nil {
		t.Fatal("value not in list should fail")
	}
}

func TestMatchSizeAssertion_ExactInt(t *testing.T) {
	matcher := raw(`{"$size": 3}`)
	if err := MatchAssertion(matcher, []any{1.0, 2.0, 3.0}); err != nil {
		t.Fatalf("size 3 == 3 should pass, got: %v", err)
	}
	if err := MatchAssertion(matcher, []any{1.0}); err == nil {
		t.Fatal("size 1 != 3 should fail")
	}
}

func TestMatchSizeAssertion_Gte(t *testing.T) {
	matcher := raw(`{"$size": {"$gte": 2}}`)
	if err := MatchAssertion(matcher, []any{1.0, 2.0, 3.0}); err != nil {
		t.Fatalf("size 3 >= 2 should pass, got: %v", err)
	}
	if err := MatchAssertion(matcher, []any{1.0}); err == nil {
		t.Fatal("size 1 < 2 should fail")
	}
}

func TestMatchSizeAssertion_NotArray(t *testing.T) {
	matcher := raw(`{"$size": 1}`)
	if err := MatchAssertion(matcher, "not-array"); err == nil {
		t.Fatal("non-array should fail $size")
	}
}

func TestMatchOrAssertion(t *testing.T) {
	matcher := raw(`{"$or": ["active", "completed"]}`)
	if err := MatchAssertion(matcher, "active"); err != nil {
		t.Fatalf("value matching first alternative should pass, got: %v", err)
	}
	if err := MatchAssertion(matcher, "completed"); err != nil {
		t.Fatalf("value matching second alternative should pass, got: %v", err)
	}
	if err := MatchAssertion(matcher, "pending"); err == nil {
		t.Fatal("value matching no alternative should fail")
	}
}

func TestMatchRangeAssertion(t *testing.T) {
	matcher := raw(`{"range": {"min": 1, "max": 10}}`)
	if err := MatchAssertion(matcher, 5.0); err != nil {
		t.Fatalf("5 in [1,10] should pass, got: %v", err)
	}
	if err := MatchAssertion(matcher, 0.5); err == nil {
		t.Fatal("0.5 < 1 should fail")
	}
	if err := MatchAssertion(matcher, 11.0); err == nil {
		t.Fatal("11 > 10 should fail")
	}
}

func TestMatchRangeAssertion_MinOnly(t *testing.T) {
	matcher := raw(`{"range": {"min": 5}}`)
	if err := MatchAssertion(matcher, 10.0); err != nil {
		t.Fatalf("10 >= 5 should pass, got: %v", err)
	}
	if err := MatchAssertion(matcher, 3.0); err == nil {
		t.Fatal("3 < 5 should fail")
	}
}

// --- ResolveJSONPath ---

func parseJSON(s string) any {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(err)
	}
	return v
}

func TestResolveJSONPath_SimpleField(t *testing.T) {
	data := parseJSON(`{"name": "alice", "age": 30}`)
	val, err := ResolveJSONPath("$.name", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "alice" {
		t.Fatalf("expected alice, got: %v", val)
	}
}

func TestResolveJSONPath_NestedField(t *testing.T) {
	data := parseJSON(`{"job": {"type": "email.send", "queue": "default"}}`)
	val, err := ResolveJSONPath("$.job.type", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "email.send" {
		t.Fatalf("expected email.send, got: %v", val)
	}
}

func TestResolveJSONPath_ArrayIndex(t *testing.T) {
	data := parseJSON(`{"items": ["a", "b", "c"]}`)
	val, err := ResolveJSONPath("$.items[1]", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "b" {
		t.Fatalf("expected b, got: %v", val)
	}
}

func TestResolveJSONPath_ArrayIndexOutOfBounds(t *testing.T) {
	data := parseJSON(`{"items": ["a"]}`)
	_, err := ResolveJSONPath("$.items[5]", data)
	if err == nil {
		t.Fatal("expected out-of-bounds error")
	}
}

func TestResolveJSONPath_NestedArrayObject(t *testing.T) {
	data := parseJSON(`{"jobs": [{"id": "1", "type": "a"}, {"id": "2", "type": "b"}]}`)
	val, err := ResolveJSONPath("$.jobs[0].type", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "a" {
		t.Fatalf("expected a, got: %v", val)
	}
}

func TestResolveJSONPath_Wildcard(t *testing.T) {
	data := parseJSON(`{"jobs": [{"type": "a"}, {"type": "b"}, {"type": "c"}]}`)
	val, err := ResolveJSONPath("$.jobs[*].type", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	arr, ok := val.([]any)
	if !ok {
		t.Fatalf("expected array, got: %T", val)
	}
	if len(arr) != 3 {
		t.Fatalf("expected 3 elements, got: %d", len(arr))
	}
	if arr[0] != "a" || arr[1] != "b" || arr[2] != "c" {
		t.Fatalf("unexpected values: %v", arr)
	}
}

func TestResolveJSONPath_FilterExpression(t *testing.T) {
	data := parseJSON(`{"jobs": [{"id": "1", "state": "active"}, {"id": "2", "state": "completed"}]}`)
	val, err := ResolveJSONPath("$.jobs[?(@.state=='completed')].id", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "2" {
		t.Fatalf("expected 2, got: %v", val)
	}
}

func TestResolveJSONPath_MissingField(t *testing.T) {
	data := parseJSON(`{"name": "alice"}`)
	val, err := ResolveJSONPath("$.missing", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != nil {
		t.Fatalf("expected nil for missing field, got: %v", val)
	}
}

func TestResolveJSONPath_NoDollarPrefix(t *testing.T) {
	data := parseJSON(`{"name": "alice"}`)
	val, err := ResolveJSONPath("name", data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "alice" {
		t.Fatalf("expected alice, got: %v", val)
	}
}

// --- splitJSONPath ---

func TestSplitJSONPath(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a.b.c", []string{"a", "b", "c"}},
		{"items[0].name", []string{"items[0]", "name"}},
		{"a[0][1].b", []string{"a[0][1]", "b"}},
		{"jobs[?(@.state=='active')].id", []string{"jobs[?(@.state=='active')]", "id"}},
	}
	for _, tc := range tests {
		got := splitJSONPath(tc.input)
		if len(got) != len(tc.expected) {
			t.Errorf("splitJSONPath(%q): got %v, want %v", tc.input, got, tc.expected)
			continue
		}
		for i := range got {
			if got[i] != tc.expected[i] {
				t.Errorf("splitJSONPath(%q)[%d]: got %q, want %q", tc.input, i, got[i], tc.expected[i])
			}
		}
	}
}

// --- toFloat64 ---

func TestToFloat64(t *testing.T) {
	tests := []struct {
		input    any
		expected float64
		ok       bool
	}{
		{42.0, 42.0, true},
		{float32(3.14), 3.140000104904175, true},
		{int(10), 10.0, true},
		{int64(100), 100.0, true},
		{int32(50), 50.0, true},
		{"not-a-number", 0, false},
		{true, 0, false},
		{nil, 0, false},
	}
	for _, tc := range tests {
		got, ok := toFloat64(tc.input)
		if ok != tc.ok {
			t.Errorf("toFloat64(%v): ok=%v, want ok=%v", tc.input, ok, tc.ok)
			continue
		}
		if ok && got != tc.expected {
			t.Errorf("toFloat64(%v): got %v, want %v", tc.input, got, tc.expected)
		}
	}
}

// --- jsonType ---

func TestJsonType(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{"hello", "string"},
		{42.0, "number"},
		{true, "boolean"},
		{nil, "null"},
		{[]any{}, "array"},
		{map[string]any{}, "object"},
	}
	for _, tc := range tests {
		got := jsonType(tc.input)
		if got != tc.expected {
			t.Errorf("jsonType(%v): got %q, want %q", tc.input, got, tc.expected)
		}
	}
}
