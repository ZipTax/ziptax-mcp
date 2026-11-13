package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// captureLog redirects the standard logger to a buffer for the duration
// of the test and resets the deprecation-warning throttle so the warning
// is eligible to fire.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	lastKeyParamWarnUnix.Store(0)
	return &buf
}

func TestAPIKeyFromQuery(t *testing.T) {
	t.Run("promotes ?key= to X-API-KEY header", func(t *testing.T) {
		logs := captureLog(t)

		var gotKey string
		handler := apiKeyFromQuery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotKey = r.Header.Get("X-API-KEY")
		}))

		req := httptest.NewRequest(http.MethodPost, "/?key=legacy-secret", nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)

		if gotKey != "legacy-secret" {
			t.Errorf("X-API-KEY = %q, want %q", gotKey, "legacy-secret")
		}
		if !strings.Contains(logs.String(), "deprecated") {
			t.Error("expected a deprecation warning in the logs")
		}
		if strings.Contains(logs.String(), "legacy-secret") {
			t.Error("deprecation warning must not contain the API key")
		}
	})

	t.Run("existing X-API-KEY header wins over ?key=", func(t *testing.T) {
		captureLog(t)

		var gotKey string
		handler := apiKeyFromQuery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotKey = r.Header.Get("X-API-KEY")
		}))

		req := httptest.NewRequest(http.MethodPost, "/?key=fromquery", nil)
		req.Header.Set("X-API-KEY", "fromheader")
		handler.ServeHTTP(httptest.NewRecorder(), req)

		if gotKey != "fromheader" {
			t.Errorf("X-API-KEY = %q, want %q", gotKey, "fromheader")
		}
	})

	t.Run("no ?key= leaves the header unset and logs nothing", func(t *testing.T) {
		logs := captureLog(t)

		var gotKey string
		handler := apiKeyFromQuery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotKey = r.Header.Get("X-API-KEY")
		}))

		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))

		if gotKey != "" {
			t.Errorf("X-API-KEY = %q, want empty", gotKey)
		}
		if logs.Len() != 0 {
			t.Errorf("unexpected log output: %s", logs.String())
		}
	})

	t.Run("deprecation warning is throttled", func(t *testing.T) {
		logs := captureLog(t)

		handler := apiKeyFromQuery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		for range 5 {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/?key=abc", nil))
		}

		if got := strings.Count(logs.String(), "deprecated"); got != 1 {
			t.Errorf("deprecation warning logged %d times, want 1", got)
		}
	})
}

// A tool call sent through the production HTTP handler with the legacy
// ?key= parameter is logged with the key's hash prefix, and neither the
// usage log nor the standard logger (which mcp-go also writes to) carries
// the key or its full hash.
func TestKeyParamToolCallIsLoggedWithoutKeyMaterial(t *testing.T) {
	const apiKey = "zt-usage-log-query-key-9e2c"
	stdLogs := captureLog(t)
	logs := captureUsageLog(t)

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != apiKey {
			t.Errorf("API received key %q, want the ?key= value", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"requestCount":1}`))
	}))
	defer api.Close()

	mcpSrv := httptest.NewServer(newMCPHandler(newMCPServer(NewZipTaxClient(api.URL), &methodCounts{})))
	defer mcpSrv.Close()

	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_account_metrics","arguments":{}}}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, mcpSrv.URL+"/?key="+apiKey, strings.NewReader(body))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("calling MCP server: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response: %v", err)
	}
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(respBody), "requestCount") {
		t.Fatalf("response %d %s, want 200 with the metrics result", resp.StatusCode, respBody)
	}

	lines := usageLines(t, logs)
	if len(lines) != 1 {
		t.Fatalf("got %d usage log lines, want 1: %s", len(lines), logs)
	}
	for field, want := range map[string]any{
		"msg":      "mcp_tool_call",
		"tool":     "get_account_metrics",
		"key_hash": keyHashPrefix(apiKey),
		"outcome":  "ok",
	} {
		if lines[0][field] != want {
			t.Errorf("%s = %v, want %v", field, lines[0][field], want)
		}
	}
	if !strings.Contains(stdLogs.String(), "deprecated") {
		t.Errorf("want the ?key= deprecation warning in the standard log, got: %s", stdLogs)
	}
	assertNoKeyMaterial(t, logs.String(), apiKey)
	assertNoKeyMaterial(t, stdLogs.String(), apiKey)
}
