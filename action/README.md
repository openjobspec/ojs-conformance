# OJS Conformance GitHub Action

Run [Open Job Spec](https://openjobspec.org) conformance tests against any OJS-compliant server directly in your CI pipeline.

## Quick Start

```yaml
- name: Run OJS Conformance Tests
  uses: openjobspec/ojs-conformance/action@main
  with:
    server-url: http://localhost:8080
```

## Usage with a Redis backend

```yaml
jobs:
  conformance:
    runs-on: ubuntu-latest
    services:
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
    steps:
      - name: Start OJS server
        run: |
          docker run -d --network host \
            -e REDIS_URL=redis://localhost:6379 \
            -e OJS_ALLOW_INSECURE_NO_AUTH=true \
            ghcr.io/openjobspec/ojs-backend-redis:0.1.0

      - name: Wait for server
        run: |
          for i in $(seq 1 30); do
            curl -sf http://localhost:8080/ojs/v1/health && break
            sleep 1
          done

      - name: Run conformance tests
        uses: openjobspec/ojs-conformance/action@main
        with:
          server-url: http://localhost:8080
          redis-url: redis://localhost:6379
```

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `server-url` | ✅ | — | Base URL of the OJS server |
| `level` | ❌ | `all` | Conformance level: `0`–`4` or `all` |
| `category` | ❌ | — | Filter by category (e.g., `envelope`, `retry`) |
| `test-id` | ❌ | — | Run single test (e.g., `L0-ENV-001`) |
| `output` | ❌ | `table` | Output format: `table` or `json` |
| `redis-url` | ❌ | — | Redis URL for FLUSHDB between tests |
| `tolerance` | ❌ | `50` | Timing tolerance percentage |
| `timeout` | ❌ | `30` | HTTP request timeout (seconds) |
| `verbose` | ❌ | `false` | Show step-by-step details |

## Outputs

| Output | Description |
|--------|-------------|
| `passed` | Number of tests passed |
| `failed` | Number of tests failed |
| `total` | Total tests run |
| `result` | `PASS` or `FAIL` |
| `json-report` | Full JSON results |

## Conformance Levels

| Level | Focus | Tests |
|-------|-------|-------|
| 0 | Core (envelope, lifecycle, operations) | ~50 |
| 1 | Reliable (retries, timeouts, dead-letter) | ~40 |
| 2 | Scheduled (cron, delays, TTL) | ~20 |
| 3 | Workflows (chain, group, batch) | ~15 |
| 4 | Advanced (priority, bulk, unique) | ~20 |

## Using outputs

```yaml
- name: Run tests
  id: conformance
  uses: openjobspec/ojs-conformance/action@main
  with:
    server-url: http://localhost:8080

- name: Check results
  run: |
    echo "Passed: ${{ steps.conformance.outputs.passed }}"
    echo "Failed: ${{ steps.conformance.outputs.failed }}"
```

## Badge

After passing conformance, add a badge to your README:

```markdown
![OJS Conformant](https://img.shields.io/badge/OJS-conformance%20L0--L4-brightgreen)
```
