/**
 * @description
 * This is the main entry point for the Poly-Pro Analytics backend service.
 * It is responsible for initializing and starting the application server.
 *
 * Key features:
 * - Configuration Loading: Loads environment variables from a .env.local file.
 * - Database Connection: Establishes and manages a connection to the PostgreSQL database.
 * - Server Initialization: Sets up the Gin web server with all its routes and middleware.
 * - Graceful Shutdown: Handles interrupt signals (like Ctrl+C) to shut down the server gracefully.
 */

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poly-pro/backend/internal/api"
	"github.com/poly-pro/backend/internal/config"
	db "github.com/poly-pro/backend/internal/db"
)

func main() {
	// Initialize a structured logger for better log management.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// ------------------------------------------------------------------
	// Configuration Loading
	// ------------------------------------------------------------------
	// Load application configuration from environment variables.
	// The application will exit if the configuration cannot be loaded.
	cfg, err := config.LoadConfig(".")
	if err != nil {
		logger.Error("cannot load config", "error", err)
		os.Exit(1)
	}
	logger.Info("configuration loaded successfully")

	// ------------------------------------------------------------------
	// Database Connection
	// ------------------------------------------------------------------
	// Create a database connection pool. pgxpool is used for its efficiency
	// and built-in support for connection pooling.
	connPool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("cannot connect to the database", "error", err)
		os.Exit(1)
	}
	// Defer closing the connection pool until the application exits.
	defer connPool.Close()

	// Ping the database to ensure the connection is alive.
	if err := connPool.Ping(context.Background()); err != nil {
		logger.Error("database ping failed", "error", err)
		os.Exit(1)
	}
	logger.Info("database connection established")

	// ------------------------------------------------------------------
	// Server Initialization
	// ------------------------------------------------------------------
	// Create a new instance of our database store using the connection pool.
	// This provides our API handlers with a way to query the database.
	store := db.New(connPool)

	// Create a new server instance, passing in the configuration and database store.
	server := api.NewServer(cfg, store)

	// Create a new HTTP server based on the Gin router.
	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: server.Router,
	}

	// ------------------------------------------------------------------
	// Start Server & Handle Graceful Shutdown
	// ------------------------------------------------------------------
	// Create a channel to receive errors from the server's goroutine.
	serverErrors := make(chan error, 1)

	// Start the server in a separate goroutine so it doesn't block.
	go func() {
		logger.Info("starting server", "address", httpServer.Addr)
		serverErrors <- httpServer.ListenAndServe()
	}()

	// Create a channel to listen for OS interrupt signals.
	shutdownChannel := make(chan os.Signal, 1)
	signal.Notify(shutdownChannel, syscall.SIGINT, syscall.SIGTERM)

	// Block until a server error or a shutdown signal is received.
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	case sig := <-shutdownChannel:
		logger.Info("shutdown signal received", "signal", sig)

		// Create a context with a timeout to allow for graceful shutdown.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Attempt to gracefully shut down the server.
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}

		// Close all server connections and resources.
		if err := server.Close(); err != nil {
			logger.Error("failed to close server resources", "error", err)
		}

		logger.Info("server shutdown complete")
	}

	logger.Info("application has shut down")
}
