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

func TestStringArg(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		param   string
		want    string
		wantErr bool
	}{
		{name: "absent", args: map[string]any{}, param: "address", want: ""},
		{name: "null", args: map[string]any{"address": nil}, param: "address", want: ""},
		{name: "string", args: map[string]any{"address": "200 Spectrum Center Dr"}, param: "address", want: "200 Spectrum Center Dr"},
		{name: "numeric lat", args: map[string]any{"lat": 33.65253}, param: "lat", want: "33.65253"},
		{name: "numeric negative lng", args: map[string]any{"lng": -117.74794}, param: "lng", want: "-117.74794"},
		{name: "numeric historical", args: map[string]any{"historical": float64(202601)}, param: "historical", want: "202601"},
		{name: "numeric sat_item_total", args: map[string]any{"sat_item_total": 1500.5}, param: "sat_item_total", want: "1500.5"},
		{name: "numeric postalcode is rejected", args: map[string]any{"postalcode": float64(2134)}, param: "postalcode", wantErr: true},
		{name: "numeric taxability_code is rejected", args: map[string]any{"taxability_code": float64(20010)}, param: "taxability_code", wantErr: true},
		{name: "boolean lat is rejected", args: map[string]any{"lat": true}, param: "lat", wantErr: true},
		{name: "object address is rejected", args: map[string]any{"address": map[string]any{"street": "x"}}, param: "address", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stringArg(tt.args, tt.param)
			if (err != nil) != tt.wantErr {
				t.Fatalf("stringArg() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("stringArg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "compact JSON is indented",
			body: `{"rate":0.0775,"jurName":"CA"}`,
			want: "{\n  \"rate\": 0.0775,\n  \"jurName\": \"CA\"\n}",
		},
		{
			name: "XML is returned unchanged",
			body: `<?xml version="1.0"?><response><rate>0.0775</rate></response>`,
			want: `<?xml version="1.0"?><response><rate>0.0775</rate></response>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatResponse(json.RawMessage(tt.body)); got != tt.want {
				t.Errorf("formatResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLookupTaxRateHandlerArgumentTypes(t *testing.T) {
	const xmlBody = `<?xml version="1.0"?><response><rate>0.0775</rate></response>`

	tests := []struct {
		name      string
		args      map[string]any
		wantQuery map[string]string // nil means the API must not be called
		wantText  string
	}{
		{
			name:      "numeric lat and lng reach the API as strings",
			args:      map[string]any{"lat": 33.65253, "lng": -117.74794, "historical": float64(202601)},
			wantQuery: map[string]string{"lat": "33.65253", "lng": "-117.74794", "historical": "202601"},
			wantText:  "{\n  \"ok\": true\n}",
		},
		{
			name:      "format=xml returns the XML body",
			args:      map[string]any{"address": "200 Spectrum Center Dr, Irvine, CA", "format": "xml"},
			wantQuery: map[string]string{"format": "xml"},
			wantText:  xmlBody,
		},
		{
			name:     "numeric postalcode is an error",
			args:     map[string]any{"postalcode": float64(2134)},
			wantText: "postalcode must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.wantQuery == nil {
					t.Error("API was called, want no call")
				}
				for k, want := range tt.wantQuery {
					if got := r.URL.Query().Get(k); got != want {
						t.Errorf("query %s = %q, want %q", k, got, want)
					}
				}
				if r.URL.Query().Get("format") == "xml" {
					w.Header().Set("Content-Type", "application/xml")
					_, _ = w.Write([]byte(xmlBody))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer ts.Close()

			handler := lookupTaxRateHandler(NewZipTaxClient(ts.URL))
			request := mcp.CallToolRequest{Header: http.Header{"X-Api-Key": {"testkey"}}}
			request.Params.Arguments = tt.args

			result, err := handler(t.Context(), request)
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if wantErr := tt.wantQuery == nil; result.IsError != wantErr {
				t.Errorf("IsError = %v, want %v", result.IsError, wantErr)
			}
			if len(result.Content) != 1 {
				t.Fatalf("got %d content items, want 1", len(result.Content))
			}
			text, ok := result.Content[0].(mcp.TextContent)
			if !ok {
				t.Fatalf("content is %T, want mcp.TextContent", result.Content[0])
			}
			if text.Text != tt.wantText {
				t.Errorf("text = %q, want %q", text.Text, tt.wantText)
			}
		})
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
