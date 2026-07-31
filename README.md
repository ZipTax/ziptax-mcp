# ZipTax MCP Server

An HTTP [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that provides US and Canadian sales tax rate lookups via the [ZipTax API](https://zip-tax.com).

## MCP Endpoint

```
https://mcp.zip-tax.com
```

## Authentication

All requests require a valid ZipTax API key sent as an HTTP header. Two header methods are supported:

```http
X-API-KEY: your-api-key
```

```http
Authorization: Bearer your-api-key
```

Get an API key at [platform.zip.tax](https://platform.zip.tax). Do not pass API keys in the URL; keys in URLs can end up in logs.

## Example Use Cases

Once connected, ask your AI agent things like:

- "What's the sales tax rate for ZIP code 90210?"
- "What's the combined sales tax rate at 200 Spectrum Center Dr, Irvine, CA?"
- "What's the sales tax rate for Canadian postal code M5V 2T6 in Toronto, Ontario?"
- "What was the sales tax rate in Nashville, TN (37203) in January of this year?"
- "How many API requests do I have left on my ZipTax plan this month?"

## Tools

Both tools are read-only lookups (annotated with `readOnlyHint=true`, `destructiveHint=false`, `idempotentHint=true`).

### `lookup_tax_rate`

Look up sales and use tax rates for a US or Canadian location.

**Parameters:**

| Parameter | Description |
|-----------|-------------|
| `postalcode` | US ZIP code (5-digit) or Canadian postal code |
| `address` | Full street address for geocoded lookup |
| `state` | Two-letter US state or Canadian province code |
| `city` | City name |
| `county` | County name |
| `country_code` | `US` (default) or `CA` for Canada |
| `lat` / `lng` | Latitude/longitude for coordinate-based lookup |
| `historical` | Historical period in `YYYYMM` format |
| `adjustment` | Set to `auto` for unincorporated area adjustments |
| `taxability_code` | Product taxability code (TIC) |
| `sat_item_total` | Item total for Tennessee Single Article Tax |
| `format` | Response format: `json` (default) or `xml` |

### `get_account_metrics`

Get account usage metrics and quota information.

## Local Development

```bash
# Run the server
go run .

# Run tests
go test -v ./...

# Build Docker image
docker build -t ziptax-mcp .
docker run -p 8080:8080 ziptax-mcp
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server listen port |
| `ZIPTAX_API_BASE_URL` | `https://api.zip-tax.com` | ZipTax API base URL |

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
