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

// extractAPIKey retrieves the ZipTax API key from (in priority order):
//  1. The X-API-KEY request header
//  2. The Authorization request header (with or without "Bearer " prefix)
func extractAPIKey(request mcp.CallToolRequest) string {
	if key := request.Header.Get("X-API-KEY"); key != "" {
		return key
	}
	if auth := request.Header.Get("Authorization"); auth != "" {
		auth = strings.TrimPrefix(auth, "Bearer ")
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
	return mcp.NewTool(
		"lookup_tax_rate",
		mcp.WithDescription(
			"Look up sales and use tax rates for a US or Canadian location. "+
				"Provide a postal code at minimum, or a full address for more precise results. "+
				"Returns tax rates broken down by jurisdiction (state, county, city, district). "+
				"Requires a valid ZipTax API key via the X-API-KEY or Authorization header. "+
				"Get a key at https://platform.zip.tax"),
		mcp.WithString("postalcode",
			mcp.Description("US ZIP code (5-digit) or Canadian postal code"),
		),
		mcp.WithString("address",
			mcp.Description("Full street address for geocoded lookup (requires geo-enabled account)"),
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
			mcp.Description("Country code: US (default) or CA for Canada"),
		),
		mcp.WithString("lat",
			mcp.Description("Latitude for coordinate-based lookup (requires geo-enabled account)"),
		),
		mcp.WithString("lng",
			mcp.Description("Longitude for coordinate-based lookup (requires geo-enabled account)"),
		),
		mcp.WithString("historical",
			mcp.Description("Historical period in YYYYMM format (e.g., 202312 for December 2023)"),
		),
		mcp.WithString("adjustment",
			mcp.Description("Set to 'auto' to enable state-specific unincorporated area adjustments"),
		),
		mcp.WithString("taxability_code",
			mcp.Description("Product taxability code (TIC) for product-specific tax rules (requires entitlement)"),
		),
		mcp.WithString("sat_item_total",
			mcp.Description("Item total for Tennessee Single Article Tax calculation"),
		),
		mcp.WithString("format",
			mcp.Description("Response format: 'json' (default) or 'xml'"),
		),
	)
}

func lookupTaxRateHandler(client *ZipTaxClient) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		apiKey := extractAPIKey(request)
		if apiKey == "" {
			return mcp.NewToolResultError("Missing API key. Send it via the X-API-KEY or Authorization header. Get a key at https://platform.zip.tax"), nil
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

		if params["postalcode"] == "" && params["address"] == "" && params["lat"] == "" {
			return mcp.NewToolResultError("At least one of postalcode, address, or lat/lng is required"), nil
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
		mcp.WithDescription(
			"Get account usage metrics and quota information for the authenticated ZipTax account. "+
				"Returns current request counts, limits, and entitlements. "+
				"Requires a valid ZipTax API key via the X-API-KEY or Authorization header. "+
				"Get a key at https://platform.zip.tax"),
	)
}

func getAccountMetricsHandler(client *ZipTaxClient) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		apiKey := extractAPIKey(request)
		if apiKey == "" {
			return mcp.NewToolResultError("Missing API key. Send it via the X-API-KEY or Authorization header. Get a key at https://platform.zip.tax"), nil
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
