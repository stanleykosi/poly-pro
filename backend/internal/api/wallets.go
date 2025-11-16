/**
 * @description
 * This file contains the HTTP handlers for wallet-related API endpoints.
 * It handles wallet creation, retrieval, and management operations.
 *
 * Key features:
 * - Wallet Creation: Allows users to create new wallets or link existing Polymarket wallets
 * - Wallet Retrieval: Allows users to view their wallet information
 * - Authentication: All endpoints require authentication via Clerk JWT
 * - Input Validation: Validates incoming requests before processing
 */

package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/poly-pro/backend/internal/auth"
	"github.com/poly-pro/backend/internal/services"
)

// createWalletRequest defines the structure of the JSON body for wallet creation
type createWalletRequest struct {
	PolymarketFunderAddress string `json:"polymarket_funder_address,omitempty"` // Optional: for linking existing wallet
	PrivateKey              string `json:"private_key,omitempty"`                // Optional: for linking existing wallet
}

/**
 * @description
 * createWallet is a Gin handler that processes a request to create a new wallet.
 * It can either generate a new wallet or link an existing Polymarket proxy wallet.
 *
 * @param c *gin.Context The Gin context for the request.
 *
 * @notes
 * - This handler must be used with the authentication middleware.
 * - If both polymarket_funder_address and private_key are provided, it links an existing wallet.
 * - If neither is provided, it generates a new wallet.
 */
func (server *Server) createWallet(c *gin.Context) {
	// 1. Retrieve the authenticated user's Clerk ID from the context
	clerkUserID, exists := c.Get(string(auth.ClerkUserIDKey))
	if !exists {
		server.logger.Error("clerkUserID not found in context for createWallet")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "User identifier not found in request context"})
		return
	}

	// 2. Parse and validate the incoming JSON request body
	var req createWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.logger.Warn("invalid create wallet request", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid request body: " + err.Error()})
		return
	}

	// 3. Validate that if one field is provided, both must be provided
	if (req.PolymarketFunderAddress != "" && req.PrivateKey == "") ||
		(req.PolymarketFunderAddress == "" && req.PrivateKey != "") {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "Both polymarket_funder_address and private_key must be provided together, or neither for a new wallet",
		})
		return
	}

	server.logger.Info("processing create wallet request",
		"clerk_user_id", clerkUserID,
		"has_funder_address", req.PolymarketFunderAddress != "",
		"has_private_key", req.PrivateKey != "")

	// 4. Call the WalletService to create the wallet
	params := services.CreateWalletParams{
		UserID:                  clerkUserID.(string),
		PolymarketFunderAddress: req.PolymarketFunderAddress,
		PrivateKey:              req.PrivateKey,
	}

	wallet, err := server.walletService.CreateWallet(c.Request.Context(), params)
	if err != nil {
		server.logger.Error("failed to create wallet", "error", err, "user_id", clerkUserID)
		
		// Return appropriate error messages
		errMsg := err.Error()
		if strings.Contains(errMsg, "user not found") {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": "User not found in database. Please ensure you have completed sign-up. If this issue persists, contact support.",
				"code":    "USER_NOT_FOUND",
			})
			return
		}
		if errMsg == "user already has an active wallet" {
			c.JSON(http.StatusConflict, gin.H{"status": "error", "message": "User already has an active wallet"})
			return
		}
		if errMsg == "invalid funder address format" || errMsg == "invalid private key" || errMsg == "private key does not match the provided address" {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": errMsg})
			return
		}
		
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to create wallet"})
		return
	}

	// 5. Return the wallet information (without private key)
	server.logger.Info("wallet created successfully", "user_id", clerkUserID, "wallet_id", wallet.ID)
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Wallet created successfully",
		"data":    wallet,
	})
}

/**
 * @description
 * getWallets is a Gin handler that retrieves all wallets for the authenticated user.
 *
 * @param c *gin.Context The Gin context for the request.
 *
 * @notes
 * - This handler must be used with the authentication middleware.
 * - Returns an empty array if the user has no wallets.
 */
func (server *Server) getWallets(c *gin.Context) {
	// 1. Retrieve the authenticated user's Clerk ID from the context
	clerkUserID, exists := c.Get(string(auth.ClerkUserIDKey))
	if !exists {
		server.logger.Error("clerkUserID not found in context for getWallets")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "User identifier not found in request context"})
		return
	}

	// 2. Call the WalletService to get user's wallets
	wallets, err := server.walletService.GetUserWallets(c.Request.Context(), clerkUserID.(string))
	if err != nil {
		server.logger.Error("failed to get wallets", "error", err, "user_id", clerkUserID)
		
		errMsg := err.Error()
		if strings.Contains(errMsg, "user not found") {
			// User doesn't exist - return empty array instead of error
			// This allows the frontend to show the wallet creation modal
			server.logger.Warn("user not found when getting wallets, returning empty array", "clerk_id", clerkUserID)
			c.JSON(http.StatusOK, gin.H{
				"status": "success",
				"data":   []services.WalletInfo{},
				"meta": gin.H{
					"count": 0,
				},
			})
			return
		}
		
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to retrieve wallets"})
		return
	}

	// 3. Return the wallets
	server.logger.Info("wallets retrieved successfully", "user_id", clerkUserID, "count", len(wallets))
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   wallets,
		"meta": gin.H{
			"count": len(wallets),
		},
	})
}

