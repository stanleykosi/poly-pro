/**
 * @description
 * This file contains the HTTP handler for fetching market-related data.
 * It integrates with Polymarket's Gamma API to fetch real market data.
 *
 * Key features:
 * - Market Data Endpoint: Exposes an endpoint to retrieve initial market details.
 * - Gamma API Integration: Fetches real market data from Polymarket's Gamma API.
 * - RESTful Design: Follows REST principles by using a GET request with a path parameter
 *   to identify the market resource.
 */

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MarketDetails represents the basic details of a market.
// This structure will be sent to the frontend upon initial page load.
type MarketDetails struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	ResolutionSource string `json:"resolution_source"`
}

/**
 * @function getMarketDetails
 * @description A Gin handler that fetches and returns market details from Polymarket's Gamma API.
 * It uses the 'id' parameter from the URL to identify the market (condition ID).
 *
 * @param c *gin.Context The Gin context for the request.
 *
 * @notes
 * - This handler fetches real market data from Polymarket's Gamma API.
 * - The 'id' parameter should be a condition ID (hex string starting with 0x).
 * - If no matching market is found, it returns a 404 Not Found error.
 */
func (server *Server) getMarketDetails(c *gin.Context) {
	marketID := c.Param("id")

	server.logger.Info("fetching details for market", "market_id", marketID)

	// Fetch market data from Gamma API
	gammaMarket, err := server.gammaClient.GetMarketByConditionID(c.Request.Context(), marketID)
	if err != nil {
		server.logger.Warn("failed to fetch market from Gamma API", "error", err, "market_id", marketID)
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Market not found"})
		return
	}

	// Convert Gamma API response to our MarketDetails format
	marketDetails := MarketDetails{
		ID:               gammaMarket.ConditionID,
		Title:            gammaMarket.Question,
		Description:      gammaMarket.Question, // Gamma API doesn't have a separate description field
		ResolutionSource: gammaMarket.ResolutionSource,
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "data": marketDetails})
}

