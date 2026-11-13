package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// captureUsageLog points usageLogger at a buffer for the duration of the
// test.
func captureUsageLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := usageLogger
	usageLogger = slog.New(slog.NewJSONHandler(&buf, nil))
	t.Cleanup(func() { usageLogger = prev })
	return &buf
}

// usageLines decodes each line of the usage log. A line that is not a
// JSON object fails the test, since Logs Insights only parses JSON lines.
func usageLines(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	for _, raw := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if raw == "" {
			continue
		}
		var line map[string]any
		if err := json.Unmarshal([]byte(raw), &line); err != nil {
			t.Fatalf("usage log line is not a JSON object: %q: %v", raw, err)
		}
		lines = append(lines, line)
	}
	return lines
}

// longHexRun matches any run of hex characters longer than a key-hash
// prefix, which would mean a full or wider hash reached the log.
var longHexRun = regexp.MustCompile(fmt.Sprintf("[0-9a-fA-F]{%d,}", keyHashPrefixLen+1))

// assertNoKeyMaterial fails if logs carry apiKey, its full SHA-256 hash,
// or any hex run longer than a key-hash prefix.
func assertNoKeyMaterial(t *testing.T, logs, apiKey string) {
	t.Helper()
	sum := sha256.Sum256([]byte(apiKey))
	fullHash := hex.EncodeToString(sum[:])
	if strings.Contains(logs, apiKey) {
		t.Errorf("log output contains the raw API key: %s", logs)
	}
	if strings.Contains(strings.ToLower(logs), fullHash) {
		t.Errorf("log output contains the full SHA-256 of the API key: %s", logs)
	}
	if run := longHexRun.FindString(logs); run != "" {
		t.Errorf("log output contains a hex run of %d characters (%q), want at most %d", len(run), run, keyHashPrefixLen)
	}
}

func TestKeyHashPrefix(t *testing.T) {
	// SHA-256("abc") is ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
	// (FIPS 180-2 test vector), so Databricks' left(sha2('abc', 256), 12)
	// gives the same prefix.
	if got, want := keyHashPrefix("abc"), "ba7816bf8f01"; got != want {
		t.Errorf("keyHashPrefix(%q) = %q, want %q", "abc", got, want)
	}
	if got := keyHashPrefix(""); got != "" {
		t.Errorf("keyHashPrefix(\"\") = %q, want empty", got)
	}
}

// timeoutError is a net.Error that reports a timeout.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestUpstreamOutcome(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want callOutcome
	}{
		{name: "success", err: nil, want: callOutcome{class: outcomeOK, upstreamStatus: http.StatusOK}},
		{name: "non-200 status", err: &APIStatusError{StatusCode: http.StatusUnauthorized, Body: "bad key"}, want: callOutcome{class: outcomeUpstreamStatus, upstreamStatus: http.StatusUnauthorized}},
		{name: "wrapped non-200 status", err: fmt.Errorf("lookup: %w", &APIStatusError{StatusCode: http.StatusTooManyRequests}), want: callOutcome{class: outcomeUpstreamStatus, upstreamStatus: http.StatusTooManyRequests}},
		{name: "timeout", err: fmt.Errorf("API request failed: %w", timeoutError{}), want: callOutcome{class: outcomeUpstreamTimeout}},
		{name: "connection failure", err: fmt.Errorf("API request failed: %w", errors.New("connection refused")), want: callOutcome{class: outcomeUpstreamError}},
		{name: "truncated body keeps the status", err: &ResponseReadError{StatusCode: http.StatusServiceUnavailable, Err: io.ErrUnexpectedEOF}, want: callOutcome{class: outcomeUpstreamError, upstreamStatus: http.StatusServiceUnavailable}},
		{name: "body read timeout keeps the status", err: &ResponseReadError{StatusCode: http.StatusOK, Err: timeoutError{}}, want: callOutcome{class: outcomeUpstreamTimeout, upstreamStatus: http.StatusOK}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := upstreamOutcome(tt.err); got != tt.want {
				t.Errorf("upstreamOutcome() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A panicking tool body is still logged, and the panic still reaches the
// server's recovery.
func TestWithUsageLogRecordsAPanickingCall(t *testing.T) {
	logs := captureUsageLog(t)
	const apiKey = "zt-usage-log-panic-key"

	handler := withUsageLog("boom", func(ctx context.Context, request mcp.CallToolRequest, apiKey string) (*mcp.CallToolResult, callOutcome) {
		panic("boom")
	})

	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic was swallowed, want it re-raised for the server's recovery")
			}
		}()
		request := mcp.CallToolRequest{Header: http.Header{"X-Api-Key": {apiKey}}}
		_, _ = handler(t.Context(), request)
	}()

	lines := usageLines(t, logs)
	if len(lines) != 1 {
		t.Fatalf("got %d usage log lines, want 1: %s", len(lines), logs)
	}
	if got := lines[0]["outcome"]; got != outcomeInternalError {
		t.Errorf("outcome = %v, want %q", got, outcomeInternalError)
	}
	assertNoKeyMaterial(t, logs.String(), apiKey)
}

// Requests dispatched by the real server are counted by method and logged
// only on flush, one line per method.
func TestMethodCountsLogSetupTraffic(t *testing.T) {
	logs := captureUsageLog(t)
	methods := &methodCounts{}
	s := newMCPServer(NewZipTaxClient("http://127.0.0.1:0"), methods)

	for _, msg := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		// No request header reaches HandleMessage here, so this call is
		// rejected for a missing key and logged as a tool call.
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_account_metrics","arguments":{}}}`,
	} {
		s.HandleMessage(t.Context(), json.RawMessage(msg))
	}

	lines := usageLines(t, logs)
	if len(lines) != 1 || lines[0]["msg"] != "mcp_tool_call" || lines[0]["missing_key"] != true {
		t.Fatalf("before flush, want only the missing-key tool call line, got: %s", logs)
	}

	logs.Reset()
	methods.flush()

	want := []struct {
		method   string
		requests float64
	}{
		{"initialize", 1},
		{"tools/call", 1},
		{"tools/list", 2},
	}
	lines = usageLines(t, logs)
	if len(lines) != len(want) {
		t.Fatalf("got %d method count lines, want %d: %s", len(lines), len(want), logs)
	}
	for i, w := range want {
		line := lines[i]
		if line["msg"] != "mcp_method_count" || line["method"] != w.method || line["requests"] != w.requests {
			t.Errorf("line %d = %v, want msg=mcp_method_count method=%s requests=%v", i, line, w.method, w.requests)
		}
	}

	logs.Reset()
	methods.flush()
	if logs.Len() != 0 {
		t.Errorf("flush with no new requests logged: %s", logs)
	}
}

// Concurrent counting and flushing must neither race nor lose requests.
func TestMethodCountsConcurrentCountAndFlush(t *testing.T) {
	logs := captureUsageLog(t)
	methods := &methodCounts{}

	const goroutines, perGoroutine = 8, 250
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perGoroutine {
				methods.count(t.Context(), nil, mcp.MethodToolsList, nil)
			}
		}()
	}
	stop := make(chan struct{})
	flushed := make(chan struct{})
	go func() {
		defer close(flushed)
		for {
			select {
			case <-stop:
				return
			default:
				methods.flush()
			}
		}
	}()
	wg.Wait()
	close(stop)
	<-flushed
	methods.flush()

	var total float64
	for _, line := range usageLines(t, logs) {
		total += line["requests"].(float64)
	}
	if want := float64(goroutines * perGoroutine); total != want {
		t.Errorf("flushed %v requests in total, want %v", total, want)
	}
}
