package verify

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func TestRegisterBuiltinsRegistersStructuredAndProbeVerifiers(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	for _, kind := range []string{
		"exit_code",
		"output_contains",
		"value_exists",
		"value_equals",
		"value_in",
		"string_contains_at",
		"string_matches_at",
		"number_compare",
		"collection_contains",
		"tcp_port_open",
		"file_exists_eventually",
		"file_content_contains_eventually",
		"http_eventually_status_code",
		"http_eventually_json_field_equals",
	} {
		if _, ok := reg.Get(kind); !ok {
			t.Fatalf("expected builtin verifier %q to be registered", kind)
		}
	}
}

func TestStructuredVerifiersEvaluateResultPaths(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	result := action.Result{
		OK: true,
		Data: map[string]any{
			"status_code": 200,
			"stdout":      "service ready on port 8080",
			"payload": map[string]any{
				"status": "ready",
				"tags":   []any{"blue", "green"},
				"count":  3,
			},
		},
		Meta: map[string]any{
			"attempts": 2,
		},
	}
	state := session.State{}

	spec := Spec{
		Mode: ModeAll,
		Checks: []Check{
			{Kind: "value_exists", Args: map[string]any{"path": "result.data.payload.status"}},
			{Kind: "value_equals", Args: map[string]any{"path": "result.data.payload.status", "expected": "ready"}},
			{Kind: "value_in", Args: map[string]any{"path": "result.data.status_code", "allowed": []any{200, 201}}},
			{Kind: "string_contains_at", Args: map[string]any{"path": "result.data.stdout", "text": "ready"}},
			{Kind: "string_matches_at", Args: map[string]any{"path": "result.data.stdout", "pattern": "port\\s+8080"}},
			{Kind: "number_compare", Args: map[string]any{"path": "result.meta.attempts", "op": "gte", "expected": 2}},
			{Kind: "collection_contains", Args: map[string]any{"path": "result.data.payload.tags", "expected": "green"}},
		},
	}

	got, err := reg.Evaluate(context.Background(), spec, result, state)
	if err != nil {
		t.Fatalf("evaluate structured verifiers: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected structured verifiers to succeed, got %#v", got)
	}
}

func TestStructuredVerifiersFailWhenPathOrExpectationIsWrong(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	result := action.Result{
		OK: true,
		Data: map[string]any{
			"stdout": "service booting",
			"count":  1,
		},
	}

	got, err := reg.Evaluate(context.Background(), Spec{
		Mode: ModeAll,
		Checks: []Check{
			{Kind: "value_exists", Args: map[string]any{"path": "result.data.missing"}},
			{Kind: "number_compare", Args: map[string]any{"path": "result.data.count", "op": "gt", "expected": 1}},
		},
	}, result, session.State{})
	if err != nil {
		t.Fatalf("evaluate failing structured verifiers: %v", err)
	}
	if got.Success {
		t.Fatalf("expected structured verifiers to fail, got %#v", got)
	}
}

func TestFileProbeVerifiersEventuallyObserveFilesystemState(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	dir := t.TempDir()
	path := filepath.Join(dir, "eventual.txt")

	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(path, []byte("eventual hello"), 0o644)
	}()

	got, err := reg.Evaluate(context.Background(), Spec{
		Mode: ModeAll,
		Checks: []Check{
			{Kind: "file_exists_eventually", Args: map[string]any{"path": path, "timeout_ms": 1000, "interval_ms": 25}},
			{Kind: "file_content_contains_eventually", Args: map[string]any{"path": path, "text": "hello", "timeout_ms": 1000, "interval_ms": 25}},
		},
	}, action.Result{}, session.State{})
	if err != nil {
		t.Fatalf("evaluate file probe verifiers: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected file probe verifiers to succeed, got %#v", got)
	}
}

func TestHTTPProbeVerifiersEventuallyObserveRemoteState(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests < 3 {
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"status":"booting"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	defer srv.Close()

	got, err := reg.Evaluate(context.Background(), Spec{
		Mode: ModeAll,
		Checks: []Check{
			{Kind: "http_eventually_status_code", Args: map[string]any{"url": srv.URL, "allowed": []any{200}, "timeout_ms": 1000, "interval_ms": 25}},
			{Kind: "http_eventually_json_field_equals", Args: map[string]any{"url": srv.URL, "field": "status", "expected": "ready", "timeout_ms": 1000, "interval_ms": 25}},
		},
	}, action.Result{}, session.State{})
	if err != nil {
		t.Fatalf("evaluate http probe verifiers: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected http probe verifiers to succeed, got %#v", got)
	}
}

func TestTCPPortOpenVerifierDetectsListeningPort(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer ln.Close()

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected tcp addr, got %T", ln.Addr())
	}

	got, err := reg.Evaluate(context.Background(), Spec{
		Mode: ModeAll,
		Checks: []Check{
			{Kind: "tcp_port_open", Args: map[string]any{"host": "127.0.0.1", "port": tcpAddr.Port, "timeout_ms": 500}},
		},
	}, action.Result{}, session.State{})
	if err != nil {
		t.Fatalf("evaluate tcp probe verifier: %v", err)
	}
	if !got.Success {
		t.Fatalf("expected tcp probe verifier to succeed, got %#v", got)
	}
}
