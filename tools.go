package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const defaultVersion = "v60"

// errMissingAPIKey is returned when a tool call carries no API key. It
// must only reference header-based authentication.
const errMissingAPIKey = "Missing API key. Send it in the X-API-KEY header or as a Bearer token in the Authorization header. Get a key at https://platform.zip.tax"

// extractAPIKey retrieves the ZipTax API key from (in priority order):
//  1. The X-API-KEY request header (also populated by the legacy ?key=
//     URL parameter fallback middleware)
//  2. The Authorization request header (with or without "Bearer " prefix)
func extractAPIKey(request mcp.CallToolRequest) string {
	if key := request.Header.Get("X-API-KEY"); key != "" {
		return key
	}
	if auth := request.Header.Get("Authorization"); auth != "" {
		// The auth scheme is case-insensitive per RFC 7235, so accept
		// "Bearer", "bearer", "BEARER", etc.
		const bearerPrefix = "Bearer "
		if len(auth) >= len(bearerPrefix) && strings.EqualFold(auth[:len(bearerPrefix)], bearerPrefix) {
			auth = auth[len(bearerPrefix):]
		}
		return strings.TrimSpace(auth)
	}
	return ""
}

// RegisterTools registers all MCP tools on the given server.
func RegisterTools(s *server.MCPServer, client *ZipTaxClient) {
	s.AddTool(lookupTaxRateTool(), lookupTaxRateHandler(client))
	s.AddTool(getAccountMetricsTool(), getAccountMetricsHandler(client))
}

// --- lookup_tax_rate ---

func lookupTaxRateTool() mcp.Tool {
	tool := mcp.NewTool(
		"lookup_tax_rate",
		mcp.WithTitleAnnotation("Look Up Sales Tax Rate"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription(
			"Look up sales and use tax rates for a US or Canadian location. "+
				"Provide a full street address for door-level accuracy, or a lat/lng pair for a geographic point lookup. "+
				"Returns tax rates broken down by jurisdiction (state, county, city, district). "+
				"Requires a valid ZipTax API key sent in the X-API-KEY header or Authorization header. "+
				"Get a key at https://platform.zip.tax"),
		mcp.WithString("postalcode",
			mcp.Description("US ZIP code (5-digit) or Canadian postal code. Least precise option: returns every rate overlapping the ZIP rather than one authoritative rate, with no adjustment for unincorporated areas. Use only when no address or lat/lng is available"),
		),
		mcp.WithString("address",
			mcp.Description("Full street address for door-level geocoded lookup. Preferred input"),
		),
		mcp.WithString("state",
			mcp.Description("Two-letter US state or Canadian province code (e.g., CA, ON)"),
		),
		mcp.WithString("city",
			mcp.Description("City name"),
		),
		mcp.WithString("county",
			mcp.Description("County name"),
		),
		mcp.WithString("country_code",
			mcp.Description("Country code: US (default) or CA for Canada. CA requires a Pro or Enterprise plan"),
		),
		mcp.WithString("lat",
			mcp.Description("Latitude for coordinate-based lookup. Same door-level precision as an address"),
		),
		mcp.WithString("lng",
			mcp.Description("Longitude for coordinate-based lookup. Same door-level precision as an address"),
		),
		mcp.WithString("historical",
			mcp.Description("Historical period in YYYYMM format (e.g., 202601 for January 2026); lookback is limited to the past 12 months. Requires a Pro or Enterprise plan"),
		),
		mcp.WithString("adjustment",
			mcp.Description("Set to 'auto' to enable state-specific unincorporated area adjustments"),
		),
		mcp.WithString("taxability_code",
			mcp.Description("Product taxability code (TIC) for product-specific tax rules. Requires a Pro or Enterprise plan"),
		),
		mcp.WithString("sat_item_total",
			mcp.Description("Item total for Tennessee Single Article Tax calculation"),
		),
		mcp.WithString("format",
			mcp.Description("Response format: 'json' (default) or 'xml'"),
		),
	)

	// Callers must provide a jurisdiction-accurate location: a street
	// address or a lat/lng pair. (Postal-code-only lookups still work
	// for backward compatibility but are not advertised, since a postal
	// code is not accurate to a single jurisdiction.) The structured
	// ToolInputSchema type cannot express anyOf, so convert the built
	// schema to a raw JSON schema and add the constraint there. mcp-go
	// marshals RawInputSchema in place of InputSchema, and errors if
	// both are set.
	schemaJSON, err := json.Marshal(tool.InputSchema)
	if err != nil {
		panic(fmt.Sprintf("marshaling lookup_tax_rate input schema: %v", err))
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		panic(fmt.Sprintf("unmarshaling lookup_tax_rate input schema: %v", err))
	}
	schema["anyOf"] = []any{
		map[string]any{"required": []string{"address"}},
		map[string]any{"required": []string{"lat", "lng"}},
	}
	rawSchema, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("marshaling lookup_tax_rate raw input schema: %v", err))
	}
	tool.InputSchema = mcp.ToolInputSchema{}
	tool.RawInputSchema = rawSchema

	return tool
}

func lookupTaxRateHandler(client *ZipTaxClient) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		apiKey := extractAPIKey(request)
		if apiKey == "" {
			return mcp.NewToolResultError(errMissingAPIKey), nil
		}

		params := make(map[string]string)
		paramNames := []string{
			"postalcode", "address", "state", "city", "county",
			"country_code", "lat", "lng", "historical", "adjustment",
			"taxability_code", "sat_item_total", "format",
		}
		for _, name := range paramNames {
			if v := request.GetString(name, ""); v != "" {
				params[name] = v
			}
		}

		if params["postalcode"] == "" && params["address"] == "" && (params["lat"] == "" || params["lng"] == "") {
			return mcp.NewToolResultError("Provide a location: a full street address, or both lat and lng"), nil
		}

		result, err := client.LookupTax(apiKey, defaultVersion, params)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("ZipTax API error: %v", err)), nil
		}

		formatted, err := json.MarshalIndent(json.RawMessage(result), "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(formatted)), nil
	}
}

// --- get_account_metrics ---

func getAccountMetricsTool() mcp.Tool {
	return mcp.NewTool(
		"get_account_metrics",
		mcp.WithTitleAnnotation("Get Account Usage Metrics"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithDescription(
			"Get account usage metrics and quota information for the authenticated ZipTax account. "+
				"Returns current request counts, limits, and entitlements. "+
				"Requires a valid ZipTax API key sent in the X-API-KEY header or Authorization header. "+
				"Get a key at https://platform.zip.tax"),
	)
}

func getAccountMetricsHandler(client *ZipTaxClient) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		apiKey := extractAPIKey(request)
		if apiKey == "" {
			return mcp.NewToolResultError(errMissingAPIKey), nil
		}

		result, err := client.GetAccountMetrics(apiKey, defaultVersion)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("ZipTax API error: %v", err)), nil
		}

		formatted, err := json.MarshalIndent(json.RawMessage(result), "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to format response: %v", err)), nil
		}

		return mcp.NewToolResultText(string(formatted)), nil
	}
}
