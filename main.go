package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// serverInstructions is the client-facing usage text advertised by the
// MCP server. It must only reference header-based authentication.
const serverInstructions = "ZipTax MCP Server provides US and Canadian sales tax rate lookups. " +
	"Authenticate by sending your ZipTax API key in the X-API-KEY HTTP header " +
	"or as a Bearer token in the Authorization header. " +
	"Get an API key at https://platform.zip.tax"

// lastKeyParamWarnUnix throttles the ?key= deprecation warning to at most
// once per minute, so unauthenticated request spam cannot flood the logs.
var lastKeyParamWarnUnix atomic.Int64

// apiKeyFromQuery is HTTP middleware that copies the "key" URL query
// parameter into the X-API-KEY request header when the header is not
// already set. This is an undocumented fallback kept for backward
// compatibility with existing clients; header-based authentication
// (X-API-KEY or Authorization) is the supported method.
func apiKeyFromQuery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key := r.URL.Query().Get("key"); key != "" && r.Header.Get("X-API-KEY") == "" {
			now := time.Now().Unix()
			if last := lastKeyParamWarnUnix.Load(); now-last >= 60 && lastKeyParamWarnUnix.CompareAndSwap(last, now) {
				log.Printf("deprecated: API key received via ?key= URL parameter; migrate to the X-API-KEY or Authorization header")
			}
			r.Header.Set("X-API-KEY", key)
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	apiBaseURL := os.Getenv("ZIPTAX_API_BASE_URL")
	if apiBaseURL == "" {
		apiBaseURL = "https://api.zip-tax.com"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	client := NewZipTaxClient(apiBaseURL)

	mcpServer := server.NewMCPServer(
		"ZipTax Sales Tax API",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
		server.WithInstructions(serverInstructions),
	)

	RegisterTools(mcpServer, client)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	})

	httpServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithEndpointPath("/"),
		server.WithStateLess(true),
	)
	// Wrap the MCP handler so that a ?key= URL parameter is promoted to
	// the X-API-KEY header (backward-compatibility fallback only).
	mux.Handle("/", apiKeyFromQuery(httpServer))

	addr := fmt.Sprintf("0.0.0.0:%s", port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("ZipTax MCP Server starting on %s", addr)
		log.Printf("MCP endpoint: %s/", addr)
		log.Printf("Health endpoint: %s/health", addr)
		log.Printf("Proxying to: %s", apiBaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-done
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}
