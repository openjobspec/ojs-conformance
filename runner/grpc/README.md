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
./ojs-conformance-grpc-runner -url localhost:9090 -suites ../../suites -format table
```

JSON report:

```bash
./ojs-conformance-grpc-runner -url localhost:9090 -suites ../../suites -format json
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-url` | `localhost:9090` | gRPC server address (host:port) |
| `-suites` | `./suites` | Path to test suites directory |
| `-level` | `-1` (all) | Conformance level to test (0-4) |
| `-category` | `""` (all) | Filter by category |
| `-test` | `""` (all) | Run a single test by ID |
| `-format` | `table` | Output format: `table` or `json` |
| `-verbose` | `false` | Show detailed step results |
| `-tolerance` | `50` | Timing tolerance percentage |
| `-timeout` | `30` | Per-RPC timeout in seconds |

### Exit Codes

- `0` - All tests passed (conformant)
- `1` - One or more tests failed
- `2` - Configuration error (connection failure, no tests found)

## How It Works

The runner reads the same JSON test definitions used by the HTTP runner and maps
HTTP-style test actions to gRPC RPCs using the generated proto stubs from
`ojs-proto`:

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

gRPC status codes are mapped to HTTP status codes for assertion compatibility
(e.g., `codes.NotFound` → `404`, `codes.InvalidArgument` → `400`).

The same JSON test definitions work for both the HTTP and gRPC runners.
