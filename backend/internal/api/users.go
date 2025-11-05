/**
 * @description
 * This file contains the HTTP handlers for user-related API endpoints.
 * It handles requests for fetching user data, such as the currently authenticated user's profile.
 *
 * Key features:
 * - Protected Routes: Handlers in this file are intended to be used with the authentication middleware.
 * - Service Layer Interaction: It delegates the business logic of retrieving user data to the UserService.
 * - Context-aware: It retrieves the authenticated user's identity from the Gin context, which is populated
 *   by the authentication middleware.
 */
package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/poly-pro/backend/internal/auth"
)

/**
 * @description
 * getMe is a Gin handler that retrieves the profile of the currently authenticated user.
 *
 * @param c *gin.Context The Gin context for the request.
 *
 * @notes
 * - This handler MUST be used behind the authentication middleware, as it relies on the
 *   Clerk User ID being present in the context.
 * - It fetches the user record from the database using the Clerk ID.
 * - If the user is not found in the database (which would be an inconsistent state, as Clerk
 *   should have created them via webhook), it returns a 404 Not Found error.
 */
func (server *Server) getMe(c *gin.Context) {
	// 1. Retrieve the Clerk User ID from the context.
	// The auth middleware guarantees this value exists if the handler is reached.
	clerkUserID, exists := c.Get(string(auth.ClerkUserIDKey))
	if !exists {
		// This should theoretically never happen if the middleware is applied correctly.
		server.logger.Error("clerkUserID not found in context after auth middleware")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "User identifier not found in request context"})
		return
	}

	// 2. Use the user service to fetch the user from the database.
	user, err := server.userService.GetUserByClerkID(c.Request.Context(), clerkUserID.(string))
	if err != nil {
		// Handle the case where the user is authenticated but not found in our DB.
		if errors.Is(err, pgx.ErrNoRows) {
			server.logger.Warn("user authenticated but not found in DB", "clerk_id", clerkUserID)
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Authenticated user not found in the system"})
			return
		}
		// Handle other potential database errors.
		server.logger.Error("failed to get user by clerk ID", "error", err, "clerk_id", clerkUserID)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to retrieve user profile"})
		return
	}

	// 3. Return the user's data.
	c.JSON(http.StatusOK, gin.H{"status": "success", "data": user})
}

