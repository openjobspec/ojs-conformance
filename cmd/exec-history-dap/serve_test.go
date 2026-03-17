package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func writeReq(t *testing.T, w io.Writer, seq int, command string, args map[string]any) {
	t.Helper()
	body := map[string]any{
		"seq":     seq,
		"type":    "request",
		"command": command,
	}
	if args != nil {
		body["arguments"] = args
	}
	b, _ := json.Marshal(body)
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(b); err != nil {
		t.Fatal(err)
	}
}

// drainAll reads all DAP messages from r until EOF and returns them.
func drainAll(t *testing.T, r io.Reader) []map[string]any {
	t.Helper()
	br := bufio.NewReader(r)
	var out []map[string]any
	for {
		m, err := readDAPMessage(br)
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		out = append(out, m)
	}
}

// trace fixture with 2 spans: one success, one retryable.
const fixture = `{"schema_version":"ojs-exec-history/0.1","started_at":"2025-01-01T00:00:00Z","envelope_id":"01HXXX000000000000000000A1","span_id":"s1","span_kind":"job.attempt","outcome":"success","duration_ms":12,"sdk":"ojs-go-sdk","sdk_version":"0.3.0","lang":"go","attempt":1}
{"schema_version":"ojs-exec-history/0.1","started_at":"2025-01-01T00:00:01Z","envelope_id":"01HXXX000000000000000000A2","span_id":"s2","span_kind":"job.attempt","outcome":"retryable","duration_ms":34,"sdk":"ojs-go-sdk","sdk_version":"0.3.0","lang":"go","attempt":1}
`

func newTestServer(t *testing.T) (*server, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.jsonl")
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	in := &bytes.Buffer{}
	out := &bytes.Buffer{}
	srv := newServer(in, out, path, "envelope")
	return srv, in, out
}

// runOne drives the server through a script and returns all messages.
func runOne(t *testing.T, srv *server, in *bytes.Buffer, out *bytes.Buffer) []map[string]any {
	t.Helper()
	// Wrap server output behind a pipe so Run can read EOF when client done.
	pr, pw := io.Pipe()
	srv.in = in // requests already buffered
	// Replace srv.out with pw-backed writer so we can drain in parallel.
	srv.out = bufio.NewWriter(pw)
	var wg sync.WaitGroup
	var msgs []map[string]any
	wg.Add(1)
	go func() {
		defer wg.Done()
		msgs = drainAll(t, pr)
	}()
	if err := srv.Run(context.Background()); err != nil {
		// disconnect returns io.EOF which Run swallows; other errors fatal.
		t.Fatalf("Run: %v", err)
	}
	_ = srv.out.Flush()
	_ = pw.Close()
	wg.Wait()
	_ = out // unused; we drain via pipe
	return msgs
}

func responsesByCommand(msgs []map[string]any, cmd string) []map[string]any {
	var out []map[string]any
	for _, m := range msgs {
		if m["type"] == "response" && m["command"] == cmd {
			out = append(out, m)
		}
	}
	return out
}

func eventsByName(msgs []map[string]any, name string) []map[string]any {
	var out []map[string]any
	for _, m := range msgs {
		if m["type"] == "event" && m["event"] == name {
			out = append(out, m)
		}
	}
	return out
}

func TestServeFullSession(t *testing.T) {
	srv, in, out := newTestServer(t)
	writeReq(t, in, 1, "initialize", nil)
	writeReq(t, in, 2, "launch", map[string]any{})
	writeReq(t, in, 3, "configurationDone", nil)
	writeReq(t, in, 4, "threads", nil)
	writeReq(t, in, 5, "stackTrace", nil)
	writeReq(t, in, 6, "scopes", nil)
	writeReq(t, in, 7, "continue", map[string]any{"threadId": 1})
	writeReq(t, in, 8, "continue", map[string]any{"threadId": 2})
	writeReq(t, in, 9, "disconnect", nil)

	msgs := runOne(t, srv, in, out)

	if got := len(responsesByCommand(msgs, "initialize")); got != 1 {
		t.Errorf("initialize responses: %d", got)
	}
	if got := len(eventsByName(msgs, "initialized")); got != 1 {
		t.Errorf("initialized events: %d", got)
	}
	stops := eventsByName(msgs, "stopped")
	if len(stops) != 2 {
		t.Errorf("expected 2 stopped events, got %d", len(stops))
	}
	if len(stops) >= 2 {
		// First span is success → reason "step", second is retryable → "exception".
		body0 := stops[0]["body"].(map[string]any)
		if body0["reason"] != "step" {
			t.Errorf("first stop reason = %v, want step", body0["reason"])
		}
		body1 := stops[1]["body"].(map[string]any)
		if body1["reason"] != "exception" {
			t.Errorf("second stop reason = %v, want exception", body1["reason"])
		}
	}
	if got := len(eventsByName(msgs, "terminated")); got != 1 {
		t.Errorf("terminated events: %d", got)
	}

	// threads response should include 2 threads (one per envelope).
	threadResps := responsesByCommand(msgs, "threads")
	if len(threadResps) == 0 {
		t.Fatal("no threads response")
	}
	body := threadResps[0]["body"].(map[string]any)
	threads := body["threads"].([]any)
	if len(threads) != 1 {
		// At time of threads request only the first span has stopped, so
		// only 1 thread is visible. Sanity-check that.
		t.Errorf("threads at first stop = %d, want 1", len(threads))
	}
}

func TestServeVariablesExpansion(t *testing.T) {
	srv, in, out := newTestServer(t)
	writeReq(t, in, 1, "initialize", nil)
	writeReq(t, in, 2, "launch", nil)
	writeReq(t, in, 3, "configurationDone", nil)
	writeReq(t, in, 4, "scopes", nil)
	// We don't know the variablesReference yet — request id 5 will use a
	// hardcoded reference based on knowledge of the impl (first allocVar
	// returns 1000). If the impl changes this expectation should change.
	writeReq(t, in, 5, "variables", map[string]any{"variablesReference": 1000})
	writeReq(t, in, 6, "disconnect", nil)

	msgs := runOne(t, srv, in, out)

	scopeResps := responsesByCommand(msgs, "scopes")
	if len(scopeResps) == 0 {
		t.Fatal("no scopes response")
	}
	scopes := scopeResps[0]["body"].(map[string]any)["scopes"].([]any)
	if len(scopes) != 1 {
		t.Fatalf("expected 1 scope, got %d", len(scopes))
	}
	ref := int(scopes[0].(map[string]any)["variablesReference"].(float64))
	if ref != 1000 {
		t.Logf("note: variables ref = %d (not 1000); test expectation may need updating", ref)
	}
	varResps := responsesByCommand(msgs, "variables")
	if len(varResps) == 0 {
		t.Fatal("no variables response")
	}
	vars := varResps[0]["body"].(map[string]any)["variables"].([]any)
	if len(vars) == 0 {
		t.Fatal("variables response empty")
	}
	// Check that some core span fields appear.
	names := map[string]bool{}
	for _, v := range vars {
		names[v.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"envelope_id", "span_id", "outcome", "sdk"} {
		if !names[want] {
			t.Errorf("expected variable %q in scope, names = %v", want, names)
		}
	}
}

func TestReadDAPMessageMalformed(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("garbage no headers\r\n\r\n"))
	if _, err := readDAPMessage(r); err == nil {
		t.Error("expected error for missing content-length")
	}
}

func TestServeUnknownCommand(t *testing.T) {
	srv, in, out := newTestServer(t)
	writeReq(t, in, 1, "initialize", nil)
	writeReq(t, in, 2, "launch", nil)
	writeReq(t, in, 3, "totallyMadeUp", nil)
	writeReq(t, in, 4, "disconnect", nil)
	msgs := runOne(t, srv, in, out)
	r := responsesByCommand(msgs, "totallyMadeUp")
	if len(r) != 1 {
		t.Fatalf("expected 1 response for unknown cmd, got %d", len(r))
	}
	if r[0]["success"] != false {
		t.Errorf("unknown cmd should fail, got success=%v", r[0]["success"])
	}
}
