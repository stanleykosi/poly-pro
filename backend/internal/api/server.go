/**
 * @description
 * This file sets up the main HTTP server for the backend service using the Gin framework.
 * It is responsible for initializing the router, setting up middleware, and defining API routes.
 *
 * Key features:
 * - Gin Router: Utilizes Gin for high-performance HTTP routing.
 * - Middleware: Includes default middleware for logging and panic recovery.
 * - Route Grouping: Organizes API routes under a versioned `/api/v1` group.
 * - Dependency Injection: The server holds dependencies like configuration and database access,
 *   which can be passed to HTTP handlers.
 */

package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/poly-pro/backend/internal/auth"
	"github.com/poly-pro/backend/internal/config"
	db "github.com/poly-pro/backend/internal/db"
	"github.com/poly-pro/backend/internal/polymarket"
	"github.com/poly-pro/backend/internal/services"
	"github.com/poly-pro/backend/internal/websocket"
	"github.com/redis/go-redis/v9"
)

// Server serves HTTP requests for the Poly-Pro Analytics backend service.
type Server struct {
	config              config.Config
	store               db.Querier
	Router              *gin.Engine
	logger              *slog.Logger
	userService         *services.UserService
	polymarketService   *services.PolymarketService
	marketStreamService *services.MarketStreamService
	signerClient        services.SignerClient
	hub                 *websocket.Hub
	redisClient         *redis.Client
	gammaClient         *polymarket.GammaAPIClient
}

/**
 * @description
 * NewServer creates a new HTTP server and sets up all the necessary routing and services.
 *
 * @param ctx The root context for the server, used for graceful shutdown.
 * @param config The application configuration.
 * @param store The database querier for database operations.
 * @param redisClient The client for connecting to Redis.
 * @returns A pointer to a new Server instance.
 *
 * @notes
 * - This function encapsulates the entire setup of the Gin router and related services,
 *   making it easy to instantiate the server from the main application entry point.
 */
func NewServer(ctx context.Context, config config.Config, store db.Querier, redisClient *redis.Client) *Server {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Initialize gRPC client for the remote signer
	signerClient, err := services.NewSignerClient(config.RemoteSignerAddress, logger)
	if err != nil {
		logger.Error("failed to create signer client", "error", err)
		os.Exit(1)
	}

	// Initialize Gamma API client
	gammaClient := polymarket.NewGammaAPIClient(config.GammaAPIURL, logger)

	// Initialize services
	userService := services.NewUserService(store, logger)
	polymarketService := services.NewPolymarketService(store, logger, signerClient, config)
	marketStreamService := services.NewMarketStreamService(ctx, logger, redisClient, config, store, gammaClient)

	// Initialize the WebSocket Hub
	hub := websocket.NewHub(ctx, logger, redisClient)

	// Initialize a new Server instance
	server := &Server{
		config:              config,
		store:               store,
		logger:              logger,
		userService:         userService,
		polymarketService:   polymarketService,
		marketStreamService: marketStreamService,
		signerClient:        signerClient,
		hub:                 hub,
		redisClient:         redisClient,
		gammaClient:         gammaClient,
	}

	// Initialize the Gin router with default middleware (logger and recovery)
	router := gin.Default()

	// Add CORS middleware
	router.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		// Allow requests from localhost (development) and production frontend
		// List of allowed origins
		allowedOrigins := []string{
			"http://localhost:3000",
			"http://localhost:3001",
			"https://poly-pro-production.up.railway.app",
		}
		
		// Check if origin is in allowed list
		allowed := false
		if origin != "" {
			for _, allowedOrigin := range allowedOrigins {
				if origin == allowedOrigin {
					allowed = true
					break
				}
			}
		}
		
		// Set CORS headers
		if allowed && origin != "" {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		} else if origin == "" {
			// No origin header (e.g., same-origin request or Postman)
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			// Origin not in allowed list - still set headers but don't set Access-Control-Allow-Origin
			// This allows the browser to show the error clearly
		}
		
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// ------------------------------------------------------------------
	// Route Definitions
	// ------------------------------------------------------------------
	// A simple health check endpoint to confirm the server is running.
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "success",
			"message": "Poly-Pro Analytics Backend is healthy and running!",
		})
	})

	// Group all API routes under `/api/v1` for versioning.
	v1 := router.Group("/api/v1")
	{
		// --- Public Routes ---
		// WebSocket route - does not require JWT auth for connection,
		// but could be implemented to require it.
		v1.GET("/ws", server.serveWs)

		// Endpoint to get historical OHLCV data for a market. Public data for charting.
		// This route must be registered BEFORE /markets/:id to avoid route conflicts.
		v1.GET("/markets/:id/history", server.getMarketHistory)

		// Endpoint to get static details for a market. This is public data.
		v1.GET("/markets/:id", server.getMarketDetails)

		// Webhook routes are public but should have their own verification logic.
		webhookGroup := v1.Group("/webhooks")
		{
			// Endpoint for receiving webhooks from Clerk, specifically for user creation events.
			webhookGroup.POST("/clerk", server.handleCreateUserWebhook)
		}

		// --- Protected Routes ---
		// Initialize the authentication middleware.
		authMiddleware, err := auth.NewAuthMiddleware(config.ClerkIssuerURL)
		if err != nil {
			logger.Error("failed to create auth middleware", "error", err)
			os.Exit(1) // Exit if middleware can't be created, as it's critical.
		}

		// Create a new group for routes that require authentication.
		authGroup := v1.Group("/")
		authGroup.Use(authMiddleware)
		{
			// User-related protected routes
			userRoutes := authGroup.Group("/users")
			{
				// Endpoint to get the current authenticated user's profile.
				userRoutes.GET("/me", server.getMe)
			}

			// Order-related protected routes
			orderRoutes := authGroup.Group("/orders")
			{
				// Endpoint to place a new order.
				orderRoutes.POST("/", server.placeOrder)
			}
		}
	}

	// Attach the configured router to our server instance
	server.Router = router

	// Start the WebSocket hub and market stream service in the background.
	go server.hub.Run()
	go server.marketStreamService.RunStream()

	return server
}

/**
 * @description
 * Close closes all connections and resources held by the server.
 * This should be called during graceful shutdown of the application.
 */
func (s *Server) Close() error {
	if s.signerClient != nil {
		if err := s.signerClient.Close(); err != nil {
			s.logger.Error("failed to close signer client", "error", err)
			return err
		}
	}
	if s.redisClient != nil {
		if err := s.redisClient.Close(); err != nil {
			s.logger.Error("failed to close redis client", "error", err)
			return err
		}
	}
	return nil
}

