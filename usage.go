package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// usageLogger writes the usage log: one JSON line per tool call, plus
// periodic MCP method counts. Stderr is shipped by ECS to the
// /ecs/ziptax-mcp CloudWatch log group. Lines never carry the API key or
// its full hash.
var usageLogger = slog.New(slog.NewJSONHandler(os.Stderr, nil))

// keyHashPrefixLen is how many hex characters of SHA-256(API key) a usage
// log line carries: enough to tell accounts apart and to join on
// left(sha2(key, 256), 12) in Databricks, too few to stand in for the key
// or its full hash.
const keyHashPrefixLen = 12

// keyHashPrefix returns the first keyHashPrefixLen hex characters of the
// SHA-256 of apiKey, or "" when there is no key.
func keyHashPrefix(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:keyHashPrefixLen/2])
}

// Outcome classes recorded in a tool call's usage log line.
const (
	outcomeOK              = "ok"
	outcomeMissingKey      = "missing_key"
	outcomeInvalidArgs     = "invalid_arguments"
	outcomeUpstreamStatus  = "upstream_status" // the API answered with a status other than 200
	outcomeUpstreamTimeout = "upstream_timeout"
	outcomeUpstreamError   = "upstream_error" // any other failure reaching or reading the API
	outcomeInternalError   = "internal_error" // the tool handler panicked
)

// callOutcome is how a tool call ended, for its usage log line.
type callOutcome struct {
	class string
	// upstreamStatus is the ZipTax API's HTTP status, or 0 when the API
	// was not called or did not answer.
	upstreamStatus int
}

// upstreamOutcome classifies the error from a ZipTaxClient call.
func upstreamOutcome(err error) callOutcome {
	if err == nil {
		return callOutcome{class: outcomeOK, upstreamStatus: http.StatusOK}
	}
	var statusErr *APIStatusError
	if errors.As(err, &statusErr) {
		return callOutcome{class: outcomeUpstreamStatus, upstreamStatus: statusErr.StatusCode}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return callOutcome{class: outcomeUpstreamTimeout}
	}
	return callOutcome{class: outcomeUpstreamError}
}

// toolBody is the part of a tool handler that runs once the caller's API
// key is known. It returns the tool result and how the call ended.
type toolBody func(ctx context.Context, request mcp.CallToolRequest, apiKey string) (*mcp.CallToolResult, callOutcome)

// withUsageLog turns a tool body into a tool handler. It rejects calls
// that carry no API key and writes one usage log line per call, rejected
// calls included.
func withUsageLog(tool string, body toolBody) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		apiKey := extractAPIKey(request)
		// Stays internal_error only if the body panics; the server
		// recovers the panic (WithRecovery) after this line is logged.
		outcome := callOutcome{class: outcomeInternalError}
		defer func() { logToolCall(ctx, tool, apiKey, outcome, time.Since(start)) }()

		if apiKey == "" {
			outcome = callOutcome{class: outcomeMissingKey}
			return mcp.NewToolResultError(errMissingAPIKey), nil
		}

		var result *mcp.CallToolResult
		result, outcome = body(ctx, request, apiKey)
		return result, nil
	}
}

func logToolCall(ctx context.Context, tool, apiKey string, outcome callOutcome, latency time.Duration) {
	usageLogger.LogAttrs(ctx, slog.LevelInfo, "mcp_tool_call",
		slog.String("tool", tool),
		slog.String("key_hash", keyHashPrefix(apiKey)),
		slog.Bool("missing_key", apiKey == ""),
		slog.String("outcome", outcome.class),
		slog.Int("upstream_status", outcome.upstreamStatus),
		slog.Int64("latency_ms", latency.Milliseconds()),
	)
}

// methodCountsInterval is how often MCP method counts are logged.
const methodCountsInterval = 5 * time.Minute

// methodCounts tallies MCP requests by method between flushes, so setup
// traffic (initialize, tools/list) shows in the usage log as a few lines
// per interval rather than one line per request. Only methods the server
// dispatches reach the hook, so the set of keys stays small.
type methodCounts struct {
	mu     sync.Mutex
	counts map[string]int64
}

// count is a server.BeforeAnyHookFunc.
func (m *methodCounts) count(_ context.Context, _ any, method mcp.MCPMethod, _ any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counts == nil {
		m.counts = make(map[string]int64)
	}
	m.counts[string(method)]++
}

// flush logs one line per method counted since the last flush, then
// resets the counts. It logs nothing when no requests arrived.
func (m *methodCounts) flush() {
	m.mu.Lock()
	counts := m.counts
	m.counts = nil
	m.mu.Unlock()

	for _, method := range slices.Sorted(maps.Keys(counts)) {
		usageLogger.LogAttrs(context.Background(), slog.LevelInfo, "mcp_method_count",
			slog.String("method", method),
			slog.Int64("count", counts[method]),
		)
	}
}
