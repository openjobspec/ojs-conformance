// serve.go — interactive DAP server for exec-history traces. Spawned by
// an IDE (VS Code, JetBrains, nvim-dap) over stdio. Supports a working
// subset of the protocol: initialize, launch, configurationDone,
// threads, stackTrace, scopes, variables, continue, next, disconnect.
//
// Step semantics:
//
//   - At launch time the trace is loaded into memory and validated.
//   - The first span is the "current" span; configurationDone emits a
//     stopped event for it.
//   - continue (and next, treated as alias) advances to the next span,
//     emitting another stopped event. When spans run out we send
//     terminated.
//
// Scope/Variables semantics:
//
//   - One scope per stopped span called "Span". Its variablesReference
//     points at a freshly populated map containing the entire span as
//     key/value pairs (per spanAsMap).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/openjobspec/ojs-conformance/lib"
)

func serveCmd(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	tracePath := fs.String("trace", "", "path to exec-history JSONL trace (- for stdin)")
	threadBy := fs.String("thread-by", "envelope", "DAP thread grouping (envelope|sdk)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tracePath == "" {
		return errors.New("--trace is required")
	}
	switch *threadBy {
	case "envelope", "sdk":
	default:
		return fmt.Errorf("--thread-by must be envelope|sdk, got %q", *threadBy)
	}

	srv := newServer(os.Stdin, os.Stdout, *tracePath, *threadBy)
	return srv.Run(context.Background())
}

// server is the DAP request loop. It is single-goroutine; DAP allows
// concurrent requests in theory but for a stdio adapter sequential
// processing is correct and simpler.
type server struct {
	in       io.Reader
	out      *bufio.Writer
	outMu    sync.Mutex
	seq      int64

	tracePath string
	threadBy  string

	// Loaded after launch.
	spans     []lib.ExecHistoryEvent
	cursor    int // index of the *current* span (already stopped on)
	threadIDs map[string]int
	nextTID   int
	varRefs   map[int]map[string]any
	nextVar   int
	stopped   bool
	terminated bool
}

func newServer(in io.Reader, out io.Writer, tracePath, threadBy string) *server {
	return &server{
		in:        in,
		out:       bufio.NewWriter(out),
		tracePath: tracePath,
		threadBy:  threadBy,
		threadIDs: map[string]int{},
		nextTID:   1,
		varRefs:   map[int]map[string]any{},
		nextVar:   1000,
		cursor:    -1,
	}
}

// Run reads DAP messages until disconnect or EOF.
func (s *server) Run(_ context.Context) error {
	br := bufio.NewReader(s.in)
	for {
		msg, err := readDAPMessage(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if err := s.handle(msg); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if s.terminated {
			// Flush any pending events then keep the loop alive so
			// disconnect can land. Real IDEs usually disconnect right
			// after seeing terminated.
			_ = s.out.Flush()
		}
	}
}

func (s *server) handle(msg map[string]any) error {
	t, _ := msg["type"].(string)
	if t != "request" {
		// We don't expect events from the client.
		return nil
	}
	cmd, _ := msg["command"].(string)
	reqSeq, _ := msg["seq"].(float64)
	switch cmd {
	case "initialize":
		if err := s.respond(int64(reqSeq), cmd, true, "", map[string]any{
			"supportsConfigurationDoneRequest": true,
			"supportsTerminateRequest":         true,
			"supportsStepBack":                 false,
		}); err != nil {
			return err
		}
		return s.event("initialized", nil)
	case "launch":
		if err := s.loadTrace(); err != nil {
			return s.respond(int64(reqSeq), cmd, false, err.Error(), nil)
		}
		if err := s.respond(int64(reqSeq), cmd, true, "", nil); err != nil {
			return err
		}
		return s.event("process", map[string]any{
			"name":            "ojs-exec-history",
			"systemProcessId": os.Getpid(),
			"isLocalProcess":  true,
			"startMethod":     "launch",
		})
	case "configurationDone":
		if err := s.respond(int64(reqSeq), cmd, true, "", nil); err != nil {
			return err
		}
		return s.advanceAndStop()
	case "threads":
		threads := make([]map[string]any, 0)
		// Threads we already know about (same id => same name).
		// To keep things deterministic we sort by id.
		ids := make([]int, 0, len(s.threadIDs))
		nameByID := map[int]string{}
		for k, v := range s.threadIDs {
			ids = append(ids, v)
			nameByID[v] = k
		}
		sort.Ints(ids)
		for _, id := range ids {
			threads = append(threads, map[string]any{
				"id":   id,
				"name": nameByID[id],
			})
		}
		return s.respond(int64(reqSeq), cmd, true, "", map[string]any{"threads": threads})
	case "stackTrace":
		span, ok := s.currentSpan()
		if !ok {
			return s.respond(int64(reqSeq), cmd, true, "", map[string]any{"stackFrames": []any{}, "totalFrames": 0})
		}
		frame := map[string]any{
			"id":     1,
			"name":   span.SpanKind + " (" + span.Outcome + ")",
			"line":   1,
			"column": 1,
			"source": map[string]any{
				"name": span.SDK,
				"path": s.tracePath,
			},
		}
		return s.respond(int64(reqSeq), cmd, true, "", map[string]any{
			"stackFrames": []any{frame},
			"totalFrames": 1,
		})
	case "scopes":
		span, ok := s.currentSpan()
		if !ok {
			return s.respond(int64(reqSeq), cmd, true, "", map[string]any{"scopes": []any{}})
		}
		ref := s.allocVar(spanAsMap(&span))
		scope := map[string]any{
			"name":               "Span",
			"variablesReference": ref,
			"expensive":          false,
		}
		return s.respond(int64(reqSeq), cmd, true, "", map[string]any{"scopes": []any{scope}})
	case "variables":
		args, _ := msg["arguments"].(map[string]any)
		refF, _ := args["variablesReference"].(float64)
		ref := int(refF)
		m, ok := s.varRefs[ref]
		if !ok {
			return s.respond(int64(reqSeq), cmd, false, fmt.Sprintf("unknown variablesReference %d", ref), nil)
		}
		vars := varsFromMap(m, &s.nextVar, s.varRefs)
		return s.respond(int64(reqSeq), cmd, true, "", map[string]any{"variables": vars})
	case "continue", "next":
		// next is treated as alias for continue (single-step granularity).
		if err := s.respond(int64(reqSeq), cmd, true, "", map[string]any{"allThreadsContinued": false}); err != nil {
			return err
		}
		return s.advanceAndStop()
	case "disconnect", "terminate":
		_ = s.respond(int64(reqSeq), cmd, true, "", nil)
		if !s.terminated {
			_ = s.event("terminated", map[string]any{})
			s.terminated = true
		}
		return io.EOF
	default:
		return s.respond(int64(reqSeq), cmd, false, "unsupported command "+cmd, nil)
	}
}

// loadTrace reads + validates the JSONL trace into memory.
func (s *server) loadTrace() error {
	var f io.ReadCloser
	if s.tracePath == "-" {
		f = io.NopCloser(os.Stdin)
	} else {
		ff, err := os.Open(s.tracePath)
		if err != nil {
			return err
		}
		f = ff
	}
	defer f.Close()
	_, _, err := lib.Replay(f, true, func(e *lib.ExecHistoryEvent) error {
		s.spans = append(s.spans, *e)
		return nil
	})
	return err
}

func (s *server) currentSpan() (lib.ExecHistoryEvent, bool) {
	if !s.stopped || s.cursor < 0 || s.cursor >= len(s.spans) {
		return lib.ExecHistoryEvent{}, false
	}
	return s.spans[s.cursor], true
}

// advanceAndStop bumps the cursor to the next span and emits a stopped
// event. If we run out of spans, emits terminated.
func (s *server) advanceAndStop() error {
	s.cursor++
	if s.cursor >= len(s.spans) {
		s.stopped = false
		s.terminated = true
		return s.event("terminated", map[string]any{})
	}
	span := s.spans[s.cursor]
	threadKey := span.EnvelopeID
	if s.threadBy == "sdk" {
		threadKey = span.SDK
	}
	tid, fresh := s.threadIDFor(threadKey)
	if fresh {
		if err := s.event("thread", map[string]any{
			"reason":   "started",
			"threadId": tid,
		}); err != nil {
			return err
		}
	}
	s.stopped = true
	return s.event("stopped", map[string]any{
		"reason":            spanStopReason(span.Outcome),
		"description":       span.SpanKind + " (" + span.Outcome + ")",
		"threadId":          tid,
		"allThreadsStopped": false,
	})
}

func (s *server) threadIDFor(key string) (int, bool) {
	if id, ok := s.threadIDs[key]; ok {
		return id, false
	}
	id := s.nextTID
	s.nextTID++
	s.threadIDs[key] = id
	return id, true
}

func (s *server) allocVar(v map[string]any) int {
	id := s.nextVar
	s.nextVar++
	s.varRefs[id] = v
	return id
}

// varsFromMap converts a map[string]any into DAP "variables" entries.
// Nested objects/arrays get a fresh variablesReference so a client can
// expand them with another /variables request. This lets the IDE Tree
// view drill into envelope.attrs without us flattening everything.
func varsFromMap(m map[string]any, nextVar *int, refs map[int]map[string]any) []map[string]any {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		v := m[k]
		entry := map[string]any{
			"name":               k,
			"value":              valueRepr(v),
			"variablesReference": 0,
		}
		if sub, ok := v.(map[string]any); ok {
			ref := *nextVar
			*nextVar++
			refs[ref] = sub
			entry["variablesReference"] = ref
			entry["value"] = "{...}"
		}
		out = append(out, entry)
	}
	return out
}

func valueRepr(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return t
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func (s *server) event(name string, body any) error {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	s.seq++
	msg := map[string]any{"seq": s.seq, "type": "event", "event": name}
	if body != nil {
		msg["body"] = body
	}
	return writeDAPMessage(s.out, msg)
}

func (s *server) respond(reqSeq int64, cmd string, success bool, message string, body any) error {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	s.seq++
	msg := map[string]any{
		"seq":         s.seq,
		"type":        "response",
		"request_seq": reqSeq,
		"command":     cmd,
		"success":     success,
	}
	if message != "" {
		msg["message"] = message
	}
	if body != nil {
		msg["body"] = body
	}
	return writeDAPMessage(s.out, msg)
}

// readDAPMessage reads one DAP-framed message.
func readDAPMessage(br *bufio.Reader) (map[string]any, error) {
	contentLen := -1
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && line == "" && contentLen < 0 {
				return nil, io.EOF
			}
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("dap read: unexpected EOF mid-header")
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if v, ok := strings.CutPrefix(line, "Content-Length:"); ok {
			_, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &contentLen)
			if err != nil {
				return nil, fmt.Errorf("dap read: bad content-length %q: %w", v, err)
			}
		}
	}
	if contentLen <= 0 {
		return nil, fmt.Errorf("dap read: missing content-length")
	}
	buf := make([]byte, contentLen)
	if _, err := io.ReadFull(br, buf); err != nil {
		return nil, fmt.Errorf("dap read body: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf, &m); err != nil {
		return nil, fmt.Errorf("dap unmarshal: %w", err)
	}
	return m, nil
}

func writeDAPMessage(w *bufio.Writer, msg map[string]any) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	if _, err := w.Write(payload); err != nil {
		return err
	}
	return w.Flush()
}
