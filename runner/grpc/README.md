# OJS gRPC Conformance Runner

Runs the OJS conformance test suite against a gRPC endpoint.

## Prerequisites

- Go 1.22+
- An OJS-conformant server running and accessible via gRPC

## Building

```bash
cd runner/grpc
go build -o ojs-conformance-grpc-runner .
```

## Usage

### Basic Usage

Run all tests against a local gRPC server:

```bash
./ojs-conformance-grpc-runner -url localhost:9090 -suites ../../suites
```

### TLS Connections

Connect to a gRPC server using TLS:

```bash
./ojs-conformance-grpc-runner -url grpc.example.com:443 -suites ../../suites -tls
```

Skip TLS certificate verification (e.g., self-signed certs):

```bash
./ojs-conformance-grpc-runner -url localhost:9090 -suites ../../suites -tls -insecure
```

### Filtering

Filter by conformance level:

```bash
./ojs-conformance-grpc-runner -url localhost:9090 -suites ../../suites -level 0
```

Filter by category:

```bash
./ojs-conformance-grpc-runner -url localhost:9090 -suites ../../suites -category envelope
```

Run a single test:

```bash
./ojs-conformance-grpc-runner -url localhost:9090 -suites ../../suites -test L0-ENV-001
```

### Output Formats

Human-readable table (default):

```bash
./ojs-conformance-grpc-runner -url localhost:9090 -suites ../../suites -output table
```

JSON report:

```bash
./ojs-conformance-grpc-runner -url localhost:9090 -suites ../../suites -output json
```

### Test Isolation with Redis

Flush Redis between tests for full isolation:

```bash
./ojs-conformance-grpc-runner -url localhost:9090 -suites ../../suites -redis redis://localhost:6379
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | `localhost:9090` | gRPC server address (host:port) |
| `-suites` | `./suites` | Path to test suites directory |
| `-level` | `-1` (all) | Conformance level to test (0-4) |
| `-category` | `""` (all) | Filter by category |
| `-test` | `""` (all) | Run a single test by ID |
| `-output` | `table` | Output format: `table` or `json` |
| `-verbose` | `false` | Show detailed step results |
| `-tolerance` | `50` | Timing tolerance percentage |
| `-timeout` | `30` | Per-RPC timeout in seconds |
| `-tls` | `false` | Use TLS for gRPC connection |
| `-insecure` | `false` | Skip TLS certificate verification (use with `-tls`) |
| `-redis` | `""` | Redis URL for FLUSHDB between tests |

### Exit Codes

- `0` - All tests passed (conformant)
- `1` - One or more tests failed
- `2` - Configuration error (connection failure, no tests found)

## Architecture

### Adapter Layer (`adapter.go`)

The adapter translates between the HTTP-oriented test definitions and gRPC:

- **Route resolution**: Maps HTTP verb + path pairs to gRPC method names using
  an ordered route table (exact matches before prefix matches).
- **Status code translation**: Maps gRPC status codes to HTTP equivalents so
  that test assertions (e.g., `"status": 201`) work unchanged.
- **HTTP status overrides**: RPCs that correspond to HTTP 201 Created (Enqueue,
  EnqueueBatch, RegisterCron, CreateWorkflow) override the default mapping.

### Route Mapping

| HTTP Action + Path | gRPC RPC |
|---|---|
| `POST /ojs/v1/jobs` | `OJSService/Enqueue` |
| `POST /ojs/v1/jobs/batch` | `OJSService/EnqueueBatch` |
| `GET /ojs/v1/jobs/:id` | `OJSService/GetJob` |
| `DELETE /ojs/v1/jobs/:id` | `OJSService/CancelJob` |
| `POST /ojs/v1/workers/fetch` | `OJSService/Fetch` |
| `POST /ojs/v1/workers/ack` | `OJSService/Ack` |
| `POST /ojs/v1/workers/nack` | `OJSService/Nack` |
| `POST /ojs/v1/workers/heartbeat` | `OJSService/Heartbeat` |
| `GET /ojs/v1/queues` | `OJSService/ListQueues` |
| `GET /ojs/v1/queues/:name/stats` | `OJSService/QueueStats` |
| `POST /ojs/v1/queues/:name/pause` | `OJSService/PauseQueue` |
| `POST /ojs/v1/queues/:name/resume` | `OJSService/ResumeQueue` |
| `GET /ojs/v1/dead-letter` | `OJSService/ListDeadLetter` |
| `POST /ojs/v1/dead-letter/:id/retry` | `OJSService/RetryDeadLetter` |
| `DELETE /ojs/v1/dead-letter/:id` | `OJSService/DeleteDeadLetter` |
| `POST /ojs/v1/cron` | `OJSService/RegisterCron` |
| `DELETE /ojs/v1/cron/:name` | `OJSService/UnregisterCron` |
| `GET /ojs/v1/cron` | `OJSService/ListCron` |
| `POST /ojs/v1/workflows` | `OJSService/CreateWorkflow` |
| `GET /ojs/v1/workflows/:id` | `OJSService/GetWorkflow` |
| `DELETE /ojs/v1/workflows/:id` | `OJSService/CancelWorkflow` |
| `GET /ojs/manifest` | `OJSService/Manifest` |
| `GET /ojs/v1/health` | `OJSService/Health` |

### gRPC ↔ HTTP Status Code Mapping

| gRPC Code | HTTP Status |
|---|---|
| `OK` | `200` |
| `InvalidArgument` | `400` |
| `Unauthenticated` | `401` |
| `PermissionDenied` | `403` |
| `NotFound` | `404` |
| `AlreadyExists` | `409` |
| `FailedPrecondition` | `412` |
| `ResourceExhausted` | `429` |
| `Internal` | `500` |
| `Unimplemented` | `501` |
| `Unavailable` | `503` |
| `DeadlineExceeded` | `504` |

### File Structure

| File | Purpose |
|---|---|
| `main.go` | CLI entry point, flag parsing, test orchestration |
| `adapter.go` | HTTP-to-gRPC route resolution and status code translation |
| `client.go` | gRPC client wrapper, RPC dispatch, proto ↔ JSON conversion |
| `runner.go` | Test loading, filtering, execution, assertion evaluation, reporting |

The same JSON test definitions work for both the HTTP and gRPC runners.
