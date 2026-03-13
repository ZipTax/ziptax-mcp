package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewZipTaxClient(t *testing.T) {
	client := NewZipTaxClient("https://api.zip-tax.com")
	if client.baseURL != "https://api.zip-tax.com" {
		t.Errorf("expected baseURL to be https://api.zip-tax.com, got %s", client.baseURL)
	}
	if client.httpClient == nil {
		t.Error("expected httpClient to be non-nil")
	}
}

func TestLookupTax(t *testing.T) {
	expected := map[string]interface{}{
		"rCode": float64(100),
		"results": []interface{}{
			map[string]interface{}{
				"geoPostalCode": "90210",
				"geoCity":       "BEVERLY HILLS",
				"geoState":      "CA",
				"taxSales":      0.0925,
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/request/v60" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("expected key=test-key, got %s", r.URL.Query().Get("key"))
		}
		if r.URL.Query().Get("postalcode") != "90210" {
			t.Errorf("expected postalcode=90210, got %s", r.URL.Query().Get("postalcode"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewZipTaxClient(server.URL)
	result, err := client.LookupTax("test-key", "v60", map[string]string{
		"postalcode": "90210",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if parsed["rCode"] != float64(100) {
		t.Errorf("expected rCode=100, got %v", parsed["rCode"])
	}
}

func TestLookupTaxError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewZipTaxClient(server.URL)
	_, err := client.LookupTax("test-key", "v60", map[string]string{
		"postalcode": "90210",
	})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestGetAccountMetrics(t *testing.T) {
	expected := map[string]interface{}{
		"requestCount": float64(42),
		"requestLimit": float64(1000),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/account/v60/metrics" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("expected key=test-key, got %s", r.URL.Query().Get("key"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewZipTaxClient(server.URL)
	result, err := client.GetAccountMetrics("test-key", "v60")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if parsed["requestCount"] != float64(42) {
		t.Errorf("expected requestCount=42, got %v", parsed["requestCount"])
	}
}
