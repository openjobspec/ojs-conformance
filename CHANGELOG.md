# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 1.0.0 (2026-02-18)


### Features

* **lib:** add core data types and schemas ([0222827](https://github.com/openjobspec/ojs-conformance/commit/02228279dfd065d45b01fc50a4c49451dfb0b0d1))
* **lib:** add JSON assertion matching system ([633ce2e](https://github.com/openjobspec/ojs-conformance/commit/633ce2e961a3fd35dc76f1faf0dfc7b3e438cb0e))
* **lib:** add timing assertion utilities ([bcf8784](https://github.com/openjobspec/ojs-conformance/commit/bcf8784a2b1fd51db43ccc5df9b555d7194db396))
* **lib:** extend assertion matchers and JSONPath capabilities ([33e3be2](https://github.com/openjobspec/ojs-conformance/commit/33e3be2358d3feab5c86e3cd36123d1bde298337))
* **runner:** add gRPC conformance test runner ([961fb0c](https://github.com/openjobspec/ojs-conformance/commit/961fb0c6524fc543e828b299bdd70c4210ed6ca4))
* **runner:** add Redis test isolation, WAIT action, and flexible assertions ([fe8da46](https://github.com/openjobspec/ojs-conformance/commit/fe8da46ae85d3af6f0eddb6126a1b36543389d40))
* **runner:** implement HTTP conformance test runner ([0c5428b](https://github.com/openjobspec/ojs-conformance/commit/0c5428bfe317a83e3e90f86cead94fba078944cd))


### Bug Fixes

* correct attempt numbering in conformance tests to 1-indexed ([a1b7141](https://github.com/openjobspec/ojs-conformance/commit/a1b71410871a2556aeda846900099f4c2766727c))

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
