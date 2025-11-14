/**
 * @description
 * This file contains the HTTP handlers for order-related API endpoints.
 * It is responsible for handling requests to place, cancel, and view orders.
 *
 * Key features:
 * - Input Validation: The `placeOrder` handler validates the incoming request body
 *   to ensure all required fields for placing an order are present and valid.
 * - Authentication: Relies on the authentication middleware to provide the
 *   authenticated user's ID, ensuring that orders are placed on behalf of the correct user.
 * - Service Delegation: Delegates the core business logic of creating and signing
 *   the order to the `PolymarketService`, adhering to the principle of separation of concerns.
 * - Standardized Responses: Returns structured JSON responses for both success and error cases.
 */

package api

import (
	"log/slog"
	"math/big"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/poly-pro/backend/internal/auth"
	"github.com/poly-pro/backend/internal/services"
)

// placeOrderRequest defines the structure of the JSON body expected
// for a request to the `POST /api/v1/orders` endpoint.
type placeOrderRequest struct {
	MarketID string  `json:"marketId" binding:"required"`
	TokenID  string  `json:"tokenId" binding:"required"`
	Price    float64 `json:"price" binding:"required,gt=0,lt=1"`
	Size     float64 `json:"size" binding:"required,gt=0"`
	Side     string  `json:"side" binding:"required,oneof=BUY SELL"`
}

/**
 * @description
 * placeOrder is a Gin handler that processes a request to create and sign a new order.
 *
 * @param c *gin.Context The Gin context for the request.
 *
 * @notes
 * - This handler must be used with the authentication middleware.
 * - It parses the order details, validates them, and then calls the PolymarketService
 *   to handle the EIP-712 signing workflow.
 * - For the MVP, this endpoint returns the signed order but does not yet submit it to
 *   the Polymarket API.
 */
func (server *Server) placeOrder(c *gin.Context) {
	// 1. Retrieve the authenticated user's Clerk ID from the context.
	clerkUserID, exists := c.Get(string(auth.ClerkUserIDKey))
	if !exists {
		server.logger.Error("clerkUserID not found in context for placeOrder")
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "User identifier not found in request context"})
		return
	}

	// 2. Parse and validate the incoming JSON request body.
	var req placeOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.logger.Warn("invalid place order request", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid request body: " + err.Error()})
		return
	}

	// 3. Convert TokenID from string to big.Int
	tokenID, ok := new(big.Int).SetString(req.TokenID, 10)
	if !ok {
		server.logger.Warn("invalid token id format", "token_id", req.TokenID)
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "message": "Invalid tokenId format"})
		return
	}

	server.logger.Info("processing place order request",
		"clerk_user_id", clerkUserID,
		"market_id", req.MarketID,
		"token_id", req.TokenID,
		"price", req.Price,
		"size", req.Size,
		"side", req.Side,
	)

	// 4. Call the PolymarketService to create and sign the order.
	// For Polymarket proxy wallets (email login), the signature type is 1.
	params := services.PlaceOrderParams{
		UserID:        clerkUserID.(string),
		MarketID:      req.MarketID,
		TokenID:       tokenID,
		Price:         req.Price,
		Size:          req.Size,
		Side:          req.Side,
		SignatureType: 1, // POLY_PROXY for email-based accounts
	}

	signedOrder, dbOrder, err := server.polymarketService.CreateAndSignOrder(c.Request.Context(), params)
	if err != nil {
		server.logger.Error("failed to create and sign order", "error", err, "user_id", clerkUserID)
		// Here you could inspect the error to return a more specific status code
		// e.g., if the error is from the signer client, it might be a 503 Service Unavailable.
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "message": "Failed to process order"})
		return
	}

	// 5. Return the signed order and database order in the response.
	server.logger.Info("order successfully created and signed", 
		"user_id", clerkUserID, 
		"order_id", dbOrder.ID,
		slog.Any("signed_order", signedOrder))
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Order placed successfully",
		"data": gin.H{
			"order":       dbOrder,
			"signed_order": signedOrder,
		},
	})
}

