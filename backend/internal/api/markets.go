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
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/poly-pro/backend/internal/polymarket"
)

// MarketDetails represents the basic details of a market.
// This structure will be sent to the frontend upon initial page load.
type MarketDetails struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	ResolutionSource string `json:"resolution_source"`
}

// MarketListItem represents a simplified market entry for listing pages.
type MarketListItem struct {
	ID               string  `json:"id"`
	Title            string  `json:"title"`
	Description      string  `json:"description"`
	ResolutionSource string  `json:"resolution_source"`
	Slug             string  `json:"slug"`
	Category         string  `json:"category"`
	Liquidity        string  `json:"liquidity"`
	EndDate          *string `json:"end_date,omitempty"`
}

/**
 * @function getMarketDetails
 * @description A Gin handler that fetches and returns market details from Polymarket's Gamma API.
 * It uses the 'id' parameter from the URL to identify the market. It tries to fetch by slug first,
 * then falls back to condition ID if slug lookup fails.
 *
 * @param c *gin.Context The Gin context for the request.
 *
 * @notes
 * - This handler fetches real market data from Polymarket's Gamma API.
 * - The 'id' parameter can be either a slug or a condition ID (hex string starting with 0x).
 * - It first tries to fetch by slug, then falls back to condition ID if slug lookup fails.
 * - If no matching market is found, it returns a 404 Not Found error.
 */
func (server *Server) getMarketDetails(c *gin.Context) {
	marketIdentifier := c.Param("id")

	server.logger.Info("fetching details for market", "identifier", marketIdentifier)

	var gammaMarket *polymarket.GammaMarket
	var err error

	// Try fetching by slug first (if it doesn't look like a condition ID)
	// Condition IDs typically start with "0x" and are hex strings
	if len(marketIdentifier) > 2 && marketIdentifier[:2] != "0x" {
		// Try slug first
		gammaMarket, err = server.gammaClient.GetMarketBySlug(c.Request.Context(), marketIdentifier)
		if err == nil {
			server.logger.Info("successfully fetched market by slug", "slug", marketIdentifier)
		} else {
			server.logger.Debug("failed to fetch market by slug, will try condition ID", "slug", marketIdentifier, "error", err)
		}
	}

	// If slug lookup failed or identifier looks like a condition ID, try condition ID
	if gammaMarket == nil {
		gammaMarket, err = server.gammaClient.GetMarketByConditionID(c.Request.Context(), marketIdentifier)
		if err != nil {
			server.logger.Warn("failed to fetch market from Gamma API", "error", err, "identifier", marketIdentifier)
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "message": "Market not found"})
			return
		}
		server.logger.Info("successfully fetched market by condition ID", "condition_id", marketIdentifier)
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

/**
 * @function listMarkets
 * @description A Gin handler that fetches and returns a list of all active markets from Polymarket's Gamma API.
 * It supports pagination via query parameters (limit and offset).
 *
 * @param c *gin.Context The Gin context for the request.
 *
 * @query limit (optional): Maximum number of markets to return (default: 100, max: 100)
 * @query offset (optional): Number of markets to skip (default: 0)
 *
 * @notes
 * - This handler fetches real market data from Polymarket's Gamma API.
 * - Returns only active (non-closed) markets.
 * - Supports pagination for large result sets.
 */
func (server *Server) listMarkets(c *gin.Context) {
	// Parse query parameters for pagination
	limit := 100 // Default limit
	offset := 0  // Default offset

	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	if offsetParam := c.Query("offset"); offsetParam != "" {
		if parsedOffset, err := strconv.Atoi(offsetParam); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	server.logger.Info("fetching active markets from Gamma API", "limit", limit, "offset", offset)

	// Fetch markets from Gamma API
	gammaMarkets, err := server.gammaClient.ListActiveMarkets(c.Request.Context(), limit, offset)
	if err != nil {
		server.logger.Error("failed to fetch markets from Gamma API", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to fetch markets",
		})
		return
	}

	// Convert Gamma API response to our MarketListItem format
	markets := make([]MarketListItem, 0, len(gammaMarkets))
	for _, gammaMarket := range gammaMarkets {
		markets = append(markets, MarketListItem{
			ID:               gammaMarket.ConditionID,
			Title:            gammaMarket.Question,
			Description:      gammaMarket.Question, // Gamma API doesn't have a separate description field
			ResolutionSource: gammaMarket.ResolutionSource,
			Slug:             gammaMarket.Slug,
			Category:         gammaMarket.Category,
			Liquidity:        gammaMarket.Liquidity,
			EndDate:          gammaMarket.EndDate,
		})
	}

	server.logger.Info("successfully fetched markets", "count", len(markets))

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   markets,
		"meta": gin.H{
			"count":  len(markets),
			"limit":  limit,
			"offset": offset,
		},
	})
}

