# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Conformance test suites organized by level (0–4) and extension
- Level 0 (Core): Job envelope validation, basic CRUD operations, lifecycle transitions
- Level 1 (Reliable): Retry policies, backoff strategies, dead letter queue handling
- Level 2 (Scheduled): Delayed job execution, cron expressions, timezone support
- Level 3 (Workflows): DAG execution, fan-out/fan-in, unique jobs, batch operations
- Level 4 (Advanced): Middleware chains, lifecycle events, full feature validation
- Extension suites: admin-api, backpressure, dead-letter, rate-limiting
- HTTP test runner (`runner/http/`) for executing suites against any OJS server
- gRPC test runner (`runner/grpc/`) for gRPC protocol binding validation
- Rich assertion library (`lib/`) with JSONPath, regex, range, and type matchers
- 145 JSON test definition files
- README with structure overview and usage instructions

