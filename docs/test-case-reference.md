# OJS Conformance Test Case Reference

Complete reference for the OJS conformance test DSL — covering every field, matcher, operator, and syntax available in test JSON files.

## Table of Contents

- [Test Case Schema](#test-case-schema)
- [Step Schema](#step-schema)
- [Assertions Object](#assertions-object)
- [Matcher Reference](#matcher-reference)
  - [String Matchers](#string-matchers)
  - [Number Matchers](#number-matchers)
  - [Array Matchers](#array-matchers)
  - [Boolean and Null](#boolean-and-null)
  - [Object Operators](#object-operators)
- [JSONPath Syntax](#jsonpath-syntax)
- [Template References](#template-references)
- [Timing Assertions](#timing-assertions)
- [Intent Reference](#intent-reference)

---

## Test Case Schema

Each test is a self-contained JSON file with these top-level fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `test_id` | string | yes | Unique identifier (e.g. `"L0-ENV-001"`) |
| `level` | int | yes | Conformance level: 0–4 |
| `category` | string | yes | Grouping within a level (e.g. `"envelope"`, `"retry"`) |
| `name` | string | yes | Kebab-case name matching the filename |
| `description` | string | yes | Human-readable explanation of what the test validates |
| `spec_ref` | string | yes | Section reference in the OJS specification |
| `tags` | string[] | yes | Searchable labels (e.g. `["level-0", "envelope", "positive"]`) |
| `setup` | object | no | Steps to run before the test (same shape as `steps`) |
| `steps` | Step[] | yes | Ordered list of steps to execute |
| `teardown` | object | no | Steps to run after the test (same shape as `steps`) |

### Example

```json
{
  "test_id": "L0-ENV-001",
  "level": 0,
  "category": "envelope",
  "name": "valid-minimal-job",
  "description": "Enqueue a job with only required fields",
  "spec_ref": "ojs-core#section-5.1",
  "tags": ["level-0", "envelope", "positive"],
  "steps": [ ... ]
}
```

---

## Step Schema

Each step describes a single HTTP interaction (or a special action like `WAIT`).

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique step identifier within this test |
| `action` | string | yes | HTTP method (`GET`, `POST`, `DELETE`) or special action (`WAIT`, `ASSERT`) |
| `intent` | string | no | Semantic label describing the step's purpose (see [Intent Reference](#intent-reference)) |
| `path` | string | conditional | URL path (required for HTTP actions, omitted for `WAIT`/`ASSERT`). Supports [template references](#template-references). |
| `headers` | object | no | HTTP headers as key-value string pairs |
| `body` | any | no | Request body (JSON object, array, or raw value). Supports [template references](#template-references). |
| `delay_ms` | int | no | Milliseconds to wait **before** executing this step |
| `duration_ms` | int | no | Duration in milliseconds (used with `WAIT` action) |
| `description` | string | no | Human-readable description of what this step does |
| `assertions` | object | no | Expected outcomes (see [Assertions Object](#assertions-object)) |

### WAIT Action

The `WAIT` action pauses execution without making an HTTP request. Use it for timing-sensitive tests.

- If `duration_ms` is set, the runner sleeps for that duration.
- If only `delay_ms` is set, the runner sleeps for that duration instead.
- If both are 0 or absent, the step returns immediately.
- Assertions are **not evaluated** for WAIT steps.

```json
{
  "id": "wait-for-retry",
  "action": "WAIT",
  "intent": "wait",
  "duration_ms": 3000
}
```

### ASSERT Action

The `ASSERT` action evaluates cross-step assertions without making an HTTP request. Use it to compare results from multiple previous steps.

```json
{
  "id": "verify-exclusive",
  "action": "ASSERT",
  "intent": "assert",
  "description": "Verify exactly one fetch claimed the job",
  "assertions": {
    "exclusive_claim": {
      "job_id": "{{steps.step-1.response.body.job.id}}",
      "fetches": ["{{steps.step-2.response.body.jobs}}", "{{steps.step-3.response.body.jobs}}"],
      "exactly_one_has_job": true
    }
  }
}
```

### Delays

Any step (not just WAIT) can include `delay_ms` to pause before execution:

```json
{
  "id": "fetch-after-delay",
  "action": "POST",
  "intent": "fetch",
  "delay_ms": 3000,
  "path": "/ojs/v1/workers/fetch",
  "body": { "queues": ["default"] }
}
```

---

## Assertions Object

The `assertions` object defines expected outcomes for a step.

| Field | Type | Description |
|-------|------|-------------|
| `status` | int, string, or object | Expected HTTP status code (see below) |
| `status_in` | int[] | List of acceptable status codes (passes if actual matches any) |
| `body` | object | Map of JSONPath expressions to matchers |
| `body_absent` | string[] | JSONPath expressions that must **not** resolve to a value |
| `body_raw` | any | Reserved for raw body matching (defined but not yet implemented) |
| `body_contains` | string[] | Substrings that must appear in the raw response body |
| `headers` | object | Map of header names to expected values (case-insensitive names, exact value match) |
| `timing_ms` | object | Response time assertions (see [Timing Assertions](#timing-assertions)) |

### Status Assertion

The `status` field supports three formats:

**Integer** — exact match:
```json
{ "status": 201 }
```

**String matcher** — range or one-of:
```json
{ "status": "number:range(400,422)" }
{ "status": "one_of:200,201,409" }
```

**Object operator** — using `$in`:
```json
{ "status": { "$in": [200, 409] } }
```

### Status In

`status_in` is a simpler alternative for checking multiple valid status codes:

```json
{ "status_in": [200, 201] }
```

### Body Assertions

The `body` field maps JSONPath expressions to matchers. Each entry is evaluated independently.

```json
{
  "body": {
    "$.job.id": "string:uuidv7",
    "$.job.state": "available",
    "$.job.attempt": 0,
    "$.job.queue": { "$in": ["default", "test"] }
  }
}
```

#### Top-level `$or` in Body

The body object can use a top-level `$or` key whose value is an array of alternative body assertion objects. The assertion passes if **any** alternative matches:

```json
{
  "body": {
    "$or": [
      { "$.jobs": "array:empty" },
      { "$.jobs[0].state": "available" }
    ]
  }
}
```

### Body Absent

`body_absent` lists JSONPath expressions that must resolve to `nil` (the field must not exist):

```json
{
  "body_absent": ["$.job.result", "$.job.error"]
}
```

### Body Contains

`body_contains` checks for substrings in the raw response body (case-sensitive):

```json
{
  "body_contains": ["\"state\":\"completed\""]
}
```

### Headers

`headers` maps header names (case-insensitive per HTTP spec) to expected values (exact match):

```json
{
  "headers": {
    "Content-Type": "application/openjobspec+json"
  }
}
```

---

## Matcher Reference

Matchers are the values in body assertion maps. They can be strings, numbers, booleans, null, arrays, or objects.

### String Matchers

| Matcher | Description | Example |
|---------|-------------|---------|
| `"any"` | Field exists with any non-null value | `"$.id": "any"` |
| `"absent"` | Field must not exist (resolves to nil) | `"$.error": "absent"` |
| `"exists"` | Field must exist (may be any value including null) | `"$.meta": "exists"` |
| `"string:nonempty"` | Non-empty string | `"$.name": "string:nonempty"` |
| `"string:non_empty"` | Alias for `string:nonempty` | `"$.name": "string:non_empty"` |
| `"string:uuid"` | Valid UUID (any version) | `"$.id": "string:uuid"` |
| `"string:uuidv7"` | Valid UUIDv7 (version=7, variant bits correct) | `"$.id": "string:uuidv7"` |
| `"string:datetime"` | RFC 3339 timestamp (e.g. `2024-01-15T10:30:00Z`) | `"$.created_at": "string:datetime"` |
| `"string:contains:X"` | String contains substring X (case-sensitive) | `"$.error": "string:contains:not found"` |
| `"string:pattern(regex)"` | String matches Go regex pattern | `"$.type": "string:pattern(^test\\..*)"` |
| `"literal"` | Exact string match | `"$.state": "available"` |

#### UUID Patterns

- **UUID (any):** `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
- **UUIDv7:** `^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`

#### Datetime Pattern

`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`

### Number Matchers

| Matcher | Description | Example |
|---------|-------------|---------|
| `"number:positive"` | Number > 0 | `"$.attempt": "number:positive"` |
| `"number:non_negative"` | Number >= 0 | `"$.attempt": "number:non_negative"` |
| `"number:range(a,b)"` | Number in range [a, b] inclusive | `"$.priority": "number:range(0,100)"` |
| `"~value"` | Approximate match (with tolerance) | `"$.duration_ms": "~3000"` |
| `42` | Exact numeric match | `"$.attempt": 0` |

#### Approximate Matching (`~`)

The `~` prefix enables approximate numeric comparison:
- **Tolerance** = max(expected × tolerance_pct / 100, 100ms)
- **Default tolerance_pct** = 50 (configurable via `-tolerance` flag)
- Passes if `|actual - expected| <= tolerance`

```json
"$.retry_delay_ms": "~2000"
```

With default tolerance (50%), this matches values from 1000 to 3000.

### Array Matchers

| Matcher | Description | Example |
|---------|-------------|---------|
| `"array:nonempty"` | Array with 1+ elements | `"$.jobs": "array:nonempty"` |
| `"array:empty"` | Empty array (`[]`) | `"$.jobs": "array:empty"` |
| `"array:length:N"` | Array with exactly N elements | `"$.jobs": "array:length:1"` |
| `"array:length(N)"` | Alias for `array:length:N` | `"$.jobs": "array:length(1)"` |
| `"array:min_length:N"` | Array with at least N elements | `"$.errors": "array:min_length:2"` |
| `"array:min:N"` | Alias for `array:min_length:N` | `"$.errors": "array:min:2"` |
| `"contains:X"` | Array contains element matching X | `"$.tags": "contains:urgent"` |
| `"not_contains:X"` | Array does not contain element matching X | `"$.tags": "not_contains:deleted"` |
| `[m1, m2, ...]` | Positional element matching (see below) | `"$.items": [1, 2, 3]` |

#### Positional Array Matching

An array literal as matcher requires exact length and applies each sub-matcher to the corresponding element:

```json
"$.results": ["string:nonempty", "string:nonempty", "string:nonempty"]
```

This requires exactly 3 elements and each must be a non-empty string.

#### Element Matching in `contains` / `not_contains`

Elements are converted to strings using Go's `%v` format before comparison. This means:
- Strings match as-is
- Numbers match their string representation (e.g. `42` matches `"42"`)
- Objects/arrays match their Go default format

### Boolean and Null

| Matcher | Description | Example |
|---------|-------------|---------|
| `true` | Exact boolean true | `"$.active": true` |
| `false` | Exact boolean false | `"$.paused": false` |
| `null` | Exact null value | `"$.result": null` |

### Object Operators

When a matcher is a JSON object, it is interpreted as a combination of operators.

#### `$exists`

Checks whether a field exists.

```json
"$.job.id": { "$exists": true }
"$.job.error": { "$exists": false }
```

Can be combined with `$type`:

```json
"$.job.id": { "$exists": true, "$type": "string" }
```

#### `$type`

Checks the JSON type of a value. Used with `$exists`. Valid types:

| Type | Matches |
|------|---------|
| `"string"` | JSON strings |
| `"number"` | JSON numbers (int or float) |
| `"boolean"` | `true` or `false` |
| `"null"` | `null` |
| `"array"` | JSON arrays |
| `"object"` | JSON objects |

#### `$match`

Regex pattern matching (requires a string value):

```json
"$.error.type": { "$match": "^Validation.*" }
```

#### `$in`

Value must match any element in the list. Each alternative is tested recursively, so they can be matchers themselves:

```json
"$.state": { "$in": ["available", "active", "completed"] }
"$.status": { "$in": [200, 201] }
```

#### `$size`

Checks array length. Two formats:

**Exact length:**
```json
"$.jobs": { "$size": 3 }
```

**Minimum length:**
```json
"$.jobs": { "$size": { "$gte": 1 } }
```

#### `$or`

Logical OR — value must match at least one alternative:

```json
"$.state": { "$or": ["available", "active"] }
```

Alternatives are tested recursively, so complex matchers work:

```json
"$.value": { "$or": ["string:nonempty", { "$exists": false }] }
```

#### `$empty`

Checks for empty/null body:

```json
"$": { "$empty": true }
```

#### `range`

Number range check with optional min/max bounds:

```json
"$.priority": { "range": { "min": 0, "max": 100 } }
"$.delay_ms": { "range": { "min": 1000 } }
"$.retries": { "range": { "max": 5 } }
```

---

## JSONPath Syntax

Body assertions use JSONPath expressions to navigate the response JSON. All paths start with `$` representing the root.

### Dot Notation

Access object fields with dots:

```
$.job.id           → root.job.id
$.job.state        → root.job.state
$.job.options.queue → root.job.options.queue
```

### Array Indexing

Access array elements with bracket notation:

```
$.jobs[0]           → first element
$.jobs[0].id        → id of first element
$.jobs[0].args[0]   → first arg of first job
```

Chained indexing is supported:

```
$.matrix[0][1]      → second element of first row
```

### Wildcards

Collect values from all array elements:

```
$.jobs[*].id        → array of all job IDs
$.jobs[*].state     → array of all states
```

Returns a flattened `[]any` array. Elements that fail to resolve are silently skipped.

### Filter Expressions

Select the first array element matching a condition:

```
$.jobs[?(@.state=='active')].id     → ID of first active job
$.items[?(@.type=='test.echo')].args → args of first matching item
```

**Limitations:**
- Only the `==` operator is supported
- Returns the **first** matching element, not all matches
- Values can be quoted strings or unquoted literals

---

## Template References

Steps can reference values from previous step responses using template syntax.

### Syntax

```
{{steps.<STEP_ID>.response.body.<FIELD_PATH>}}
```

| Component | Description |
|-----------|-------------|
| `steps` | Fixed prefix |
| `<STEP_ID>` | The `id` of a previous step |
| `response.body` | Fixed — references the parsed response body |
| `<FIELD_PATH>` | Dot-separated path into the response JSON |

### Usage in Path

```json
{
  "id": "get-job",
  "action": "GET",
  "intent": "get-job",
  "path": "/ojs/v1/jobs/{{steps.enqueue.response.body.job.id}}"
}
```

### Usage in Body

```json
{
  "body": {
    "job_id": "{{steps.enqueue.response.body.job.id}}",
    "worker_id": "worker-1"
  }
}
```

### Usage in Assertions

Template references can appear in both assertion paths and matcher values:

```json
{
  "assertions": {
    "body": {
      "$.job.id": "{{steps.step-1.response.body.job.id}}"
    }
  }
}
```

### Value Conversion

When a template resolves:
- **Strings** are inserted as-is
- **Integers** (whole numbers) are formatted without decimals
- **Floats** are formatted with decimal notation
- **Objects/arrays** are marshaled to JSON strings

### Scoping Rules

- Templates can only reference steps that executed **before** the current step
- If a referenced step doesn't exist or the field path doesn't resolve, the original template string is left unchanged (no error is raised)

---

## Timing Assertions

The `timing_ms` object validates HTTP response time.

| Field | Type | Description |
|-------|------|-------------|
| `less_than` | int | Response must complete in fewer than N milliseconds |
| `greater_than` | int | Response must take more than N milliseconds |
| `approximate` | int | Response time should be approximately N milliseconds |

### Less Than

Fails if `actual_ms >= threshold` (inclusive bound):

```json
{
  "timing_ms": { "less_than": 500 }
}
```

### Greater Than

Fails if `actual_ms <= threshold` (inclusive bound):

```json
{
  "timing_ms": { "greater_than": 1000 }
}
```

### Approximate

Uses configurable tolerance. Default: 50% with a minimum of 100ms.

```json
{
  "timing_ms": { "approximate": 3000 }
}
```

**Formula:** `tolerance = max(expected * tolerance_pct / 100, min_tolerance_ms)`

With defaults (50%, 100ms minimum):
- `approximate: 3000` → passes for 1500–4500ms
- `approximate: 100` → passes for 0–200ms (tolerance = 100ms, min floor)
- `approximate: 50` → passes for 0–150ms (tolerance = 100ms minimum)

The tolerance percentage is configurable via the runner's `-tolerance` flag.

---

## Intent Reference

The `intent` field is an optional, informational label on each step. It makes multi-step tests immediately scannable by describing the semantic purpose of the HTTP call.

| Intent | HTTP Action | Path Pattern | Description |
|--------|------------|--------------|-------------|
| `enqueue` | POST | `/ojs/v1/jobs` | Submit a new job |
| `fetch` | POST | `/ojs/v1/workers/fetch` | Fetch available jobs from queues |
| `ack` | POST | `/ojs/v1/workers/ack` | Acknowledge (complete) a job |
| `nack` | POST | `/ojs/v1/workers/nack` | Negatively acknowledge (fail) a job |
| `cancel` | DELETE | `/ojs/v1/jobs/{id}` | Cancel a job |
| `get-job` | GET | `/ojs/v1/jobs/{id}` | Retrieve job details |
| `heartbeat` | POST | `/ojs/v1/workers/heartbeat` | Extend a job's visibility timeout |
| `bulk-enqueue` | POST | `/ojs/v1/jobs/batch` | Submit multiple jobs at once |
| `register-cron` | POST | `/ojs/v1/cron` | Register a cron schedule |
| `list-crons` | GET | `/ojs/v1/cron` | List all cron schedules |
| `delete-cron` | DELETE | `/ojs/v1/cron/{id}` | Delete a cron schedule |
| `list-dead-letter` | GET | `/ojs/v1/dead-letter` | List dead letter queue entries |
| `delete-dead-letter` | DELETE | `/ojs/v1/dead-letter/{id}` | Delete a dead letter entry |
| `retry-dead-letter` | POST | `/ojs/v1/dead-letter/{id}/retry` | Retry a dead letter job |
| `pause-queue` | POST | `/ojs/v1/queues/{name}/pause` | Pause a queue |
| `resume-queue` | POST | `/ojs/v1/queues/{name}/resume` | Resume a paused queue |
| `queue-stats` | GET | `/ojs/v1/queues/{name}/stats` | Get queue statistics |
| `create-workflow` | POST | `/ojs/v1/workflows` | Create a workflow (chain/group/batch) |
| `get-workflow` | GET | `/ojs/v1/workflows/{id}` | Retrieve workflow status |
| `cancel-workflow` | DELETE | `/ojs/v1/workflows/{id}` | Cancel a workflow |
| `wait` | WAIT | — | Pause execution (no HTTP call) |
| `assert` | ASSERT | — | Cross-step assertion (no HTTP call) |

The runner ignores this field — it is purely for human readability.
