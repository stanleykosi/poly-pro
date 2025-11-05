/**
 * @description
 * This is the main entry point for the Poly-Pro Analytics Remote Signing service.
 * It is responsible for initializing and starting the gRPC server that listens
 * for signing requests.
 *
 * Key features:
 * - Configuration Loading: Loads environment variables (port, dummy key).
 * - Dependency Initialization: Sets up the logger, vault, crypto signer, and gRPC server.
 * - gRPC Server Startup: Creates a TCP listener and starts the gRPC server.
 * - Graceful Shutdown: Listens for OS interrupt signals (e.g., Ctrl+C) to shut
 *   down the gRPC server gracefully, allowing active requests to finish.
 */
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/poly-pro/remote-signer/internal/config"
	"github.com/poly-pro/remote-signer/internal/crypto"
	"github.com/poly-pro/remote-signer/internal/server"
	"github.com/poly-pro/remote-signer/internal/vault"
	"github.com/poly-pro/remote-signer/proto"
	"github.com/soheilhy/cmux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	// Initialize a structured logger for consistent, machine-readable logs.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// ------------------------------------------------------------------
	// Configuration Loading
	// ------------------------------------------------------------------
	cfg, err := config.LoadConfig(".")
	if err != nil {
		logger.Error("cannot load config", "error", err)
		os.Exit(1)
	}
	logger.Info("configuration loaded successfully")

	// ------------------------------------------------------------------
	// Dependency Initialization
	// ------------------------------------------------------------------
	// Initialize the vault (using the mock implementation for now).
	mockVault, err := vault.NewMockVault(cfg.DummyPrivateKey, logger)
	if err != nil {
		logger.Error("failed to initialize mock vault", "error", err)
		os.Exit(1)
	}

	// Initialize the crypto signer.
	signer := crypto.NewSigner(logger)

	// Initialize the gRPC server implementation.
	grpcServer := server.NewGRPCServer(logger, mockVault, signer)

	// ------------------------------------------------------------------
	// Server Setup with Connection Multiplexing (HTTP + gRPC)
	// ------------------------------------------------------------------
	// Railway tries to do HTTP health checks. We need to provide an HTTP endpoint
	// that responds to health check requests, while also serving gRPC on the same port.
	// We'll use cmux (Connection Multiplexer) to route HTTP and gRPC on the same port.
	
	// Log the port being used for debugging
	logger.Info("configuring server", "port", cfg.Port)

	// Create a TCP listener on the configured port.
	// Railway injects the PORT environment variable, so we listen on all interfaces (0.0.0.0)
	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", cfg.Port))
	if err != nil {
		logger.Error("failed to listen on port", "port", cfg.Port, "error", err)
		os.Exit(1)
	}

	logger.Info("TCP listener created", "address", lis.Addr().String())

	// Create a connection multiplexer to handle both HTTP and gRPC on the same port
	mux := cmux.New(lis)

	// Match HTTP/1.x requests (for health checks)
	httpL := mux.Match(cmux.HTTP1Fast())
	
	// Match HTTP/2 requests (for gRPC)
	// gRPC uses HTTP/2, so any HTTP/2 connection is likely gRPC
	grpcL := mux.Match(cmux.HTTP2())

	// ------------------------------------------------------------------
	// HTTP Health Check Server
	// ------------------------------------------------------------------
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","service":"remote-signer","port":"%s"}`, cfg.Port)
	})
	
	httpServer := &http.Server{
		Handler:      httpMux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start HTTP server in a goroutine
	go func() {
		logger.Info("HTTP health check server starting", "port", cfg.Port)
		if err := httpServer.Serve(httpL); err != nil {
			logger.Error("HTTP server failed to serve", "error", err)
		}
	}()

	// ------------------------------------------------------------------
	// gRPC Server Setup
	// ------------------------------------------------------------------
	// Create a new gRPC server instance.
	s := grpc.NewServer()

	// Register our Signer service implementation with the gRPC server.
	proto.RegisterSignerServer(s, grpcServer)

	// Register reflection service on gRPC server. This is useful for tools
	// like grpcurl to discover and interact with the server.
	reflection.Register(s)

	// Start the gRPC server in a separate goroutine
	go func() {
		logger.Info("gRPC server starting", "address", lis.Addr().String(), "port", cfg.Port)
		if err := s.Serve(grpcL); err != nil {
			logger.Error("gRPC server failed to serve", "error", err)
			os.Exit(1)
		}
	}()

	// Start the connection multiplexer
	go func() {
		logger.Info("connection multiplexer starting", "port", cfg.Port)
		if err := mux.Serve(); err != nil {
			logger.Error("connection multiplexer failed", "error", err)
			os.Exit(1)
		}
	}()

	logger.Info("server is ready and listening", "port", cfg.Port, "address", lis.Addr().String())
	logger.Info("HTTP health check available at /health")
	logger.Info("gRPC service available for signing requests")

	// ------------------------------------------------------------------
	// Handle Graceful Shutdown
	// ------------------------------------------------------------------
	// Create a channel to listen for OS interrupt signals.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until a shutdown signal is received.
	<-quit
	logger.Info("shutdown signal received, initiating graceful shutdown")

	// Stop the connection multiplexer (this will stop accepting new connections)
	mux.Close()

	// Create a context with timeout for graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Gracefully shutdown the HTTP server
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", "error", err)
	}

	// Gracefully stop the gRPC server. This will stop accepting new connections
	// and wait for existing RPCs to finish.
	s.GracefulStop()

	logger.Info("all servers shut down gracefully")
}
