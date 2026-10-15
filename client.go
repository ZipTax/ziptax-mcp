package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// ZipTaxClient handles HTTP communication with the ZipTax REST API.
type ZipTaxClient struct {
	baseURL    string
	httpClient *http.Client
}

// APIStatusError is returned when the ZipTax API answers with a status
// other than 200. Its message reaches MCP clients in tool error results,
// so its format is kept stable.
type APIStatusError struct {
	StatusCode int
	Body       string
}

func (e *APIStatusError) Error() string {
	return fmt.Sprintf("API returned status %d: %s", e.StatusCode, e.Body)
}

// ResponseReadError is returned when the ZipTax API answered but its
// response body could not be read, for example because the connection
// closed early or timed out. It keeps the status the API answered with.
type ResponseReadError struct {
	StatusCode int
	Err        error
}

func (e *ResponseReadError) Error() string {
	return "reading response body: " + e.Err.Error()
}

func (e *ResponseReadError) Unwrap() error { return e.Err }

// APITransportError is returned when the ZipTax API could not be reached
// or did not answer in time. Its message reaches MCP clients in tool error
// results, so it names only the kind of failure. net/http's *url.Error
// quotes the request URL, whose query carries the raw API key and, for a
// lookup, the caller's address, so that layer is dropped and only its
// cause is kept, for errors.As.
type APITransportError struct {
	err error
}

// newAPITransportError wraps an error from http.Client.Do, which is always
// a *url.Error.
func newAPITransportError(err error) *APITransportError {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	return &APITransportError{err: err}
}

func (e *APITransportError) Error() string {
	var netErr net.Error
	if errors.As(e.err, &netErr) && netErr.Timeout() {
		return "API request failed: timeout"
	}
	return "API request failed: connection failed"
}

func (e *APITransportError) Unwrap() error { return e.err }

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
		return nil, newAPITransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ResponseReadError{StatusCode: resp.StatusCode, Err: err}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &APIStatusError{StatusCode: resp.StatusCode, Body: string(body)}
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
		return nil, newAPITransportError(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ResponseReadError{StatusCode: resp.StatusCode, Err: err}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &APIStatusError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	return json.RawMessage(body), nil
}
