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

### Assertion Matchers, Template References, and More

For the complete reference on assertion matchers, object operators, JSONPath syntax, template references, timing assertions, and the full test DSL, see the **[Test Case Reference](../docs/test-case-reference.md)**.
