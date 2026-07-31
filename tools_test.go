package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func assertReadOnlyAnnotations(t *testing.T, tool mcp.Tool, wantTitle string) {
	t.Helper()

	if tool.Annotations.Title != wantTitle {
		t.Errorf("title annotation = %q, want %q", tool.Annotations.Title, wantTitle)
	}
	if tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
		t.Errorf("readOnlyHint = %v, want true", tool.Annotations.ReadOnlyHint)
	}
	if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
		t.Errorf("destructiveHint = %v, want false", tool.Annotations.DestructiveHint)
	}
	if tool.Annotations.IdempotentHint == nil || !*tool.Annotations.IdempotentHint {
		t.Errorf("idempotentHint = %v, want true", tool.Annotations.IdempotentHint)
	}
	if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
		t.Errorf("openWorldHint = %v, want false", tool.Annotations.OpenWorldHint)
	}
}

func TestLookupTaxRateToolAnnotations(t *testing.T) {
	assertReadOnlyAnnotations(t, lookupTaxRateTool(), "Look Up Sales Tax Rate")
}

func TestGetAccountMetricsToolAnnotations(t *testing.T) {
	assertReadOnlyAnnotations(t, getAccountMetricsTool(), "Get Account Usage Metrics")
}

// Directory review feedback: user-facing text must not point users at the
// ?key= URL parameter, since keys in URLs end up in logs. Marshaling the
// whole tool covers the description and every parameter description.
func TestUserFacingTextDoesNotMentionURLKeyParameter(t *testing.T) {
	for _, tool := range []mcp.Tool{lookupTaxRateTool(), getAccountMetricsTool()} {
		data, err := json.Marshal(tool)
		if err != nil {
			t.Fatalf("marshaling %s: %v", tool.Name, err)
		}
		if strings.Contains(string(data), "?key=") {
			t.Errorf("%s definition mentions the ?key= URL parameter", tool.Name)
		}
	}
	if strings.Contains(serverInstructions, "?key=") {
		t.Error("server instructions mention the ?key= URL parameter")
	}
	if strings.Contains(errMissingAPIKey, "?key=") {
		t.Error("missing-API-key error message mentions the ?key= URL parameter")
	}
}

func TestExtractAPIKey(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   string
	}{
		{
			name:   "no headers",
			header: http.Header{},
			want:   "",
		},
		{
			name:   "X-API-KEY header",
			header: http.Header{"X-Api-Key": {"abc123"}},
			want:   "abc123",
		},
		{
			name:   "Authorization with Bearer prefix",
			header: http.Header{"Authorization": {"Bearer abc123"}},
			want:   "abc123",
		},
		{
			name:   "Authorization with lowercase bearer prefix",
			header: http.Header{"Authorization": {"bearer abc123"}},
			want:   "abc123",
		},
		{
			name:   "Authorization with uppercase BEARER prefix",
			header: http.Header{"Authorization": {"BEARER abc123"}},
			want:   "abc123",
		},
		{
			name:   "Authorization with raw key and no scheme",
			header: http.Header{"Authorization": {"abc123"}},
			want:   "abc123",
		},
		{
			name:   "X-API-KEY takes priority over Authorization",
			header: http.Header{"X-Api-Key": {"fromheader"}, "Authorization": {"Bearer fromauth"}},
			want:   "fromheader",
		},
		{
			name:   "bare Bearer scheme with no token is returned unchanged",
			header: http.Header{"Authorization": {"Bearer"}},
			want:   "Bearer",
		},
		{
			name:   "Bearer prefix requires a space",
			header: http.Header{"Authorization": {"Bearertoken"}},
			want:   "Bearertoken",
		},
		{
			name:   "extra whitespace around the token is trimmed",
			header: http.Header{"Authorization": {"bearer  abc123 "}},
			want:   "abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.CallToolRequest{Header: tt.header}
			if got := extractAPIKey(request); got != tt.want {
				t.Errorf("extractAPIKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
