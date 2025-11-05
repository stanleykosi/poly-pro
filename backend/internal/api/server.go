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
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/poly-pro/backend/internal/auth"
	"github.com/poly-pro/backend/internal/config"
	db "github.com/poly-pro/backend/internal/db"
	"github.com/poly-pro/backend/internal/services"
)

// Server serves HTTP requests for the Poly-Pro Analytics backend service.
type Server struct {
	config      config.Config
	store       db.Querier
	Router      *gin.Engine
	logger      *slog.Logger
	userService *services.UserService
}

/**
 * @description
 * NewServer creates a new HTTP server and sets up all the necessary routing.
 *
 * @param config The application configuration.
 * @param store The database querier for database operations.
 * @returns A pointer to a new Server instance.
 *
 * @notes
 * - This function encapsulates the entire setup of the Gin router, making it easy
 *   to instantiate the server from the main application entry point.
 */
func NewServer(config config.Config, store db.Querier) *Server {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Initialize services
	userService := services.NewUserService(store, logger)

	// Initialize a new Server instance
	server := &Server{
		config:      config,
		store:       store,
		logger:      logger,
		userService: userService,
	}

	// Initialize the Gin router with default middleware (logger and recovery)
	router := gin.Default()

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
		}
	}

	// Attach the configured router to our server instance
	server.Router = router
	return server
}

