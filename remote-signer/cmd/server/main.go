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
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/poly-pro/remote-signer/internal/config"
	"github.com/poly-pro/remote-signer/internal/crypto"
	"github.com/poly-pro/remote-signer/internal/server"
	"github.com/poly-pro/remote-signer/internal/vault"
	"github.com/poly-pro/remote-signer/proto"
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
	// gRPC Server Setup and Startup
	// ------------------------------------------------------------------
	// Create a TCP listener on the configured port.
	// Railway injects the PORT environment variable, so we listen on all interfaces (0.0.0.0)
	lis, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", cfg.Port))
	if err != nil {
		logger.Error("failed to listen on port", "port", cfg.Port, "error", err)
		os.Exit(1)
	}

	// Create a new gRPC server instance.
	s := grpc.NewServer()

	// Register our Signer service implementation with the gRPC server.
	proto.RegisterSignerServer(s, grpcServer)

	// Register reflection service on gRPC server. This is useful for tools
	// like grpcurl to discover and interact with the server.
	reflection.Register(s)

	// Start the server in a separate goroutine so it doesn't block.
	go func() {
		logger.Info("gRPC server starting", "address", lis.Addr().String())
		if err := s.Serve(lis); err != nil {
			logger.Error("gRPC server failed to serve", "error", err)
			os.Exit(1)
		}
	}()

	// ------------------------------------------------------------------
	// Handle Graceful Shutdown
	// ------------------------------------------------------------------
	// Create a channel to listen for OS interrupt signals.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until a shutdown signal is received.
	<-quit
	logger.Info("shutdown signal received, initiating graceful shutdown")

	// Gracefully stop the gRPC server. This will stop accepting new connections
	// and wait for existing RPCs to finish.
	s.GracefulStop()

	logger.Info("gRPC server shut down gracefully")
}
