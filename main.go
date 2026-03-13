package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

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
		server.WithInstructions("ZipTax MCP Server provides US and Canadian sales tax rate lookups. "+
			"Authenticate by sending your ZipTax API key in the X-API-KEY HTTP header. "+
			"Get an API key at https://platform.zip.tax"),
	)

	RegisterTools(mcpServer, client)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})

	httpServer := server.NewStreamableHTTPServer(mcpServer,
		server.WithEndpointPath("/"),
		server.WithStateLess(true),
	)
	mux.Handle("/", httpServer)

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
