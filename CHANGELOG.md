# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.2.0](https://github.com/openjobspec/ojs-conformance/compare/v0.1.0...v0.2.0) (2026-02-28)


### Features

* add conformance badge portal server ([81b86ed](https://github.com/openjobspec/ojs-conformance/commit/81b86eda84463a4db26e8eb4e00ae2e9937d215a))
* add monitoring dashboard configuration ([94c9660](https://github.com/openjobspec/ojs-conformance/commit/94c96601bc23674d016d4474007820767dd6970f))


### Bug Fixes

* correct assertion timing in async tests ([579e853](https://github.com/openjobspec/ojs-conformance/commit/579e8537fbbfb9ebb6d2f025d33e97c663c913f8))

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
