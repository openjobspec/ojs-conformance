// Command exec-history-dap is the M6/P0 sliver of the Replay Studio: it
// reads an OJS exec-history JSONL trace (spec/ojs-exec-history-0.1.md) and
// emits a Debug Adapter Protocol (DAP) event stream over stdout that an
// IDE-side DAP client can ingest as if it were a paused debugger session.
//
// Why this is the right P0 shape: every modern IDE (VS Code, JetBrains,
// neovim/nvim-dap, Eclipse) speaks DAP. If our exec-history can be
// projected into DAP, we get a working "step through a job's life" UX
// for free in any of those editors with zero per-IDE adapter code.
//
// What this binary does today (P0 spike):
//
//   - `emit` subcommand: stream-out one DAP "stopped" event per span,
//     interleaved with the surrounding Output/Thread events a DAP client
//     expects. This is one-way, no client interaction. It exists to
//     prove the projection from exec-history -> DAP is well-defined.
//
// What it does NOT do yet (P1+):
//
//   - Interactive request/response loop (initialize, launch, threads,
//     stackTrace, scopes, variables, continue, next, disconnect). The
//     skeleton is wired so it slots in without a rewrite.
//   - Multi-trace correlation, time-travel, breakpoint matching.
//   - VS Code extension packaging (M6/P1).
//
// DAP wire format reference:
//
//	https://microsoft.github.io/debug-adapter-protocol/specification
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/openjobspec/ojs-conformance/lib"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "emit":
		if err := emitCmd(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "exec-history-dap emit:", err)
			os.Exit(1)
		}
	case "serve":
		if err := serveCmd(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "exec-history-dap serve:", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println("exec-history-dap 0.1.0-p1")
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: exec-history-dap <emit|serve|version> [flags]")
	fmt.Fprintln(os.Stderr, "  emit  -trace path.jsonl [-strict] [-thread-by sdk|envelope]")
	fmt.Fprintln(os.Stderr, "  serve -trace path.jsonl [-thread-by sdk|envelope]")
	fmt.Fprintln(os.Stderr, "        Run as a DAP server reading requests from stdin.")
}

func emitCmd(args []string) error {
	fs := flag.NewFlagSet("emit", flag.ContinueOnError)
	tracePath := fs.String("trace", "", "path to exec-history JSONL trace (- for stdin)")
	strict := fs.Bool("strict", false, "stop on first invalid line")
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

	var r io.ReadCloser
	if *tracePath == "-" {
		r = io.NopCloser(os.Stdin)
	} else {
		f, err := os.Open(*tracePath)
		if err != nil {
			return err
		}
		r = f
	}
	defer r.Close()

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	em := newEmitter(w, *threadBy)
	if err := em.initialize(); err != nil {
		return err
	}

	ingested, dropped, err := lib.Replay(r, *strict, em.onSpan)
	if err != nil {
		return err
	}
	if err := em.terminate(ingested, dropped); err != nil {
		return err
	}
	return nil
}

// emitter projects ExecHistoryEvent -> DAP messages.
type emitter struct {
	w        *bufio.Writer
	seq      atomic.Int64
	threadBy string

	// Stable thread-id allocation. DAP threads are int IDs; we map
	// (envelope_id|sdk) -> int deterministically per process.
	threadIDs map[string]int
	nextTID   int

	// Variable references. DAP requires that any "scopes"/"variables"
	// payload be retrievable later by reference; we hand out IDs even
	// for the emit-only mode so a P1 interactive client can replay.
	varRefs map[int]map[string]any
	nextVar int
}

func newEmitter(w *bufio.Writer, threadBy string) *emitter {
	return &emitter{
		w:         w,
		threadBy:  threadBy,
		threadIDs: map[string]int{},
		nextTID:   1,
		varRefs:   map[int]map[string]any{},
		nextVar:   1000, // DAP convention: > 0 means structured value
	}
}

// initialize sends the bare minimum DAP envelope a client expects before
// stop events arrive: an "initialized" event, then a synthetic "process"
// event so the client has something to label its UI with.
func (e *emitter) initialize() error {
	if err := e.writeEvent("initialized", nil); err != nil {
		return err
	}
	return e.writeEvent("process", map[string]any{
		"name":            "ojs-exec-history",
		"isLocalProcess":  true,
		"startMethod":     "launch",
		"systemProcessId": os.Getpid(),
	})
}

func (e *emitter) terminate(ingested, dropped int) error {
	if err := e.writeEvent("output", map[string]any{
		"category": "console",
		"output": fmt.Sprintf("exec-history-dap: replayed %d span(s), dropped %d invalid line(s)\n",
			ingested, dropped),
	}); err != nil {
		return err
	}
	return e.writeEvent("terminated", map[string]any{})
}

// onSpan is the lib.Replay callback. Per validated exec-history span we
// emit a 4-event burst: thread (if first time we see this thread),
// output (so logs show up in the IDE console), stopped (so the IDE
// thinks the debugger paused on this span), and a continued event so
// the next span isn't queued behind a phantom "still paused" state.
func (e *emitter) onSpan(ev *lib.ExecHistoryEvent) error {
	threadKey := ev.EnvelopeID
	if e.threadBy == "sdk" {
		threadKey = ev.SDK
	}
	tid, fresh := e.threadIDFor(threadKey)
	if fresh {
		if err := e.writeEvent("thread", map[string]any{
			"reason":   "started",
			"threadId": tid,
		}); err != nil {
			return err
		}
	}
	// Console line — IDE shows this in Debug Console.
	line := fmt.Sprintf("[%s] %s/%s span_kind=%s outcome=%s duration=%.3fms attempt=%d\n",
		ev.StartedAt, ev.SDK, ev.Lang, ev.SpanKind, ev.Outcome, ev.DurationMs, ev.Attempt)
	if err := e.writeEvent("output", map[string]any{
		"category": "stdout",
		"output":   line,
	}); err != nil {
		return err
	}

	// Allocate a variables ref for this span so a P1 client can fetch
	// the underlying attrs map by id when it gets to interactive mode.
	scopeRef := e.allocVar(spanAsMap(ev))

	// Stopped event — the core of "step into a span".
	if err := e.writeEvent("stopped", map[string]any{
		"reason":            spanStopReason(ev.Outcome),
		"description":       ev.SpanKind + " (" + ev.Outcome + ")",
		"threadId":          tid,
		"preserveFocusHint": false,
		"allThreadsStopped": false,
		"hitBreakpointIds":  []int{},
		// non-standard, but DAP allows arbitrary fields; downstream
		// clients can pull this for their UI without an extra request.
		"_ojs": map[string]any{
			"span_id":         ev.SpanID,
			"parent_span_id":  ev.ParentSpanID,
			"envelope_id":     ev.EnvelopeID,
			"variables_ref":   scopeRef,
			"sdk":             ev.SDK,
			"lang":            ev.Lang,
		},
	}); err != nil {
		return err
	}

	// Continued — without this the IDE will stay "paused" forever.
	return e.writeEvent("continued", map[string]any{
		"threadId":            tid,
		"allThreadsContinued": false,
	})
}

// spanAsMap flattens an ExecHistoryEvent into the map shape DAP
// "variables" responses expect. Each top-level key is a DAP variable.
func spanAsMap(ev *lib.ExecHistoryEvent) map[string]any {
	m := map[string]any{
		"schema_version": ev.SchemaVersion,
		"envelope_id":    ev.EnvelopeID,
		"sdk":            ev.SDK,
		"lang":           ev.Lang,
		"span_kind":      ev.SpanKind,
		"started_at":     ev.StartedAt,
		"duration_ms":    ev.DurationMs,
		"outcome":        ev.Outcome,
		"attempt":        ev.Attempt,
		"span_id":        ev.SpanID,
	}
	if ev.WorkerID != "" {
		m["worker_id"] = ev.WorkerID
	}
	if ev.ParentSpanID != "" {
		m["parent_span_id"] = ev.ParentSpanID
	}
	if len(ev.Attrs) > 0 {
		// Sorted to keep golden-file tests stable.
		keys := make([]string, 0, len(ev.Attrs))
		for k := range ev.Attrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		attrs := make(map[string]any, len(keys))
		for _, k := range keys {
			attrs[k] = ev.Attrs[k]
		}
		m["attrs"] = attrs
	}
	return m
}

// spanStopReason maps an exec-history outcome onto a DAP stop reason.
// DAP defines: step, breakpoint, exception, pause, entry, goto,
// function breakpoint, data breakpoint, instruction breakpoint. We use
// "exception" for anything that didn't succeed, "step" otherwise.
func spanStopReason(outcome string) string {
	switch outcome {
	case "retryable", "discarded", "cancelled":
		return "exception"
	default:
		return "step"
	}
}

func (e *emitter) threadIDFor(key string) (int, bool) {
	if id, ok := e.threadIDs[key]; ok {
		return id, false
	}
	id := e.nextTID
	e.nextTID++
	e.threadIDs[key] = id
	return id, true
}

func (e *emitter) allocVar(v map[string]any) int {
	id := e.nextVar
	e.nextVar++
	e.varRefs[id] = v
	return id
}

// writeEvent serializes one DAP event message and writes it with the
// "Content-Length: N\r\n\r\n" framing required by the protocol.
func (e *emitter) writeEvent(name string, body any) error {
	msg := map[string]any{
		"seq":   e.seq.Add(1),
		"type":  "event",
		"event": name,
	}
	if body != nil {
		msg["body"] = body
	}
	return e.writeMessage(msg)
}

func (e *emitter) writeMessage(msg map[string]any) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("dap marshal: %w", err)
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))
	if _, err := e.w.WriteString(header); err != nil {
		return err
	}
	if _, err := e.w.Write(payload); err != nil {
		return err
	}
	// Flush periodically so an attached IDE sees events live; flushing
	// on every message is the safe default (DAP throughput is small).
	return e.w.Flush()
}

// ReadDAPMessages parses a stream of DAP-framed messages. Used by tests
// and (eventually) by the interactive P1 mode. Returns when EOF or a
// short read is encountered. A timeout is the caller's job.
func ReadDAPMessages(r io.Reader) ([]map[string]any, error) {
	br := bufio.NewReader(r)
	var out []map[string]any
	for {
		// Read headers up to blank line.
		var contentLen int
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				if errors.Is(err, io.EOF) && line == "" {
					return out, nil
				}
				return out, err
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if v, ok := strings.CutPrefix(line, "Content-Length:"); ok {
				_, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &contentLen)
				if err != nil {
					return out, fmt.Errorf("dap read: bad content-length %q: %w", v, err)
				}
			}
		}
		if contentLen <= 0 {
			return out, fmt.Errorf("dap read: missing or zero content-length")
		}
		buf := make([]byte, contentLen)
		if _, err := io.ReadFull(br, buf); err != nil {
			return out, fmt.Errorf("dap read body: %w", err)
		}
		var m map[string]any
		if err := json.Unmarshal(buf, &m); err != nil {
			return out, fmt.Errorf("dap unmarshal: %w", err)
		}
		out = append(out, m)
	}
}

// readinessSentinel exists so callers (tests) have something to wait on;
// keeping it as a small atomic is enough for the P0 spike.
var readinessSentinel atomic.Bool

func init() {
	readinessSentinel.Store(true)
	_ = time.Now // future: latency budgeting
}
