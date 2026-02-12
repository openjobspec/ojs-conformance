# OJS Conformance Test Runner

A Go-based HTTP test runner that validates OJS (Open Job Spec) implementations against the conformance test suite.

## Prerequisites

- Go 1.22+
- An OJS-conformant server running and accessible via HTTP

## Building

```bash
cd runner/http
go build -o ojs-conformance-runner .
```

## Usage

### Basic Usage

Run all tests against a local server:

```bash
./ojs-conformance-runner -url http://localhost:8080 -suites ../../suites
```

### Filtering

Filter by conformance level:

```bash
./ojs-conformance-runner -url http://localhost:8080 -suites ../../suites -level 0
```

Filter by category:

```bash
./ojs-conformance-runner -url http://localhost:8080 -suites ../../suites -category envelope
```

Run a single test:

```bash
./ojs-conformance-runner -url http://localhost:8080 -suites ../../suites -test L0-ENV-001
```

### Output Formats

Human-readable table (default):

```bash
./ojs-conformance-runner -url http://localhost:8080 -suites ../../suites -output table
```

JSON report:

```bash
./ojs-conformance-runner -url http://localhost:8080 -suites ../../suites -output json
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | `http://localhost:8080` | Base URL of the OJS server |
| `-suites` | `./suites` | Path to test suite directory |
| `-level` | `-1` (all) | Filter by conformance level (0-4) |
| `-category` | `""` (all) | Filter by category |
| `-test` | `""` (all) | Run a single test by ID |
| `-output` | `table` | Output format: `table` or `json` |
| `-verbose` | `false` | Show detailed step results |
| `-tolerance` | `50` | Timing tolerance percentage |
| `-timeout` | `30` | HTTP request timeout in seconds |

### Exit Codes

- `0` - All tests passed
- `1` - One or more tests failed
- `2` - Configuration error (no tests found, invalid flags)

## Test Server Requirements

Your OJS implementation must provide these standard test handlers for the conformance tests:

| Handler | Behavior |
|---------|----------|
| `test.echo` | Returns arguments as result |
| `test.fail_once` | Fails on first attempt, succeeds on second |
| `test.fail_twice` | Fails first two attempts, succeeds on third |
| `test.fail_always` | Always fails with a retryable error |
| `test.slow` | Sleeps for a configurable duration |
| `test.timeout` | Runs longer than any configured timeout |
| `test.panic` | Crashes the handler |
| `test.produce` | Returns a configurable result value |
| `test.noop` | Succeeds immediately with no result |

## Test File Format

Each test is a self-contained JSON file:

```json
{
  "test_id": "L0-ENV-001",
  "level": 0,
  "category": "envelope",
  "name": "valid-minimal-job",
  "description": "Enqueue a job with only required fields",
  "spec_ref": "ojs-core#section-5.1",
  "tags": ["level-0", "envelope", "positive"],
  "steps": [
    {
      "id": "enqueue",
      "action": "POST",
      "path": "/ojs/v1/jobs",
      "body": {
        "type": "test.echo",
        "args": ["hello"]
      },
      "assertions": {
        "status": 201,
        "body": {
          "$.id": "string:uuidv7",
          "$.state": "available",
          "$.queue": "default"
        }
      }
    }
  ]
}
```

### Assertion Matchers

| Matcher | Description |
|---------|-------------|
| `"string:nonempty"` | Any non-empty string |
| `"string:uuid"` | Valid UUID (any version) |
| `"string:uuidv7"` | Valid UUIDv7 |
| `"string:datetime"` | RFC 3339 timestamp |
| `"number:positive"` | Positive number |
| `"number:non_negative"` | Non-negative number |
| `"number:range(a,b)"` | Number within [a, b] |
| `"any"` | Field exists with any value |
| `"absent"` | Field must not exist |
| `"array:length(n)"` | Array with exactly n elements |
| `"array:nonempty"` | Array with 1+ elements |
| `"array:empty"` | Empty array |
| `"~value"` | Approximate value (configurable tolerance) |
| `"string:pattern(regex)"` | String matching regex pattern |
| Literal values | Exact match (strings, numbers, booleans, null) |

### Template References

Steps can reference results from previous steps:

```json
{
  "path": "/ojs/v1/jobs/{{steps.enqueue.response.body.id}}"
}
```

### Delays

Steps can include delays for timing-sensitive tests:

```json
{
  "id": "wait-and-fetch",
  "delay_ms": 3000,
  "action": "POST",
  "path": "/ojs/v1/workers/fetch",
  "body": { "queues": ["default"] }
}
```
