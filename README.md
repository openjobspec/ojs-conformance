# OJS Conformance Test Suite

Language-agnostic conformance tests for [Open Job Spec (OJS)](https://github.com/openjobspec/spec) implementations.

## Overview

This test suite validates that job queue implementations conform to the OJS specification. Tests are defined as JSON files describing HTTP interactions and expected outcomes, making them runnable against any OJS-conformant server regardless of implementation language.

## Structure

```
ojs-conformance/
  suites/                          # Test case JSON files
    level-0-core/                  # Level 0: Core conformance
      envelope/                    # Job envelope validation
      events/                      # Lifecycle event emission
      lifecycle/                   # State machine transitions
      operations/                  # PUSH/FETCH/ACK/FAIL/CANCEL/INFO/manifest/health
    level-1-reliable/              # Level 1: Reliable delivery
      retry/                       # Retry policies, backoff & error history
      dead-letter/                 # Dead letter queue
      timeout/                     # Execution timeout handling
      visibility/                  # Visibility timeout
      worker/                      # Worker heartbeat & signals
    level-2-scheduled/             # Level 2: Scheduled execution
      delay/                       # Delayed jobs (scheduled_at)
      cron/                        # Cron scheduling
      ttl/                         # Job expiration (expires_at)
    level-3-workflows/             # Level 3: Orchestration
      chain/                       # Sequential execution
      group/                       # Parallel execution
      batch/                       # Parallel with callbacks
    level-4-advanced/              # Level 4: Advanced features
      priority/                    # Priority queues
      unique/                      # Job deduplication
      rate-limit/                  # Rate limiting
      bulk/                        # Batch enqueue
      queue-ops/                   # Queue pause/resume/stats
    ext-admin-api/                 # Extension: Admin API
    ext-backpressure/              # Extension: Backpressure
    ext-dead-letter/               # Extension: Dead letter (extended)
    ext-rate-limiting/             # Extension: Rate limiting (extended)
  runner/                          # Test runner implementations
    http/                          # Go-based HTTP test runner
  lib/                             # Shared library code
```

## Conformance Levels

| Level | Name | Description |
|-------|------|-------------|
| 0 | Core | Job envelope, 8-state lifecycle, PUSH/FETCH/ACK/FAIL, error catalog, manifest, health |
| 1 | Reliable | Retry with backoff, dead letter queue, heartbeat, visibility timeout, execution timeout, error history |
| 2 | Scheduled | Delayed jobs, cron scheduling, job expiration |
| 3 | Orchestration | Chain, group, batch workflows |
| 4 | Advanced | Priority queues, unique jobs, batch enqueue, queue ops |

Each level builds upon the previous. A Level 2 implementation must also pass all Level 0 and Level 1 tests.

## Quick Start

### 1. Start your OJS server

Ensure your implementation is running and accessible via HTTP.

### 2. Build the test runner

```bash
cd runner/http
go build -o ojs-conformance-runner .
```

### 3. Run tests

```bash
# Run all tests
./ojs-conformance-runner -url http://localhost:8080 -suites ../../suites

# Run only Level 0 tests
./ojs-conformance-runner -url http://localhost:8080 -suites ../../suites -level 0

# Run a specific category
./ojs-conformance-runner -url http://localhost:8080 -suites ../../suites -category envelope

# Output as JSON
./ojs-conformance-runner -url http://localhost:8080 -suites ../../suites -output json
```

## Test Server Requirements

Your OJS implementation must register these standard test handlers:

| Handler | Behavior |
|---------|----------|
| `test.echo` | Returns arguments as result |
| `test.fail_once` | Fails first attempt, succeeds second |
| `test.fail_twice` | Fails first two attempts, succeeds third |
| `test.fail_always` | Always fails with retryable error |
| `test.slow` | Sleeps for configurable duration |
| `test.timeout` | Exceeds any configured timeout |
| `test.panic` | Crashes the handler |
| `test.produce` | Returns a configurable result value |
| `test.noop` | Succeeds immediately |

## Conformance Report

The test runner produces a conformance report:

```json
{
  "test_suite_version": "1.0.0-rc.1",
  "target": "http://localhost:8080",
  "run_at": "2026-02-12T10:30:00Z",
  "duration_ms": 45230,
  "requested_level": -1,
  "results": {
    "total": 85,
    "passed": 83,
    "failed": 2,
    "skipped": 0
  },
  "conformant": false,
  "conformant_level": 1
}
```

## Documentation

- **[Test Case Reference](docs/test-case-reference.md)** — Complete reference for the test DSL: every field, matcher, operator, JSONPath syntax, template references, and timing assertions.

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines on adding tests.

## License

Apache License 2.0 - see [LICENSE](LICENSE).
