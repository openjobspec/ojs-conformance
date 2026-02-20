package main

// Adapter layer: translates HTTP-oriented test definitions into gRPC calls.
//
// The conformance test suites are written in terms of HTTP verbs and paths
// (e.g., "POST /ojs/v1/jobs"). This file provides the mapping from those
// HTTP actions to the corresponding gRPC RPC method names so that the same
// JSON test files work for both protocols.
//
// The actual RPC dispatch lives in client.go (CallRPC); this file is
// concerned only with the route-resolution and status-code translation
// that bridges the two worlds.

import "google.golang.org/grpc/codes"

// --- HTTP path → gRPC method routing ---

// RouteMapping describes a single HTTP-to-gRPC route entry.
type RouteMapping struct {
	// HTTPAction is the HTTP verb (GET, POST, DELETE).
	HTTPAction string
	// PathPrefix is matched against the test step's path. Exact matches
	// are tried first; prefix matches handle paths containing dynamic IDs.
	PathPrefix string
	// RPCMethod is the gRPC method name dispatched by CallRPC.
	RPCMethod string
	// Exact means the path must match exactly (no prefix matching).
	Exact bool
}

// routeTable defines the ordered set of HTTP → gRPC mappings used by
// resolveMethod. Exact matches are listed before prefix matches so that
// "/ojs/v1/jobs" doesn't shadow "/ojs/v1/jobs/batch".
var routeTable = []RouteMapping{
	// --- System ---
	{HTTPAction: "GET", PathPrefix: "/ojs/manifest", RPCMethod: "Manifest", Exact: true},
	{HTTPAction: "GET", PathPrefix: "/ojs/v1/health", RPCMethod: "Health", Exact: true},

	// --- Jobs ---
	{HTTPAction: "POST", PathPrefix: "/ojs/v1/jobs/batch", RPCMethod: "EnqueueBatch", Exact: true},
	{HTTPAction: "POST", PathPrefix: "/ojs/v1/jobs", RPCMethod: "Enqueue", Exact: true},
	{HTTPAction: "GET", PathPrefix: "/ojs/v1/jobs/", RPCMethod: "GetJob"},
	{HTTPAction: "DELETE", PathPrefix: "/ojs/v1/jobs/", RPCMethod: "CancelJob"},

	// --- Workers ---
	{HTTPAction: "POST", PathPrefix: "/ojs/v1/workers/fetch", RPCMethod: "Fetch", Exact: true},
	{HTTPAction: "POST", PathPrefix: "/ojs/v1/workers/ack", RPCMethod: "Ack", Exact: true},
	{HTTPAction: "POST", PathPrefix: "/ojs/v1/workers/nack", RPCMethod: "Nack", Exact: true},
	{HTTPAction: "POST", PathPrefix: "/ojs/v1/workers/heartbeat", RPCMethod: "Heartbeat", Exact: true},

	// --- Queues ---
	{HTTPAction: "GET", PathPrefix: "/ojs/v1/queues", RPCMethod: "ListQueues", Exact: true},
	{HTTPAction: "GET", PathPrefix: "/ojs/v1/queues/", RPCMethod: "QueueStats"},
	{HTTPAction: "POST", PathPrefix: "/ojs/v1/queues/", RPCMethod: "PauseOrResumeQueue"},

	// --- Dead letter ---
	{HTTPAction: "GET", PathPrefix: "/ojs/v1/dead-letter", RPCMethod: "ListDeadLetter", Exact: true},
	{HTTPAction: "POST", PathPrefix: "/ojs/v1/dead-letter/", RPCMethod: "RetryDeadLetter"},
	{HTTPAction: "DELETE", PathPrefix: "/ojs/v1/dead-letter/", RPCMethod: "DeleteDeadLetter"},

	// --- Cron ---
	{HTTPAction: "GET", PathPrefix: "/ojs/v1/cron", RPCMethod: "ListCron", Exact: true},
	{HTTPAction: "POST", PathPrefix: "/ojs/v1/cron", RPCMethod: "RegisterCron", Exact: true},
	{HTTPAction: "DELETE", PathPrefix: "/ojs/v1/cron/", RPCMethod: "UnregisterCron"},

	// --- Workflows ---
	{HTTPAction: "POST", PathPrefix: "/ojs/v1/workflows", RPCMethod: "CreateWorkflow", Exact: true},
	{HTTPAction: "GET", PathPrefix: "/ojs/v1/workflows/", RPCMethod: "GetWorkflow"},
	{HTTPAction: "DELETE", PathPrefix: "/ojs/v1/workflows/", RPCMethod: "CancelWorkflow"},
}

// ResolveRoute finds the gRPC method name for an HTTP action + path pair.
// Returns "" if no route matches.
func ResolveRoute(action, path string) string {
	// Exact matches first.
	for _, r := range routeTable {
		if r.Exact && r.HTTPAction == action && r.PathPrefix == path {
			return r.RPCMethod
		}
	}
	// Prefix matches second.
	for _, r := range routeTable {
		if !r.Exact && r.HTTPAction == action && len(path) >= len(r.PathPrefix) && path[:len(r.PathPrefix)] == r.PathPrefix {
			return r.RPCMethod
		}
	}
	return ""
}

// --- gRPC status code → HTTP status code translation ---

// GRPCCodeToHTTPStatus maps a gRPC status code to the closest HTTP
// equivalent so that the existing HTTP-based test assertions work
// unchanged against a gRPC server.
func GRPCCodeToHTTPStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return 200
	case codes.InvalidArgument:
		return 400
	case codes.Unauthenticated:
		return 401
	case codes.PermissionDenied:
		return 403
	case codes.NotFound:
		return 404
	case codes.AlreadyExists:
		return 409
	case codes.FailedPrecondition:
		return 412
	case codes.ResourceExhausted:
		return 429
	case codes.Internal:
		return 500
	case codes.Unimplemented:
		return 501
	case codes.Unavailable:
		return 503
	case codes.DeadlineExceeded:
		return 504
	default:
		return 500
	}
}

// --- HTTP status overrides for specific RPCs ---

// HTTPStatusCreated is the override used by RPCs that map to HTTP 201.
const HTTPStatusCreated = 201

// RPCCreatesResource returns true if the named RPC corresponds to an
// HTTP endpoint that returns 201 Created on success.
func RPCCreatesResource(method string) bool {
	switch method {
	case "Enqueue", "EnqueueBatch", "RegisterCron", "CreateWorkflow":
		return true
	default:
		return false
	}
}
