// Package main implements the OJS conformance gRPC test runner.
//
// This runner executes the same conformance test suites as the HTTP runner,
// but communicates with the OJS server via gRPC instead of HTTP. It maps
// HTTP test actions to equivalent gRPC RPCs and translates assertions from
// HTTP status codes to gRPC status codes.
//
// Usage:
//
//	ojs-conformance-grpc-runner -addr localhost:9090 -suites ./suites
//	ojs-conformance-grpc-runner -addr localhost:9090 -suites ./suites -level 1
//	ojs-conformance-grpc-runner -addr localhost:9090 -suites ./suites -output json
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/openjobspec/ojs-conformance/lib"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	ojsv1 "github.com/openjobspec/ojs-proto/gen/go/ojs/v1"
)

// httpStatusToGRPCCode maps expected HTTP status codes to gRPC codes for assertion.
var httpStatusToGRPCCode = map[int]codes.Code{
	200: codes.OK,
	201: codes.OK,
	204: codes.OK,
	400: codes.InvalidArgument,
	404: codes.NotFound,
	409: codes.AlreadyExists,
	422: codes.InvalidArgument,
	429: codes.ResourceExhausted,
	500: codes.Internal,
	503: codes.Unavailable,
}

// actionToRPC maps HTTP path patterns to gRPC method names.
var actionToRPC = map[string]string{
	"POST /ojs/v1/jobs":              "Enqueue",
	"GET /ojs/v1/jobs/":              "GetJob",
	"DELETE /ojs/v1/jobs/":           "CancelJob",
	"POST /ojs/v1/jobs/batch":        "EnqueueBatch",
	"POST /ojs/v1/workers/fetch":     "Fetch",
	"POST /ojs/v1/workers/ack":       "Ack",
	"POST /ojs/v1/workers/nack":      "Nack",
	"POST /ojs/v1/workers/heartbeat": "Heartbeat",
	"GET /ojs/v1/queues":             "ListQueues",
	"GET /ojs/v1/queues/":            "QueueStats",
	"POST /ojs/v1/queues/":           "PauseOrResumeQueue",
	"GET /ojs/v1/dead-letter":        "ListDeadLetter",
	"POST /ojs/v1/dead-letter/":      "RetryDeadLetter",
	"DELETE /ojs/v1/dead-letter/":    "DeleteDeadLetter",
	"GET /ojs/v1/cron":               "ListCron",
	"POST /ojs/v1/cron":              "RegisterCron",
	"DELETE /ojs/v1/cron/":           "UnregisterCron",
	"POST /ojs/v1/workflows":         "CreateWorkflow",
	"GET /ojs/v1/workflows/":         "GetWorkflow",
	"DELETE /ojs/v1/workflows/":      "CancelWorkflow",
	"GET /ojs/manifest":              "Manifest",
	"GET /ojs/v1/health":             "Health",
}

func main() {
	var (
		grpcAddr     string
		suitesDir    string
		level        int
		category     string
		testID       string
		outputFormat string
		verbose      bool
		timeoutSec   int
	)

	flag.StringVar(&grpcAddr, "addr", "localhost:9090", "gRPC server address (host:port)")
	flag.StringVar(&suitesDir, "suites", "./suites", "Path to test suite directory")
	flag.IntVar(&level, "level", -1, "Filter by conformance level (0-4), -1 for all")
	flag.StringVar(&category, "category", "", "Filter by category")
	flag.StringVar(&testID, "test", "", "Run a single test by ID")
	flag.StringVar(&outputFormat, "output", "table", "Output format: table or json")
	flag.BoolVar(&verbose, "verbose", false, "Show detailed step results")
	flag.IntVar(&timeoutSec, "timeout", 30, "Per-step timeout in seconds")
	flag.Parse()

	// Connect to gRPC server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to gRPC server at %s: %v\n", grpcAddr, err)
		os.Exit(1)
	}
	defer conn.Close()

	client := ojsv1.NewOJSServiceClient(conn)

	// Load test cases
	tests, err := loadTests(suitesDir, level, category, testID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load tests: %v\n", err)
		os.Exit(1)
	}

	if len(tests) == 0 {
		fmt.Println("No tests matched the given filters.")
		os.Exit(0)
	}

	// Run tests
	startTime := time.Now()
	results := make([]lib.TestResult, 0, len(tests))

	for _, tc := range tests {
		result := runGRPCTest(client, tc, time.Duration(timeoutSec)*time.Second)
		results = append(results, result)
	}

	totalDuration := time.Since(startTime)

	// Build report
	report := buildReport(grpcAddr, level, results, totalDuration)

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(report)
	} else {
		printTable(report, verbose)
	}

	if !report.Conformant {
		os.Exit(1)
	}
}

// runGRPCTest executes a single conformance test over gRPC.
func runGRPCTest(client ojsv1.OJSServiceClient, tc lib.TestCase, timeout time.Duration) lib.TestResult {
	result := lib.TestResult{
		TestID:   tc.TestID,
		Name:     tc.Name,
		Level:    tc.Level,
		Category: tc.Category,
		SpecRef:  tc.SpecRef,
		FilePath: tc.FilePath,
		Status:   "pass",
	}

	start := time.Now()
	stepResults := make(map[string]*lib.StepResult)

	// Run setup steps
	if tc.Setup != nil {
		for _, step := range tc.Setup.Steps {
			runGRPCStep(client, step, stepResults, timeout)
		}
	}

	// Run test steps
	for _, step := range tc.Steps {
		if step.DelayMs > 0 {
			time.Sleep(time.Duration(step.DelayMs) * time.Millisecond)
		}

		sr := runGRPCStep(client, step, stepResults, timeout)
		result.StepResults = append(result.StepResults, *sr)

		// Check assertions
		if step.Assertions != nil {
			failures := checkGRPCAssertions(step, sr)
			if len(failures) > 0 {
				result.Status = "fail"
				result.Failures = append(result.Failures, failures...)
			}
		}
	}

	// Run teardown steps
	if tc.Teardown != nil {
		for _, step := range tc.Teardown.Steps {
			runGRPCStep(client, step, stepResults, timeout)
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

// runGRPCStep executes a single test step by calling the appropriate gRPC method.
func runGRPCStep(client ojsv1.OJSServiceClient, step lib.Step, results map[string]*lib.StepResult, timeout time.Duration) *lib.StepResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	sr := &lib.StepResult{
		StepID: step.ID,
	}

	// Parse the body
	var body map[string]any
	if step.Body != nil {
		json.Unmarshal(step.Body, &body)
	}

	// Determine gRPC method from action + path
	action := step.Action
	path := step.Path
	method := resolveMethod(action, path)

	var respJSON []byte
	var grpcErr error

	switch method {
	case "Enqueue":
		resp, err := client.Enqueue(ctx, buildEnqueueRequest(body))
		grpcErr = err
		if resp != nil {
			respJSON, _ = json.Marshal(map[string]any{"job": protoJobToMap(resp.Job)})
		}

	case "GetJob":
		id := extractIDFromPath(path, "/ojs/v1/jobs/")
		resp, err := client.GetJob(ctx, &ojsv1.GetJobRequest{JobId: id})
		grpcErr = err
		if resp != nil {
			respJSON, _ = json.Marshal(protoJobToMap(resp.Job))
		}

	case "CancelJob":
		id := extractIDFromPath(path, "/ojs/v1/jobs/")
		resp, err := client.CancelJob(ctx, &ojsv1.CancelJobRequest{JobId: id})
		grpcErr = err
		if resp != nil {
			respJSON, _ = json.Marshal(protoJobToMap(resp.Job))
		}

	case "Fetch":
		resp, err := client.Fetch(ctx, buildFetchRequest(body))
		grpcErr = err
		if resp != nil {
			jobs := make([]map[string]any, 0)
			for _, j := range resp.Jobs {
				jobs = append(jobs, protoJobToMap(j))
			}
			respJSON, _ = json.Marshal(map[string]any{"jobs": jobs})
		}

	case "Ack":
		resp, err := client.Ack(ctx, buildAckRequest(body))
		grpcErr = err
		if resp != nil {
			respJSON, _ = json.Marshal(map[string]any{
				"acknowledged": resp.Acknowledged,
			})
		}

	case "Nack":
		resp, err := client.Nack(ctx, buildNackRequest(body))
		grpcErr = err
		if resp != nil {
			result := map[string]any{
				"state": resp.State.String(),
			}
			if resp.NextAttemptAt != nil {
				result["next_attempt_at"] = resp.NextAttemptAt.AsTime().Format(time.RFC3339Nano)
			}
			respJSON, _ = json.Marshal(result)
		}

	case "Heartbeat":
		resp, err := client.Heartbeat(ctx, buildHeartbeatRequest(body))
		grpcErr = err
		if resp != nil {
			result := map[string]any{
				"directed_state": resp.DirectedState.String(),
			}
			if resp.NewDeadline != nil {
				result["new_deadline"] = resp.NewDeadline.AsTime().Format(time.RFC3339Nano)
			}
			respJSON, _ = json.Marshal(result)
		}

	case "ListQueues":
		resp, err := client.ListQueues(ctx, &ojsv1.ListQueuesRequest{})
		grpcErr = err
		if resp != nil {
			queues := make([]map[string]any, 0)
			for _, q := range resp.Queues {
				queues = append(queues, map[string]any{"name": q.Name, "paused": q.Paused})
			}
			respJSON, _ = json.Marshal(map[string]any{"queues": queues})
		}

	case "Health":
		resp, err := client.Health(ctx, &ojsv1.HealthRequest{})
		grpcErr = err
		if resp != nil {
			respJSON, _ = json.Marshal(map[string]any{"status": resp.Status.String()})
		}

	case "Manifest":
		resp, err := client.Manifest(ctx, &ojsv1.ManifestRequest{})
		grpcErr = err
		if resp != nil {
			respJSON, _ = json.Marshal(map[string]any{
				"ojs_version":       resp.OjsVersion,
				"conformance_level": resp.ConformanceLevel,
				"protocols":         resp.Protocols,
				"backend":           resp.Backend,
			})
		}

	default:
		// Unsupported method - mark as skipped
		sr.StatusCode = 0
		sr.DurationMs = time.Since(start).Milliseconds()
		results[step.ID] = sr
		return sr
	}

	sr.DurationMs = time.Since(start).Milliseconds()

	if grpcErr != nil {
		st, ok := status.FromError(grpcErr)
		if ok {
			sr.StatusCode = grpcCodeToHTTPStatus(st.Code())
			errBody := map[string]any{
				"error": map[string]any{
					"code":    st.Code().String(),
					"message": st.Message(),
				},
			}
			sr.Body, _ = json.Marshal(errBody)
		} else {
			sr.StatusCode = 500
		}
	} else {
		sr.StatusCode = 200
		sr.Body = respJSON
	}

	// Parse body for assertion lookups
	if sr.Body != nil {
		json.Unmarshal(sr.Body, &sr.Parsed)
	}

	results[step.ID] = sr
	return sr
}

// --- Request Builders ---

func buildEnqueueRequest(body map[string]any) *ojsv1.EnqueueRequest {
	req := &ojsv1.EnqueueRequest{}
	if v, ok := body["type"].(string); ok {
		req.Type = v
	}
	if v, ok := body["queue"].(string); ok {
		if req.Options == nil {
			req.Options = &ojsv1.EnqueueOptions{}
		}
		req.Options.Queue = v
	}
	if args, ok := body["args"].([]any); ok {
		for _, a := range args {
			if v, err := structpb.NewValue(a); err == nil {
				req.Args = append(req.Args, v)
			}
		}
	}
	return req
}

func buildFetchRequest(body map[string]any) *ojsv1.FetchRequest {
	req := &ojsv1.FetchRequest{}
	if queues, ok := body["queues"].([]any); ok {
		for _, q := range queues {
			if s, ok := q.(string); ok {
				req.Queues = append(req.Queues, s)
			}
		}
	}
	if v, ok := body["worker_id"].(string); ok {
		req.WorkerId = v
	}
	if v, ok := body["max_jobs"].(float64); ok {
		req.Count = int32(v)
	}
	return req
}

func buildAckRequest(body map[string]any) *ojsv1.AckRequest {
	req := &ojsv1.AckRequest{}
	if v, ok := body["job_id"].(string); ok {
		req.JobId = v
	}
	return req
}

func buildNackRequest(body map[string]any) *ojsv1.NackRequest {
	req := &ojsv1.NackRequest{}
	if v, ok := body["job_id"].(string); ok {
		req.JobId = v
	}
	if errMap, ok := body["error"].(map[string]any); ok {
		req.Error = &ojsv1.JobError{}
		if v, ok := errMap["message"].(string); ok {
			req.Error.Message = v
		}
		if v, ok := errMap["type"].(string); ok {
			req.Error.Code = v
		}
	}
	return req
}

func buildHeartbeatRequest(body map[string]any) *ojsv1.HeartbeatRequest {
	req := &ojsv1.HeartbeatRequest{}
	if v, ok := body["job_id"].(string); ok {
		req.Id = v
	}
	if v, ok := body["worker_id"].(string); ok {
		req.WorkerId = v
	}
	return req
}

// --- Helpers ---

func resolveMethod(action, path string) string {
	key := action + " " + path
	if m, ok := actionToRPC[key]; ok {
		return m
	}
	// Try prefix matching
	for pattern, method := range actionToRPC {
		parts := strings.SplitN(pattern, " ", 2)
		if len(parts) == 2 && parts[0] == action && strings.HasPrefix(path, parts[1]) {
			return method
		}
	}
	return ""
}

func extractIDFromPath(path, prefix string) string {
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func grpcCodeToHTTPStatus(code codes.Code) int {
	switch code {
	case codes.OK:
		return 200
	case codes.InvalidArgument:
		return 400
	case codes.NotFound:
		return 404
	case codes.AlreadyExists:
		return 409
	case codes.ResourceExhausted:
		return 429
	case codes.Internal:
		return 500
	case codes.Unavailable:
		return 503
	case codes.Unimplemented:
		return 501
	default:
		return 500
	}
}

func protoJobToMap(j *ojsv1.Job) map[string]any {
	if j == nil {
		return nil
	}
	m := map[string]any{
		"id":      j.Id,
		"type":    j.Type,
		"queue":   j.Queue,
		"state":   j.State,
		"attempt": j.Attempt,
	}
	if j.CreatedAt != nil {
		m["created_at"] = j.CreatedAt.AsTime().Format("2006-01-02T15:04:05Z07:00")
	}
	return m
}

// --- Test Loading (shared with HTTP runner) ---

func loadTests(dir string, level int, category, testID string) ([]lib.TestCase, error) {
	var tests []lib.TestCase

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var tc lib.TestCase
		if err := json.Unmarshal(data, &tc); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		tc.FilePath = path

		if level >= 0 && tc.Level != level {
			return nil
		}
		if category != "" && tc.Category != category {
			return nil
		}
		if testID != "" && tc.TestID != testID {
			return nil
		}

		tests = append(tests, tc)
		return nil
	})

	sort.Slice(tests, func(i, j int) bool {
		if tests[i].Level != tests[j].Level {
			return tests[i].Level < tests[j].Level
		}
		return tests[i].TestID < tests[j].TestID
	})

	return tests, err
}

// --- Assertions ---

func checkGRPCAssertions(step lib.Step, sr *lib.StepResult) []lib.Failure {
	var failures []lib.Failure

	if step.Assertions.Status != nil {
		var expected int
		json.Unmarshal(step.Assertions.Status, &expected)
		if sr.StatusCode != expected {
			failures = append(failures, lib.Failure{
				StepID:   step.ID,
				Field:    "status",
				Expected: fmt.Sprintf("%d", expected),
				Actual:   fmt.Sprintf("%d (mapped from gRPC)", sr.StatusCode),
				Message:  "gRPC status code mismatch (mapped to HTTP equivalent)",
			})
		}
	}

	if len(step.Assertions.Body) > 0 && sr.Parsed != nil {
		for field, expectedRaw := range step.Assertions.Body {
			actual, err := lib.ResolveJSONPath(field, sr.Parsed)
			if err != nil {
				failures = append(failures, lib.Failure{
					StepID:   step.ID,
					Field:    field,
					Expected: string(expectedRaw),
					Actual:   "not found",
					Message:  fmt.Sprintf("field %s: %v", field, err),
				})
				continue
			}
			if err := lib.MatchAssertion(expectedRaw, actual); err != nil {
				failures = append(failures, lib.Failure{
					StepID:   step.ID,
					Field:    field,
					Expected: string(expectedRaw),
					Actual:   fmt.Sprintf("%v", actual),
					Message:  err.Error(),
				})
			}
		}
	}

	return failures
}

// --- Reporting ---

func buildReport(target string, requestedLevel int, results []lib.TestResult, duration time.Duration) lib.SuiteReport {
	report := lib.SuiteReport{
		TestSuiteVersion: "1.0.0-rc.1",
		Target:           "grpc://" + target,
		RunAt:            time.Now().UTC().Format(time.RFC3339),
		DurationMs:       duration.Milliseconds(),
		RequestedLevel:   requestedLevel,
		Results: lib.ResultsSummary{
			ByLevel: make(map[int]lib.LevelSummary),
		},
	}

	for _, r := range results {
		report.Results.Total++
		ls := report.Results.ByLevel[r.Level]
		ls.Total++

		switch r.Status {
		case "pass":
			report.Results.Passed++
			ls.Passed++
		case "fail":
			report.Results.Failed++
			ls.Failed++
			report.Failures = append(report.Failures, r)
		case "skip":
			report.Results.Skipped++
			ls.Skipped++
			report.Skipped = append(report.Skipped, r)
		case "error":
			report.Results.Errored++
			ls.Errored++
		}

		report.Results.ByLevel[r.Level] = ls
	}

	// Determine conformance
	report.Conformant = report.Results.Failed == 0 && report.Results.Errored == 0
	report.ConformantLevel = -1
	for lvl := 0; lvl <= 4; lvl++ {
		ls, ok := report.Results.ByLevel[lvl]
		if !ok {
			continue
		}
		ls.AllPass = ls.Failed == 0 && ls.Errored == 0
		report.Results.ByLevel[lvl] = ls
		if ls.AllPass {
			report.ConformantLevel = lvl
		} else {
			break
		}
	}

	return report
}

func printTable(report lib.SuiteReport, verbose bool) {
	fmt.Printf("\nOJS gRPC Conformance Results — %s\n", report.Target)
	fmt.Printf("Suite: %s | Duration: %dms\n\n", report.TestSuiteVersion, report.DurationMs)

	fmt.Printf("  %-8s %-6s %-6s %-6s %-6s\n", "Level", "Total", "Pass", "Fail", "Skip")
	fmt.Printf("  %-8s %-6s %-6s %-6s %-6s\n", "-----", "-----", "----", "----", "----")

	for lvl := 0; lvl <= 4; lvl++ {
		ls, ok := report.Results.ByLevel[lvl]
		if !ok {
			continue
		}
		icon := "✅"
		if !ls.AllPass {
			icon = "❌"
		}
		fmt.Printf("  %s L%d    %-6d %-6d %-6d %-6d\n", icon, lvl, ls.Total, ls.Passed, ls.Failed, ls.Skipped)
	}

	fmt.Printf("\n  Total: %d | Passed: %d | Failed: %d | Skipped: %d\n",
		report.Results.Total, report.Results.Passed, report.Results.Failed, report.Results.Skipped)

	if report.Conformant {
		fmt.Printf("  ✅ Conformant at Level %d\n", report.ConformantLevel)
	} else {
		fmt.Printf("  ❌ Not conformant\n")
	}

	if len(report.Failures) > 0 {
		fmt.Println("\n  Failures:")
		for _, f := range report.Failures {
			fmt.Printf("    %s [%s] %s\n", f.TestID, f.Category, f.Name)
			for _, fail := range f.Failures {
				fmt.Printf("      Step %s: %s (expected: %s, got: %s)\n",
					fail.StepID, fail.Message, fail.Expected, fail.Actual)
			}
		}
	}
	fmt.Println()
}
