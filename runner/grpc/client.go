package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ojsv1 "github.com/openjobspec/ojs-proto/gen/go/ojs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	grpcInsecure "google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// OJSClient wraps the generated gRPC client with convenience methods.
type OJSClient struct {
	conn   *grpc.ClientConn
	client ojsv1.OJSServiceClient
}

// ConnectOptions configures the gRPC dial behaviour.
type ConnectOptions struct {
	TLS      bool // Use TLS transport credentials.
	Insecure bool // Skip TLS certificate verification (requires TLS=true).
}

// NewOJSClient connects to the gRPC server and returns a client wrapper.
func NewOJSClient(ctx context.Context, addr string, opts ConnectOptions) (*OJSClient, error) {
	var dialOpts []grpc.DialOption

	switch {
	case opts.TLS && opts.Insecure:
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}))) //nolint:gosec // user-requested skip
	case opts.TLS:
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	default:
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(grpcInsecure.NewCredentials()))
	}

	dialOpts = append(dialOpts, grpc.WithBlock())

	conn, err := grpc.DialContext(ctx, addr, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", addr, err)
	}
	return &OJSClient{conn: conn, client: ojsv1.NewOJSServiceClient(conn)}, nil
}

// Close closes the underlying gRPC connection.
func (c *OJSClient) Close() error {
	return c.conn.Close()
}

// RPCResult holds the result of a gRPC call, normalized to JSON for assertions.
type RPCResult struct {
	// ResponseJSON is the JSON-serialized response body.
	ResponseJSON []byte
	// GRPCCode is the gRPC status code (codes.OK on success).
	GRPCCode codes.Code
	// GRPCMessage is the gRPC error message (empty on success).
	GRPCMessage string
	// HTTPStatusOverride allows specific RPCs to override the HTTP status code
	// mapping (e.g., Enqueue returns 201 Created on success, not 200 OK).
	HTTPStatusOverride int
}

// CallRPC dispatches a test step to the appropriate gRPC RPC.
func (c *OJSClient) CallRPC(ctx context.Context, method string, path string, body map[string]any) (*RPCResult, error) {
	switch method {
	case "Enqueue":
		return c.enqueue(ctx, body)
	case "EnqueueBatch":
		return c.enqueueBatch(ctx, body)
	case "GetJob":
		return c.getJob(ctx, extractIDFromPath(path, "/ojs/v1/jobs/"))
	case "CancelJob":
		return c.cancelJob(ctx, extractIDFromPath(path, "/ojs/v1/jobs/"), body)
	case "Fetch":
		return c.fetch(ctx, body)
	case "Ack":
		return c.ack(ctx, body)
	case "Nack":
		return c.nack(ctx, body)
	case "Heartbeat":
		return c.heartbeat(ctx, body)
	case "ListQueues":
		return c.listQueues(ctx)
	case "QueueStats":
		return c.queueStats(ctx, path)
	case "PauseQueue":
		return c.pauseQueue(ctx, path)
	case "ResumeQueue":
		return c.resumeQueue(ctx, path)
	case "ListDeadLetter":
		return c.listDeadLetter(ctx, body)
	case "RetryDeadLetter":
		return c.retryDeadLetter(ctx, path)
	case "DeleteDeadLetter":
		return c.deleteDeadLetter(ctx, path)
	case "RegisterCron":
		return c.registerCron(ctx, body)
	case "UnregisterCron":
		return c.unregisterCron(ctx, path)
	case "ListCron":
		return c.listCron(ctx)
	case "CreateWorkflow":
		return c.createWorkflow(ctx, body)
	case "GetWorkflow":
		return c.getWorkflow(ctx, path)
	case "CancelWorkflow":
		return c.cancelWorkflow(ctx, path)
	case "Health":
		return c.health(ctx)
	case "Manifest":
		return c.manifest(ctx)
	default:
		return nil, fmt.Errorf("unsupported gRPC method: %s", method)
	}
}

// --- RPC implementations ---

func (c *OJSClient) enqueue(ctx context.Context, body map[string]any) (*RPCResult, error) {
	req := &ojsv1.EnqueueRequest{}
	if v, ok := body["type"].(string); ok {
		req.Type = v
	}
	if args, ok := body["args"].([]any); ok {
		for _, a := range args {
			if v, err := structpb.NewValue(a); err == nil {
				req.Args = append(req.Args, v)
			}
		}
	}
	if opts := buildEnqueueOptions(body); opts != nil {
		req.Options = opts
	}

	resp, err := c.client.Enqueue(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{"job": protoJobToMap(resp.Job)})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK, HTTPStatusOverride: 201}, nil
}

func (c *OJSClient) enqueueBatch(ctx context.Context, body map[string]any) (*RPCResult, error) {
	req := &ojsv1.EnqueueBatchRequest{}
	if jobs, ok := body["jobs"].([]any); ok {
		for _, j := range jobs {
			if jm, ok := j.(map[string]any); ok {
				entry := &ojsv1.BatchJobEntry{}
				if v, ok := jm["type"].(string); ok {
					entry.Type = v
				}
				if args, ok := jm["args"].([]any); ok {
					for _, a := range args {
						if v, err := structpb.NewValue(a); err == nil {
							entry.Args = append(entry.Args, v)
						}
					}
				}
				if opts := buildEnqueueOptions(jm); opts != nil {
					entry.Options = opts
				}
				req.Jobs = append(req.Jobs, entry)
			}
		}
	}
	if opts := buildEnqueueOptions(body); opts != nil {
		req.DefaultOptions = opts
	}

	resp, err := c.client.EnqueueBatch(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	jobs := make([]map[string]any, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		jobs = append(jobs, protoJobToMap(j))
	}
	respJSON, _ := json.Marshal(map[string]any{"jobs": jobs, "count": resp.Count})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK, HTTPStatusOverride: 201}, nil
}

func (c *OJSClient) getJob(ctx context.Context, jobID string) (*RPCResult, error) {
	resp, err := c.client.GetJob(ctx, &ojsv1.GetJobRequest{JobId: jobID})
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{"job": protoJobToMap(resp.Job)})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) cancelJob(ctx context.Context, jobID string, body map[string]any) (*RPCResult, error) {
	req := &ojsv1.CancelJobRequest{JobId: jobID}
	if body != nil {
		if v, ok := body["reason"].(string); ok {
			req.Reason = v
		}
	}
	resp, err := c.client.CancelJob(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{"job": protoJobToMap(resp.Job)})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) fetch(ctx context.Context, body map[string]any) (*RPCResult, error) {
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
	if v, ok := body["count"].(float64); ok {
		req.Count = int32(v)
	}

	resp, err := c.client.Fetch(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	jobs := make([]map[string]any, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		jobs = append(jobs, protoJobToMap(j))
	}
	respJSON, _ := json.Marshal(map[string]any{"jobs": jobs})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) ack(ctx context.Context, body map[string]any) (*RPCResult, error) {
	req := &ojsv1.AckRequest{}
	if v, ok := body["job_id"].(string); ok {
		req.JobId = v
	}
	if body["result"] != nil {
		if resultMap, ok := body["result"].(map[string]any); ok {
			if s, err := structpb.NewStruct(resultMap); err == nil {
				req.Result = s
			}
		}
	}
	resp, err := c.client.Ack(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{"acknowledged": resp.Acknowledged})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) nack(ctx context.Context, body map[string]any) (*RPCResult, error) {
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
		if v, ok := errMap["code"].(string); ok {
			req.Error.Code = v
		}
	}

	resp, err := c.client.Nack(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	result := map[string]any{
		"state": strings.ToLower(strings.TrimPrefix(resp.State.String(), "JOB_STATE_")),
	}
	if resp.NextAttemptAt != nil {
		result["next_attempt_at"] = resp.NextAttemptAt.AsTime().Format(time.RFC3339Nano)
	}
	respJSON, _ := json.Marshal(result)
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) heartbeat(ctx context.Context, body map[string]any) (*RPCResult, error) {
	req := &ojsv1.HeartbeatRequest{}
	if v, ok := body["job_id"].(string); ok {
		req.Id = v
	}
	if v, ok := body["id"].(string); ok {
		req.Id = v
	}
	if v, ok := body["worker_id"].(string); ok {
		req.WorkerId = v
	}
	if v, ok := body["extend_by"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			req.ExtendBy = durationpb.New(d)
		}
	}
	if v, ok := body["extend_by"].(float64); ok {
		// Interpret as seconds
		req.ExtendBy = durationpb.New(time.Duration(v) * time.Second)
	}

	resp, err := c.client.Heartbeat(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	result := map[string]any{
		"directed_state": strings.ToLower(strings.TrimPrefix(resp.DirectedState.String(), "WORKER_STATE_")),
	}
	if resp.NewDeadline != nil {
		result["new_deadline"] = resp.NewDeadline.AsTime().Format(time.RFC3339Nano)
	}
	respJSON, _ := json.Marshal(result)
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) listQueues(ctx context.Context) (*RPCResult, error) {
	resp, err := c.client.ListQueues(ctx, &ojsv1.ListQueuesRequest{})
	if err != nil {
		return grpcError(err), nil
	}
	queues := make([]map[string]any, 0, len(resp.Queues))
	for _, q := range resp.Queues {
		queues = append(queues, map[string]any{"name": q.Name, "paused": q.Paused})
	}
	respJSON, _ := json.Marshal(map[string]any{"queues": queues})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) queueStats(ctx context.Context, path string) (*RPCResult, error) {
	queue := extractIDFromPath(path, "/ojs/v1/queues/")
	queue = strings.TrimSuffix(queue, "/stats")
	resp, err := c.client.QueueStats(ctx, &ojsv1.QueueStatsRequest{Queue: queue})
	if err != nil {
		return grpcError(err), nil
	}
	result := map[string]any{"queue": resp.Queue}
	if resp.Stats != nil {
		result["stats"] = map[string]any{
			"available": resp.Stats.Available,
			"active":    resp.Stats.Active,
			"scheduled": resp.Stats.Scheduled,
			"retryable": resp.Stats.Retryable,
			"dead":      resp.Stats.Dead,
			"paused":    resp.Stats.Paused,
		}
	}
	respJSON, _ := json.Marshal(result)
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) pauseQueue(ctx context.Context, path string) (*RPCResult, error) {
	queue := extractIDFromPath(path, "/ojs/v1/queues/")
	queue = strings.TrimSuffix(queue, "/pause")
	_, err := c.client.PauseQueue(ctx, &ojsv1.PauseQueueRequest{Queue: queue})
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) resumeQueue(ctx context.Context, path string) (*RPCResult, error) {
	queue := extractIDFromPath(path, "/ojs/v1/queues/")
	queue = strings.TrimSuffix(queue, "/resume")
	_, err := c.client.ResumeQueue(ctx, &ojsv1.ResumeQueueRequest{Queue: queue})
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) listDeadLetter(ctx context.Context, body map[string]any) (*RPCResult, error) {
	req := &ojsv1.ListDeadLetterRequest{}
	if body != nil {
		if v, ok := body["queue"].(string); ok {
			req.Queue = v
		}
		if v, ok := body["limit"].(float64); ok {
			req.Limit = int32(v)
		}
	}
	resp, err := c.client.ListDeadLetter(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	jobs := make([]map[string]any, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		jobs = append(jobs, protoJobToMap(j))
	}
	respJSON, _ := json.Marshal(map[string]any{"jobs": jobs, "total_count": resp.TotalCount})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) retryDeadLetter(ctx context.Context, path string) (*RPCResult, error) {
	jobID := extractIDFromPath(path, "/ojs/v1/dead-letter/")
	jobID = strings.TrimSuffix(jobID, "/retry")
	resp, err := c.client.RetryDeadLetter(ctx, &ojsv1.RetryDeadLetterRequest{JobId: jobID})
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{"job": protoJobToMap(resp.Job)})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) deleteDeadLetter(ctx context.Context, path string) (*RPCResult, error) {
	jobID := extractIDFromPath(path, "/ojs/v1/dead-letter/")
	_, err := c.client.DeleteDeadLetter(ctx, &ojsv1.DeleteDeadLetterRequest{JobId: jobID})
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) registerCron(ctx context.Context, body map[string]any) (*RPCResult, error) {
	req := &ojsv1.RegisterCronRequest{}
	if v, ok := body["name"].(string); ok {
		req.Name = v
	}
	if v, ok := body["cron"].(string); ok {
		req.Cron = v
	}
	if v, ok := body["timezone"].(string); ok {
		req.Timezone = v
	}
	if v, ok := body["type"].(string); ok {
		req.Type = v
	}
	if args, ok := body["args"].([]any); ok {
		for _, a := range args {
			if v, err := structpb.NewValue(a); err == nil {
				req.Args = append(req.Args, v)
			}
		}
	}
	if opts := buildEnqueueOptions(body); opts != nil {
		req.Options = opts
	}
	resp, err := c.client.RegisterCron(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	result := map[string]any{"name": resp.Name}
	if resp.NextRunAt != nil {
		result["next_run_at"] = resp.NextRunAt.AsTime().Format(time.RFC3339Nano)
	}
	respJSON, _ := json.Marshal(result)
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK, HTTPStatusOverride: 201}, nil
}

func (c *OJSClient) unregisterCron(ctx context.Context, path string) (*RPCResult, error) {
	name := extractIDFromPath(path, "/ojs/v1/cron/")
	_, err := c.client.UnregisterCron(ctx, &ojsv1.UnregisterCronRequest{Name: name})
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) listCron(ctx context.Context) (*RPCResult, error) {
	resp, err := c.client.ListCron(ctx, &ojsv1.ListCronRequest{})
	if err != nil {
		return grpcError(err), nil
	}
	entries := make([]map[string]any, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		entry := map[string]any{
			"name":     e.Name,
			"cron":     e.Cron,
			"timezone": e.Timezone,
			"type":     e.Type,
		}
		if e.NextRunAt != nil {
			entry["next_run_at"] = e.NextRunAt.AsTime().Format(time.RFC3339Nano)
		}
		if e.LastRunAt != nil {
			entry["last_run_at"] = e.LastRunAt.AsTime().Format(time.RFC3339Nano)
		}
		entries = append(entries, entry)
	}
	respJSON, _ := json.Marshal(map[string]any{"entries": entries})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) createWorkflow(ctx context.Context, body map[string]any) (*RPCResult, error) {
	req := &ojsv1.CreateWorkflowRequest{}
	if v, ok := body["name"].(string); ok {
		req.Name = v
	}
	if steps, ok := body["steps"].([]any); ok {
		for _, s := range steps {
			if sm, ok := s.(map[string]any); ok {
				step := &ojsv1.WorkflowStep{}
				if v, ok := sm["id"].(string); ok {
					step.Id = v
				}
				if v, ok := sm["type"].(string); ok {
					step.Type = v
				}
				if deps, ok := sm["depends_on"].([]any); ok {
					for _, d := range deps {
						if ds, ok := d.(string); ok {
							step.DependsOn = append(step.DependsOn, ds)
						}
					}
				}
				if args, ok := sm["args"].([]any); ok {
					for _, a := range args {
						if v, err := structpb.NewValue(a); err == nil {
							step.Args = append(step.Args, v)
						}
					}
				}
				if opts := buildEnqueueOptions(sm); opts != nil {
					step.Options = opts
				}
				req.Steps = append(req.Steps, step)
			}
		}
	}

	resp, err := c.client.CreateWorkflow(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{"workflow": protoWorkflowToMap(resp.Workflow)})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK, HTTPStatusOverride: 201}, nil
}

func (c *OJSClient) getWorkflow(ctx context.Context, path string) (*RPCResult, error) {
	wfID := extractIDFromPath(path, "/ojs/v1/workflows/")
	resp, err := c.client.GetWorkflow(ctx, &ojsv1.GetWorkflowRequest{WorkflowId: wfID})
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{"workflow": protoWorkflowToMap(resp.Workflow)})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) cancelWorkflow(ctx context.Context, path string) (*RPCResult, error) {
	wfID := extractIDFromPath(path, "/ojs/v1/workflows/")
	resp, err := c.client.CancelWorkflow(ctx, &ojsv1.CancelWorkflowRequest{WorkflowId: wfID})
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{"workflow": protoWorkflowToMap(resp.Workflow)})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) health(ctx context.Context) (*RPCResult, error) {
	resp, err := c.client.Health(ctx, &ojsv1.HealthRequest{})
	if err != nil {
		return grpcError(err), nil
	}
	statusStr := strings.ToLower(strings.TrimPrefix(resp.Status.String(), "HEALTH_STATUS_"))
	result := map[string]any{"status": statusStr}
	if resp.Timestamp != nil {
		result["timestamp"] = resp.Timestamp.AsTime().Format(time.RFC3339Nano)
	}
	respJSON, _ := json.Marshal(result)
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) manifest(ctx context.Context) (*RPCResult, error) {
	resp, err := c.client.Manifest(ctx, &ojsv1.ManifestRequest{})
	if err != nil {
		return grpcError(err), nil
	}
	// Map proto field names to the HTTP JSON format that the tests expect.
	// The tests assert on $.specversion (HTTP JSON format), while the proto
	// uses ojs_version. We output both for maximum compatibility.
	result := map[string]any{
		"specversion":       resp.OjsVersion,
		"ojs_version":       resp.OjsVersion,
		"conformance_level": resp.ConformanceLevel,
		"protocols":         resp.Protocols,
		"backend":           resp.Backend,
		"schema_validation":  resp.SchemaValidation,
	}
	if resp.Implementation != nil {
		result["implementation"] = map[string]any{
			"name":     resp.Implementation.Name,
			"version":  resp.Implementation.Version,
			"language": resp.Implementation.Language,
		}
	}
	if resp.Extensions != nil {
		result["extensions"] = resp.Extensions
	}
	respJSON, _ := json.Marshal(result)
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

// --- Helpers ---

// grpcError converts a gRPC error into an RPCResult.
func grpcError(err error) *RPCResult {
	st, ok := status.FromError(err)
	if !ok {
		return &RPCResult{GRPCCode: codes.Internal, GRPCMessage: err.Error()}
	}
	errBody, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    st.Code().String(),
			"message": st.Message(),
		},
	})
	return &RPCResult{
		ResponseJSON: errBody,
		GRPCCode:     st.Code(),
		GRPCMessage:  st.Message(),
	}
}

// grpcCodeToHTTPStatus maps gRPC status codes to HTTP equivalents for assertion compatibility.
// Delegates to the adapter layer.
func grpcCodeToHTTPStatus(code codes.Code) int {
	return GRPCCodeToHTTPStatus(code)
}

// resolveMethod determines the gRPC method name from an HTTP action and path.
// Delegates to the adapter layer.
func resolveMethod(action, path string) string {
	return ResolveRoute(action, path)
}

// extractIDFromPath extracts an ID segment from a URL path after a prefix.
func extractIDFromPath(path, prefix string) string {
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// buildEnqueueOptions constructs EnqueueOptions from a JSON body.
// It checks both top-level keys and keys nested under an "options" object,
// since the test JSON format nests options under body.options while the
// proto expects them flattened into EnqueueOptions.
func buildEnqueueOptions(body map[string]any) *ojsv1.EnqueueOptions {
	opts := &ojsv1.EnqueueOptions{}
	hasOpts := false

	// First check for a nested "options" object (HTTP JSON test format)
	src := body
	hasNestedOptions := false
	if optsMap, ok := body["options"].(map[string]any); ok {
		src = optsMap
		hasNestedOptions = true
	}

	if v, ok := src["queue"].(string); ok {
		opts.Queue = v
		hasOpts = true
	}
	if v, ok := src["priority"].(float64); ok {
		opts.Priority = int32(v)
		hasOpts = true
	}
	if v, ok := src["max_attempts"].(float64); ok {
		opts.MaxAttempts = int32(v)
		hasOpts = true
	}
	if tags, ok := src["tags"].([]any); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				opts.Tags = append(opts.Tags, s)
			}
		}
		hasOpts = true
	}
	if v, ok := src["trace_id"].(string); ok {
		opts.TraceId = v
		hasOpts = true
	}

	// Handle scheduled_at / delay_until
	if v, ok := src["scheduled_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			opts.DelayUntil = timestamppb.New(t)
			hasOpts = true
		}
	}

	// Handle timeout (as seconds or duration string)
	if v, ok := src["timeout"].(float64); ok {
		opts.Timeout = durationpb.New(time.Duration(v) * time.Second)
		hasOpts = true
	}
	if v, ok := src["timeout"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			opts.Timeout = durationpb.New(d)
			hasOpts = true
		}
	}

	// Handle TTL (as seconds or duration string)
	if v, ok := src["ttl"].(float64); ok {
		opts.Ttl = durationpb.New(time.Duration(v) * time.Second)
		hasOpts = true
	}
	if v, ok := src["ttl"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			opts.Ttl = durationpb.New(d)
			hasOpts = true
		}
	}

	// Handle retry policy
	if retryMap, ok := src["retry"].(map[string]any); ok {
		retry := &ojsv1.RetryPolicy{}
		hasRetry := false
		if v, ok := retryMap["max_attempts"].(float64); ok {
			retry.MaxAttempts = int32(v)
			hasRetry = true
		}
		if v, ok := retryMap["initial_interval"].(float64); ok {
			retry.InitialInterval = durationpb.New(time.Duration(v) * time.Second)
			hasRetry = true
		}
		if v, ok := retryMap["initial_delay"].(float64); ok {
			retry.InitialInterval = durationpb.New(time.Duration(v) * time.Second)
			hasRetry = true
		}
		if v, ok := retryMap["initial_interval"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil {
				retry.InitialInterval = durationpb.New(d)
				hasRetry = true
			}
		}
		if v, ok := retryMap["max_interval"].(float64); ok {
			retry.MaxInterval = durationpb.New(time.Duration(v) * time.Second)
			hasRetry = true
		}
		if v, ok := retryMap["max_delay"].(float64); ok {
			retry.MaxInterval = durationpb.New(time.Duration(v) * time.Second)
			hasRetry = true
		}
		if v, ok := retryMap["multiplier"].(float64); ok {
			retry.BackoffCoefficient = v
			hasRetry = true
		}
		if v, ok := retryMap["backoff_coefficient"].(float64); ok {
			retry.BackoffCoefficient = v
			hasRetry = true
		}
		if hasRetry {
			opts.Retry = retry
			hasOpts = true
		}
	}

	// Handle unique policy
	if uniqueMap, ok := src["unique"].(map[string]any); ok {
		unique := &ojsv1.UniquePolicy{}
		hasUnique := false
		if v, ok := uniqueMap["key"].(string); ok {
			unique.Key = []string{v}
			hasUnique = true
		}
		if keys, ok := uniqueMap["key"].([]any); ok {
			for _, k := range keys {
				if s, ok := k.(string); ok {
					unique.Key = append(unique.Key, s)
				}
			}
			hasUnique = true
		}
		if v, ok := uniqueMap["period"].(float64); ok {
			unique.Period = durationpb.New(time.Duration(v) * time.Second)
			hasUnique = true
		}
		if v, ok := uniqueMap["period"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil {
				unique.Period = durationpb.New(d)
				hasUnique = true
			}
		}
		if v, ok := uniqueMap["on_conflict"].(string); ok {
			switch strings.ToLower(v) {
			case "reject":
				unique.OnConflict = ojsv1.UniqueConflictAction_UNIQUE_CONFLICT_ACTION_REJECT
			case "replace":
				unique.OnConflict = ojsv1.UniqueConflictAction_UNIQUE_CONFLICT_ACTION_REPLACE
			case "ignore":
				unique.OnConflict = ojsv1.UniqueConflictAction_UNIQUE_CONFLICT_ACTION_IGNORE
			case "replace_except_schedule":
				unique.OnConflict = ojsv1.UniqueConflictAction_UNIQUE_CONFLICT_ACTION_REPLACE_EXCEPT_SCHEDULE
			}
			hasUnique = true
		}
		if states, ok := uniqueMap["states"].([]any); ok {
			for _, s := range states {
				if ss, ok := s.(string); ok {
					stateVal, exists := ojsv1.JobState_value["JOB_STATE_"+strings.ToUpper(ss)]
					if exists {
						unique.States = append(unique.States, ojsv1.JobState(stateVal))
					}
				}
			}
			hasUnique = true
		}
		if hasUnique {
			opts.Unique = unique
			hasOpts = true
		}
	}

	// Handle meta (extensible metadata)
	if metaMap, ok := src["meta"].(map[string]any); ok {
		if s, err := structpb.NewStruct(metaMap); err == nil {
			opts.Meta = s
			hasOpts = true
		}
	}

	// If we used a nested "options" object, also check top-level body keys
	// as fallbacks (e.g., the batch request has default_options at top level)
	if hasNestedOptions {
		if v, ok := body["queue"].(string); ok && opts.Queue == "" {
			opts.Queue = v
			hasOpts = true
		}
		if v, ok := body["priority"].(float64); ok && opts.Priority == 0 {
			opts.Priority = int32(v)
			hasOpts = true
		}
		if v, ok := body["max_attempts"].(float64); ok && opts.MaxAttempts == 0 {
			opts.MaxAttempts = int32(v)
			hasOpts = true
		}
	}

	if !hasOpts {
		return nil
	}
	return opts
}

// protoJobToMap converts a protobuf Job message to a map for JSON serialization.
// The map uses the HTTP JSON field names that the test assertions expect.
func protoJobToMap(j *ojsv1.Job) map[string]any {
	if j == nil {
		return nil
	}
	m := map[string]any{
		"id":      j.Id,
		"type":    j.Type,
		"queue":   j.Queue,
		"state":   strings.ToLower(strings.TrimPrefix(j.State.String(), "JOB_STATE_")),
		"attempt": j.Attempt,
	}
	if j.Priority != 0 {
		m["priority"] = j.Priority
	}
	if j.MaxAttempts != 0 {
		m["max_attempts"] = j.MaxAttempts
	}
	if j.Specversion != "" {
		m["specversion"] = j.Specversion
	}
	if j.CreatedAt != nil {
		m["created_at"] = j.CreatedAt.AsTime().Format(time.RFC3339Nano)
	}
	if j.EnqueuedAt != nil {
		m["enqueued_at"] = j.EnqueuedAt.AsTime().Format(time.RFC3339Nano)
	}
	if j.ScheduledAt != nil {
		m["scheduled_at"] = j.ScheduledAt.AsTime().Format(time.RFC3339Nano)
	}
	if j.StartedAt != nil {
		m["started_at"] = j.StartedAt.AsTime().Format(time.RFC3339Nano)
	}
	if j.CompletedAt != nil {
		m["completed_at"] = j.CompletedAt.AsTime().Format(time.RFC3339Nano)
	}
	if j.ExpiresAt != nil {
		m["expires_at"] = j.ExpiresAt.AsTime().Format(time.RFC3339Nano)
	}
	if len(j.Args) > 0 {
		args := make([]any, 0, len(j.Args))
		for _, v := range j.Args {
			args = append(args, v.AsInterface())
		}
		m["args"] = args
	}
	if len(j.Errors) > 0 {
		errs := make([]map[string]any, 0, len(j.Errors))
		for _, e := range j.Errors {
			em := map[string]any{"message": e.Message}
			if e.Code != "" {
				em["type"] = e.Code
			}
			if e.Attempt != 0 {
				em["attempt"] = e.Attempt
			}
			errs = append(errs, em)
		}
		m["errors"] = errs
	}
	if len(j.Tags) > 0 {
		m["tags"] = j.Tags
	}
	if j.TraceId != "" {
		m["trace_id"] = j.TraceId
	}
	if j.WorkflowId != "" {
		m["workflow_id"] = j.WorkflowId
	}
	if j.Result != nil {
		m["result"] = j.Result.AsMap()
	}
	if j.Meta != nil {
		m["meta"] = j.Meta.AsMap()
	}
	if j.RetryPolicy != nil {
		retry := map[string]any{}
		if j.RetryPolicy.MaxAttempts != 0 {
			retry["max_attempts"] = j.RetryPolicy.MaxAttempts
		}
		if j.RetryPolicy.InitialInterval != nil {
			retry["initial_interval"] = j.RetryPolicy.InitialInterval.AsDuration().Seconds()
		}
		if j.RetryPolicy.MaxInterval != nil {
			retry["max_interval"] = j.RetryPolicy.MaxInterval.AsDuration().Seconds()
		}
		if j.RetryPolicy.BackoffCoefficient != 0 {
			retry["backoff_coefficient"] = j.RetryPolicy.BackoffCoefficient
		}
		if len(retry) > 0 {
			m["retry_policy"] = retry
		}
	}
	if j.UniquePolicy != nil {
		unique := map[string]any{}
		if len(j.UniquePolicy.Key) > 0 {
			unique["key"] = j.UniquePolicy.Key
		}
		if j.UniquePolicy.Period != nil {
			unique["period"] = j.UniquePolicy.Period.AsDuration().Seconds()
		}
		if j.UniquePolicy.OnConflict != ojsv1.UniqueConflictAction_UNIQUE_CONFLICT_ACTION_UNSPECIFIED {
			unique["on_conflict"] = strings.ToLower(strings.TrimPrefix(j.UniquePolicy.OnConflict.String(), "UNIQUE_CONFLICT_ACTION_"))
		}
		if len(unique) > 0 {
			m["unique_policy"] = unique
		}
	}
	return m
}

// protoWorkflowToMap converts a protobuf Workflow to a map for JSON serialization.
func protoWorkflowToMap(w *ojsv1.Workflow) map[string]any {
	if w == nil {
		return nil
	}
	m := map[string]any{
		"id":    w.Id,
		"name":  w.Name,
		"state": strings.ToLower(strings.TrimPrefix(w.State.String(), "WORKFLOW_STATE_")),
	}
	if w.CreatedAt != nil {
		m["created_at"] = w.CreatedAt.AsTime().Format(time.RFC3339Nano)
	}
	if w.CompletedAt != nil {
		m["completed_at"] = w.CompletedAt.AsTime().Format(time.RFC3339Nano)
	}
	if len(w.Steps) > 0 {
		steps := make([]map[string]any, 0, len(w.Steps))
		for _, s := range w.Steps {
			sm := map[string]any{
				"id":    s.Id,
				"type":  s.Type,
				"state": strings.ToLower(strings.TrimPrefix(s.State.String(), "WORKFLOW_STEP_STATE_")),
			}
			if s.JobId != "" {
				sm["job_id"] = s.JobId
			}
			if len(s.DependsOn) > 0 {
				sm["depends_on"] = s.DependsOn
			}
			steps = append(steps, sm)
		}
		m["steps"] = steps
	}
	return m
}
