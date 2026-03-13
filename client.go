package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ZipTaxClient handles HTTP communication with the ZipTax REST API.
type ZipTaxClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewZipTaxClient creates a new ZipTax API client.
func NewZipTaxClient(baseURL string) *ZipTaxClient {
	return &ZipTaxClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// LookupTax calls the ZipTax API tax lookup endpoint.
// The apiKey is passed as a query parameter named "key" per the ZipTax API convention.
func (c *ZipTaxClient) LookupTax(apiKey, version string, params map[string]string) (json.RawMessage, error) {
	u, err := url.Parse(fmt.Sprintf("%s/request/%s", c.baseURL, version))
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	q := u.Query()
	q.Set("key", apiKey)
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()

	resp, err := c.httpClient.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return json.RawMessage(body), nil
}

// GetAccountMetrics calls the ZipTax account metrics endpoint.
func (c *ZipTaxClient) GetAccountMetrics(apiKey, version string) (json.RawMessage, error) {
	u, err := url.Parse(fmt.Sprintf("%s/account/%s/metrics", c.baseURL, version))
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	q := u.Query()
	q.Set("key", apiKey)
	u.RawQuery = q.Encode()

	resp, err := c.httpClient.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return json.RawMessage(body), nil
}
