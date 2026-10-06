# Ziptax MCP Server

An HTTP [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that gives AI agents real-time US and Canadian sales and use tax rates from the [Ziptax API](https://zip.tax).

Once connected, an agent can:

- Look up sales and use tax rates by street address, latitude/longitude, or postal code.
- Get jurisdiction-level breakdowns (state, county, city, and district).
- Check account usage and quota.

Official documentation: [docs.zip.tax: MCP Server](https://docs.zip.tax/v-6-0/guides/agents-and-llms/mcp-server)

## Connection Details

| | |
|---|---|
| **Endpoint** | `https://mcp.zip-tax.com/` |
| **Method** | `POST` |
| **Transport** | Streamable HTTP (most clients call this `HTTP`) |
| **Sessions** | Stateless |
| **Server name** | `ZipTax Sales Tax API` |
| **Upstream API** | Ziptax API `v60` |

## Authentication

Send a Ziptax API key as an HTTP header. Two header methods are supported:

```http
X-API-KEY: your-api-key
```

```http
Authorization: Bearer your-api-key
```

- `X-API-KEY` is recommended. If both headers are present, `X-API-KEY` takes priority.
- The `Bearer` scheme is case-insensitive.
- Do not pass API keys in the URL. Keys in URLs can end up in logs.

Get an API key at [platform.zip.tax](https://platform.zip.tax).

## Client Configuration

Replace `your_api_key` with your Ziptax API key.

### Claude Code

```bash
claude mcp add --transport http ziptax https://mcp.zip-tax.com/ --header "X-API-KEY: your_api_key"
```

### Claude Desktop

Add this block to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "ziptax": {
      "type": "http",
      "url": "https://mcp.zip-tax.com/",
      "headers": {
        "X-API-KEY": "your_api_key"
      }
    }
  }
}
```

### Cursor

Go to **Settings → MCP** and add a server:

- **Name:** Ziptax MCP Server
- **Type:** HTTP
- **URL:** `https://mcp.zip-tax.com/`
- **Headers:** `X-API-KEY: your_api_key`

### Devin

Add the Ziptax integration from the MCP Marketplace, or configure it under **Settings → MCP Marketplace → Add Your Own**:

```json
{
  "mcpServers": {
    "ziptax-sales-tax-api": {
      "transport": "shttp",
      "url": "https://mcp.zip-tax.com/",
      "headers": {
        "X-API-KEY": "$ZIPTAX_API_KEY"
      }
    }
  }
}
```

Store the key as a secret named `ZIPTAX_API_KEY` in Devin's Secrets Manager.

### Any Other MCP Client

Any client that supports the Streamable HTTP transport can connect:

- **Type:** HTTP
- **URL:** `https://mcp.zip-tax.com/`
- **Headers:** `X-API-KEY: your_api_key`

## Example Prompts

- "What's the sales tax rate for ZIP code 90210?"
- "What's the combined sales tax rate at 200 Spectrum Center Dr, Irvine, CA?"
- "What's the sales tax rate for Canadian postal code M5V 2T6 in Toronto, Ontario?"
- "What was the sales tax rate in Nashville, TN (37203) in January of this year?"
- "How many API requests do I have left on my Ziptax plan this month?"

## Tools

The server exposes two read-only tools.

| Annotation | `lookup_tax_rate` | `get_account_metrics` |
|---|---|---|
| `title` | Look Up Sales Tax Rate | Get Account Usage Metrics |
| `readOnlyHint` | `true` | `true` |
| `destructiveHint` | `false` | `false` |
| `idempotentHint` | `true` | `true` |
| `openWorldHint` | `false` | `false` |

### `lookup_tax_rate`

Look up sales and use tax rates for a US or Canadian location. Returns rates broken down by jurisdiction (state, county, city, district).

#### Location Requirement

Provide one of:

- `address`: a full street address, for door-level precision. This is the preferred input.
- `lat` **and** `lng`: a geographic point, with the same door-level precision as an address.

The input schema declares this as an `anyOf` constraint. `postalcode` is the least precise location input; see its description below.

#### Parameters

All parameters are optional, subject to the location requirement above, and the schema declares each one as a string. `lat`, `lng`, `historical`, and `sat_item_total` also accept JSON numbers. Every other parameter must be a string. For example, send `postalcode` as `"02134"`, because a number drops the leading zero.

| Parameter | Description |
|---|---|
| `address` | Full street address for door-level geocoded lookup. Preferred input. |
| `lat` | Latitude for coordinate-based lookup. Same door-level precision as an address. Use with `lng`. |
| `lng` | Longitude for coordinate-based lookup. Same door-level precision as an address. Use with `lat`. |
| `postalcode` | US ZIP code (5-digit) or Canadian postal code. Least precise option: returns every rate overlapping the ZIP rather than one authoritative rate, with no adjustment for unincorporated areas. Use only when no address or lat/lng is available. |
| `state` | Two-letter US state or Canadian province code (for example `CA`, `ON`). |
| `city` | City name. |
| `county` | County name. |
| `country_code` | `US` (default) or `CA` for Canada. `CA` requires a Pro or Enterprise plan. |
| `historical` | Historical period in `YYYYMM` format (for example `202601` for January 2026). Lookback is limited to the past 12 months. Requires a Pro or Enterprise plan. |
| `adjustment` | Set to `auto` to enable state-specific unincorporated area adjustments. |
| `taxability_code` | Product taxability code (TIC) for product-specific tax rules. Requires a Pro or Enterprise plan. See [How to find a TIC](https://docs.zip.tax/v-6-0/guides/tutorials/how-to-find-a-tic). |
| `sat_item_total` | Item total for the Tennessee Single Article Tax calculation. |
| `format` | Response format: `json` (default) or `xml`. |

#### Example Call

```json
{
  "name": "lookup_tax_rate",
  "arguments": {
    "address": "200 Spectrum Center Dr, Irvine, CA 92618"
  }
}
```

#### Response

The tool returns the Ziptax API `v60` response as pretty-printed JSON text, or as the API's XML unchanged when `format` is `xml`. It mirrors the [REST API by Address](https://docs.zip.tax/v-6-0/guides/rest-api/by-address) response:

```json
{
  "metadata": {
    "version": "v60",
    "response": {
      "code": 100,
      "name": "RESPONSE_CODE_SUCCESS",
      "message": "Successful API Request.",
      "definition": "http://api.zip-tax.com/request/v60/schema"
    }
  },
  "baseRates": [
    {
      "rate": 0.0725,
      "jurType": "US_STATE_SALES_TAX",
      "jurName": "CA",
      "jurDescription": "US State Sales Tax",
      "jurTaxCode": "06"
    },
    {
      "rate": 0.005,
      "jurType": "US_COUNTY_SALES_TAX",
      "jurName": "ORANGE",
      "jurDescription": "US County Sales Tax",
      "jurTaxCode": "30"
    }
  ],
  "taxSummaries": [
    {
      "rate": 0.0775,
      "taxType": "SALES_TAX",
      "summaryName": "Total Base Sales Tax",
      "displayRates": [
        { "name": "Total Rate", "rate": 0.0775 }
      ]
    }
  ],
  "addressDetail": {
    "normalizedAddress": "200 Spectrum Center Dr, Irvine, CA 92618-5003, United States",
    "incorporated": "true",
    "geoLat": 33.65253,
    "geoLng": -117.74794
  }
}
```

`metadata.response.code` reports the API response code. See [Response Codes](https://docs.zip.tax/v-6-0/guides/reference/response-codes) for the full list.

### `get_account_metrics`

Get usage metrics and quota information for the authenticated Ziptax account. Takes no parameters.

#### Example Call

```json
{
  "name": "get_account_metrics",
  "arguments": {}
}
```

#### Response

The tool returns the Ziptax API `v60` account metrics response as pretty-printed JSON text:

```json
{
  "request_count": 4215,
  "request_limit": 100000,
  "usage_percent": 4.215,
  "is_active": true,
  "message": "Contact support@zip.tax to modify your account"
}
```

| Field | Description |
|---|---|
| `request_count` | Requests made on this key in the current billing period. |
| `request_limit` | Requests included in the plan. `0` means unmetered. |
| `usage_percent` | `request_count / request_limit * 100`. |
| `is_active` | `false` if the key has been disabled. |
| `message` | How to change plan or limits. |

See [Account Metrics](https://docs.zip.tax/v-6-0/guides/reference/account-metrics) for details.

## Error Handling

Errors come back as a tool result with `isError: true` and a text message, not as a JSON-RPC error.

| Message | Cause |
|---|---|
| `Missing API key. Send it in the X-API-KEY header or as a Bearer token in the Authorization header. ...` | The tool call had no API key header. |
| `<parameter> must be a string` or `<parameter> must be a string or number` | A `lookup_tax_rate` parameter was sent with a type it does not accept. |
| `Provide a location: a full street address, or both lat and lng` | `lookup_tax_rate` was called without `address`, a `lat`/`lng` pair, or `postalcode`. |
| `ZipTax API error: API returned status <code>: <body>` | The Ziptax API returned a non-200 HTTP status. |
| `ZipTax API error: API request failed: <detail>` | The Ziptax API could not be reached or did not respond within 15 seconds. |

## Local Development

```bash
# Run the server
go run .

# Run tests
go test -v ./...

# Build and run the Docker image
docker build -t ziptax-mcp .
docker run -p 8080:8080 ziptax-mcp
```

The server listens on port `8080` by default:

- `POST /`: MCP endpoint
- `GET /health`: health check, returns `{"status":"ok"}`

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server listen port |
| `ZIPTAX_API_BASE_URL` | `https://api.zip-tax.com` | Ziptax API base URL |

## Infrastructure

The `cloudformation.yaml` template provisions the complete AWS infrastructure:

- ECR repository for container images
- ECS Fargate cluster and service
- Application Load Balancer with HTTPS
- ACM certificate for `mcp.zip-tax.com`
- Route53 hosted zone and DNS record
- Security groups and IAM roles

### Deployment

CI/CD is handled by GitHub Actions:

1. **test.yml** - Runs on PRs and pushes: linting (`golangci-lint`) and tests (`go test`)
2. **build.yml** - Runs on main: builds Docker image and pushes to ECR
3. **deploy.yml** - Runs after build: updates the ECS service with the new image
4. **pages.yml** - Runs on main when this README, the LICENSE, or the workflow changes, or on manual dispatch: publishes the README as the documentation site at [mcp.zip.tax](https://mcp.zip.tax/)

`build.yml` and `deploy.yml` authenticate to AWS by assuming an IAM role through GitHub OIDC; no workflow uses static AWS keys. The role is defined in `ziptax-terraform` (`github_oidc.tf`) and trusts only this repository's `main` branch. Its ARN is the `AWS_DEPLOY_ROLE_ARN` repository variable.

### Initial Setup

1. Deploy the CloudFormation stack:
   ```bash
   aws cloudformation deploy \
     --template-file cloudformation.yaml \
     --stack-name ziptax-mcp \
     --parameter-overrides ImageUri=<ECR_URI>:latest \
     --capabilities CAPABILITY_NAMED_IAM \
     --region us-east-1
   ```

2. Add NS records from the `mcp.zip-tax.com` hosted zone to the parent `zip-tax.com` zone.

3. Wait for ACM certificate DNS validation to complete.

## License

[MIT](LICENSE)
