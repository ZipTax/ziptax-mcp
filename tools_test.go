package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// The input schema must require a jurisdiction-accurate location: a
// street address or a lat/lng pair. Postal-code-only lookups are
// deliberately not advertised in the schema.
func TestLookupTaxRateSchemaRequiresAddressOrLatLng(t *testing.T) {
	data, err := json.Marshal(lookupTaxRateTool())
	if err != nil {
		t.Fatalf("marshaling tool: %v", err)
	}

	var decoded struct {
		InputSchema struct {
			Type       string                     `json:"type"`
			Properties map[string]json.RawMessage `json:"properties"`
			AnyOf      []struct {
				Required []string `json:"required"`
			} `json:"anyOf"`
		} `json:"inputSchema"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshaling tool: %v", err)
	}

	schema := decoded.InputSchema
	if schema.Type != "object" {
		t.Errorf("inputSchema type = %q, want %q", schema.Type, "object")
	}

	if len(schema.AnyOf) != 2 {
		t.Fatalf("anyOf has %d branches, want 2: %+v", len(schema.AnyOf), schema.AnyOf)
	}
	if got := schema.AnyOf[0].Required; len(got) != 1 || got[0] != "address" {
		t.Errorf("anyOf[0].required = %v, want [address]", got)
	}
	if got := schema.AnyOf[1].Required; len(got) != 2 || got[0] != "lat" || got[1] != "lng" {
		t.Errorf("anyOf[1].required = %v, want [lat lng]", got)
	}

	// The raw-schema conversion must not drop any parameters.
	for _, name := range []string{
		"postalcode", "address", "state", "city", "county",
		"country_code", "lat", "lng", "historical", "adjustment",
		"taxability_code", "sat_item_total", "format",
	} {
		if _, ok := schema.Properties[name]; !ok {
			t.Errorf("inputSchema is missing property %q", name)
		}
	}
}

func TestLookupTaxRateHandlerLocationGuard(t *testing.T) {
	handler := lookupTaxRateHandler(nil) // guard failures return before the client is used

	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "no location", args: map[string]any{}},
		{name: "lat without lng", args: map[string]any{"lat": "33.65"}},
		{name: "lng without lat", args: map[string]any{"lng": "-117.74"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := mcp.CallToolRequest{Header: http.Header{"X-Api-Key": {"testkey"}}}
			request.Params.Arguments = tt.args

			result, err := handler(t.Context(), request)
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if !result.IsError {
				t.Error("expected a location-guard error result")
			}
		})
	}
}

// Postal-code-only lookups are not advertised in the schema but must
// keep working for existing clients.
func TestLookupTaxRateHandlerAcceptsPostalCodeOnly(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("postalcode"); got != "90210" {
			t.Errorf("postalcode = %q, want %q", got, "90210")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	handler := lookupTaxRateHandler(NewZipTaxClient(ts.URL))
	request := mcp.CallToolRequest{Header: http.Header{"X-Api-Key": {"testkey"}}}
	request.Params.Arguments = map[string]any{"postalcode": "90210"}

	result, err := handler(t.Context(), request)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success for a postalcode-only lookup")
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
