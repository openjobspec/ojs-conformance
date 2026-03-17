package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/openjobspec/ojs-conformance/lib"
)

func TestSpanStopReason(t *testing.T) {
	cases := map[string]string{
		"success":   "step",
		"in_flight": "step",
		"retryable": "exception",
		"discarded": "exception",
		"cancelled": "exception",
	}
	for in, want := range cases {
		if got := spanStopReason(in); got != want {
			t.Errorf("spanStopReason(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSpanAsMapAttrsSorted(t *testing.T) {
	ev := &lib.ExecHistoryEvent{
		SchemaVersion: lib.CurrentSchemaVersion,
		EnvelopeID:    "env-1",
		SDK:           "ojs-go-sdk",
		Lang:          "go",
		SpanKind:      "job.attempt",
		StartedAt:     "2026-04-17T12:00:00Z",
		Outcome:       "success",
		Attempt:       1,
		SpanID:        "sp-1",
		Attrs: map[string]any{
			"z": 1, "a": 2, "m": 3,
		},
	}
	m := spanAsMap(ev)
	attrs, ok := m["attrs"].(map[string]any)
	if !ok {
		t.Fatalf("attrs missing or wrong type: %T", m["attrs"])
	}
	if len(attrs) != 3 {
		t.Fatalf("expected 3 attrs, got %d", len(attrs))
	}
	if _, has := m["worker_id"]; has {
		t.Error("worker_id should be omitted when empty")
	}
}

func TestEmitterRoundTrip(t *testing.T) {
	trace := strings.Join([]string{
		`{"schema_version":"ojs-exec-history/0.1","envelope_id":"env-A","sdk":"ojs-go-sdk","lang":"go","span_kind":"queue.dequeue","started_at":"2026-04-17T12:00:00.000000001Z","duration_ms":1.2,"outcome":"success","attempt":1,"span_id":"sp-1"}`,
		`{"schema_version":"ojs-exec-history/0.1","envelope_id":"env-A","sdk":"ojs-go-sdk","lang":"go","span_kind":"job.attempt","started_at":"2026-04-17T12:00:00.001500000Z","duration_ms":250.0,"outcome":"retryable","attempt":1,"span_id":"sp-2","parent_span_id":"sp-1","attrs":{"queue":"emails"}}`,
		`{"schema_version":"ojs-exec-history/0.1","envelope_id":"env-B","sdk":"ojs-py-sdk","lang":"python","span_kind":"job.attempt","started_at":"2026-04-17T12:00:01.000000000Z","duration_ms":80.0,"outcome":"success","attempt":1,"span_id":"sp-3"}`,
		``, // tolerate blank line
	}, "\n")

	var buf bytes.Buffer
	em := newEmitter(bufWriter(&buf), "envelope")
	if err := em.initialize(); err != nil {
		t.Fatal(err)
	}
	ingested, dropped, err := lib.Replay(strings.NewReader(trace), true, em.onSpan)
	if err != nil {
		t.Fatal(err)
	}
	if ingested != 3 || dropped != 0 {
		t.Fatalf("ingested=%d dropped=%d, want 3/0", ingested, dropped)
	}
	if err := em.terminate(ingested, dropped); err != nil {
		t.Fatal(err)
	}

	msgs, err := ReadDAPMessages(&buf)
	if err != nil {
		t.Fatalf("read DAP: %v", err)
	}

	// Per-span: 1 thread (first time only) + 1 output + 1 stopped + 1 continued
	// Globals: initialized + process at start, output + terminated at end.
	// env-A produces 1 thread (first), then sp-2 reuses; env-B produces 1 thread.
	// = 2 (initialize) + (4 + 3 + 4) (spans) + 2 (terminate) = 15
	if len(msgs) != 15 {
		t.Fatalf("expected 15 DAP messages, got %d", len(msgs))
	}

	if msgs[0]["event"] != "initialized" {
		t.Errorf("first event = %v, want initialized", msgs[0]["event"])
	}
	if msgs[1]["event"] != "process" {
		t.Errorf("second event = %v, want process", msgs[1]["event"])
	}
	if msgs[len(msgs)-1]["event"] != "terminated" {
		t.Errorf("last event = %v, want terminated", msgs[len(msgs)-1]["event"])
	}

	// Find the stopped events and check the second one is "exception"
	// (sp-2 is retryable) and the third is "step" (sp-3 success).
	var stops []map[string]any
	for _, m := range msgs {
		if m["event"] == "stopped" {
			body, _ := m["body"].(map[string]any)
			stops = append(stops, body)
		}
	}
	if len(stops) != 3 {
		t.Fatalf("expected 3 stopped events, got %d", len(stops))
	}
	if stops[0]["reason"] != "step" {
		t.Errorf("stop[0] reason = %v, want step", stops[0]["reason"])
	}
	if stops[1]["reason"] != "exception" {
		t.Errorf("stop[1] reason = %v, want exception", stops[1]["reason"])
	}
	if stops[2]["reason"] != "step" {
		t.Errorf("stop[2] reason = %v, want step", stops[2]["reason"])
	}

	// envelope-keyed thread ids: env-A = 1, env-B = 2; sp-2 reuses 1.
	if int(stops[0]["threadId"].(float64)) != 1 {
		t.Errorf("stop[0] threadId = %v, want 1", stops[0]["threadId"])
	}
	if int(stops[1]["threadId"].(float64)) != 1 {
		t.Errorf("stop[1] threadId = %v, want 1 (same envelope)", stops[1]["threadId"])
	}
	if int(stops[2]["threadId"].(float64)) != 2 {
		t.Errorf("stop[2] threadId = %v, want 2 (new envelope)", stops[2]["threadId"])
	}
}

func TestEmitterThreadBySDK(t *testing.T) {
	trace := strings.Join([]string{
		`{"schema_version":"ojs-exec-history/0.1","envelope_id":"env-A","sdk":"ojs-go-sdk","lang":"go","span_kind":"job.attempt","started_at":"2026-04-17T12:00:00Z","duration_ms":1,"outcome":"success","attempt":1,"span_id":"sp-1"}`,
		`{"schema_version":"ojs-exec-history/0.1","envelope_id":"env-B","sdk":"ojs-go-sdk","lang":"go","span_kind":"job.attempt","started_at":"2026-04-17T12:00:00Z","duration_ms":1,"outcome":"success","attempt":1,"span_id":"sp-2"}`,
	}, "\n")
	var buf bytes.Buffer
	em := newEmitter(bufWriter(&buf), "sdk")
	if err := em.initialize(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := lib.Replay(strings.NewReader(trace), true, em.onSpan); err != nil {
		t.Fatal(err)
	}
	msgs, err := ReadDAPMessages(&buf)
	if err != nil {
		t.Fatal(err)
	}
	threads := 0
	for _, m := range msgs {
		if m["event"] == "thread" {
			threads++
		}
	}
	if threads != 1 {
		t.Errorf("thread-by=sdk: expected 1 thread event, got %d", threads)
	}
}

func TestReadDAPMessagesBadHeader(t *testing.T) {
	_, err := ReadDAPMessages(strings.NewReader("Garbage\r\n\r\n"))
	if err == nil {
		t.Fatal("expected error on missing content-length")
	}
}

func TestEmitterDropsInvalidNonStrict(t *testing.T) {
	trace := strings.Join([]string{
		`{"schema_version":"ojs-exec-history/0.1","envelope_id":"env-A","sdk":"ojs-go-sdk","lang":"go","span_kind":"job.attempt","started_at":"2026-04-17T12:00:00Z","duration_ms":1,"outcome":"success","attempt":1,"span_id":"sp-1"}`,
		`{"bogus":true}`,
		`{"schema_version":"ojs-exec-history/0.1","envelope_id":"env-A","sdk":"ojs-go-sdk","lang":"go","span_kind":"job.attempt","started_at":"2026-04-17T12:00:01Z","duration_ms":1,"outcome":"success","attempt":1,"span_id":"sp-2"}`,
	}, "\n")
	var buf bytes.Buffer
	em := newEmitter(bufWriter(&buf), "envelope")
	_ = em.initialize()
	ing, drop, err := lib.Replay(strings.NewReader(trace), false, em.onSpan)
	if err != nil {
		t.Fatal(err)
	}
	if ing != 2 || drop != 1 {
		t.Fatalf("ingested=%d dropped=%d, want 2/1", ing, drop)
	}
}
