package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	ojsv1 "github.com/openjobspec/ojs-proto/gen/go/ojs/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// OJSClient wraps the generated gRPC client with convenience methods.
type OJSClient struct {
	conn   *grpc.ClientConn
	client ojsv1.OJSServiceClient
}

// NewOJSClient connects to the gRPC server and returns a client wrapper.
func NewOJSClient(ctx context.Context, addr string) (*OJSClient, error) {
	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
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
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
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
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) getJob(ctx context.Context, jobID string) (*RPCResult, error) {
	resp, err := c.client.GetJob(ctx, &ojsv1.GetJobRequest{JobId: jobID})
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(protoJobToMap(resp.Job))
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
	respJSON, _ := json.Marshal(protoJobToMap(resp.Job))
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
	if v, ok := body["worker_id"].(string); ok {
		req.WorkerId = v
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
	resp, err := c.client.RegisterCron(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	result := map[string]any{"name": resp.Name}
	if resp.NextRunAt != nil {
		result["next_run_at"] = resp.NextRunAt.AsTime().Format(time.RFC3339Nano)
	}
	respJSON, _ := json.Marshal(result)
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
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
				req.Steps = append(req.Steps, step)
			}
		}
	}

	resp, err := c.client.CreateWorkflow(ctx, req)
	if err != nil {
		return grpcError(err), nil
	}
	respJSON, _ := json.Marshal(map[string]any{"workflow": protoWorkflowToMap(resp.Workflow)})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
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
	respJSON, _ := json.Marshal(map[string]any{"status": statusStr})
	return &RPCResult{ResponseJSON: respJSON, GRPCCode: codes.OK}, nil
}

func (c *OJSClient) manifest(ctx context.Context) (*RPCResult, error) {
	resp, err := c.client.Manifest(ctx, &ojsv1.ManifestRequest{})
	if err != nil {
		return grpcError(err), nil
	}
	result := map[string]any{
		"ojs_version":       resp.OjsVersion,
		"conformance_level": resp.ConformanceLevel,
		"protocols":         resp.Protocols,
		"backend":           resp.Backend,
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
	default:
		return 500
	}
}

// resolveMethod determines the gRPC method name from an HTTP action and path.
func resolveMethod(action, path string) string {
	// Exact matches first
	key := action + " " + path
	if m, ok := actionToRPC[key]; ok {
		return m
	}
	// Prefix matching for paths with IDs
	for pattern, method := range actionToRPC {
		parts := strings.SplitN(pattern, " ", 2)
		if len(parts) == 2 && parts[0] == action && strings.HasPrefix(path, parts[1]) {
			return method
		}
	}
	return ""
}

// actionToRPC maps HTTP action+path patterns to gRPC method names.
var actionToRPC = map[string]string{
	"POST /ojs/v1/jobs":              "Enqueue",
	"POST /ojs/v1/jobs/batch":        "EnqueueBatch",
	"GET /ojs/v1/jobs/":              "GetJob",
	"DELETE /ojs/v1/jobs/":           "CancelJob",
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
func buildEnqueueOptions(body map[string]any) *ojsv1.EnqueueOptions {
	opts := &ojsv1.EnqueueOptions{}
	hasOpts := false

	if v, ok := body["queue"].(string); ok {
		opts.Queue = v
		hasOpts = true
	}
	if v, ok := body["priority"].(float64); ok {
		opts.Priority = int32(v)
		hasOpts = true
	}
	if v, ok := body["max_attempts"].(float64); ok {
		opts.MaxAttempts = int32(v)
		hasOpts = true
	}
	if tags, ok := body["tags"].([]any); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				opts.Tags = append(opts.Tags, s)
			}
		}
		hasOpts = true
	}

	if !hasOpts {
		return nil
	}
	return opts
}

// protoJobToMap converts a protobuf Job message to a map for JSON serialization.
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
