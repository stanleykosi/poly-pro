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

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/poly-pro/backend/internal/api"
	"github.com/poly-pro/backend/internal/config"
	db "github.com/poly-pro/backend/internal/db"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Set Gin to release mode for production (unless explicitly set to debug)
	if os.Getenv("GIN_MODE") == "" {
		gin.SetMode(gin.ReleaseMode)
	}

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
	// Parse the database URL to configure the connection pool
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		logger.Error("cannot parse database URL", "error", err)
		os.Exit(1)
	}
	
	// Configure to use simple protocol to avoid "prepared statement already exists" errors
	// This can happen when multiple goroutines use the same connection concurrently
	// Simple protocol doesn't use prepared statements, avoiding the collision issue
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	
	// Create a database connection pool. pgxpool is used for its efficiency
	// and built-in support for connection pooling.
	connPool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
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
	// Redis Connection
	// ------------------------------------------------------------------
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("cannot parse redis URL", "error", err)
		os.Exit(1)
	}
	redisClient := redis.NewClient(redisOpts)
	if _, err := redisClient.Ping(context.Background()).Result(); err != nil {
		logger.Error("redis ping failed", "error", err)
		os.Exit(1)
	}
	logger.Info("redis connection established")
	defer redisClient.Close()

	// Create a root context that can be cancelled to trigger shutdown of background services
	rootCtx, cancelRootCtx := context.WithCancel(context.Background())
	defer cancelRootCtx()

	// ------------------------------------------------------------------
	// Server Initialization
	// ------------------------------------------------------------------
	// Create a new instance of our database store using the connection pool.
	// This provides our API handlers with a way to query the database.
	store := db.New(connPool)

	// Create a new server instance, passing in the context, configuration, database store, and redis client.
	server := api.NewServer(rootCtx, cfg, store, redisClient)

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

		// Cancel the root context to signal background goroutines (like the Hub) to stop.
		cancelRootCtx()

		// Create a context with a timeout to allow for graceful shutdown.
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()

		// Attempt to gracefully shut down the server.
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful http server shutdown failed", "error", err)
			os.Exit(1)
		}

		// Close all server connections and resources.
		if err := server.Close(); err != nil {
			logger.Error("failed to close server resources", "error", err)
		}
	}

	logger.Info("application has shut down")
}
