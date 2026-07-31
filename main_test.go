package main

import (
	"bytes"
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
