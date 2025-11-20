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
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

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
	ClobTokenIds     string `json:"clobTokenIds"`
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
	Volume           string  `json:"volume,omitempty"` // Total volume for sorting/display
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
		ClobTokenIds:     gammaMarket.ClobTokenIds,
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

	// Fetch more markets than requested to ensure we get top volume markets
	// This matches the WebSocket service approach for consistency
	fetchLimit := limit
	if limit < 500 && offset == 0 {
		// If requesting top markets (offset=0), fetch more to ensure proper volume sorting
		fetchLimit = 500
	}
	gammaMarkets, err := server.gammaClient.ListActiveMarkets(c.Request.Context(), fetchLimit, offset)
	if err != nil {
		server.logger.Error("failed to fetch markets from Gamma API", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "Failed to fetch markets",
		})
		return
	}

	// Convert Gamma API response to our MarketListItem format
	// Store volumeNum for sorting (more reliable than parsing strings)
	type marketWithVolume struct {
		MarketListItem
		volumeNum float64
	}
	
	marketsWithVolume := make([]marketWithVolume, 0, len(gammaMarkets))
	for _, gammaMarket := range gammaMarkets {
		volNum := 0.0
		if gammaMarket.VolumeNum != nil {
			volNum = *gammaMarket.VolumeNum
		} else if gammaMarket.Volume != "" {
			// Fallback to parsing volume string if volumeNum is not available
			if parsed, err := strconv.ParseFloat(gammaMarket.Volume, 64); err == nil {
				volNum = parsed
			}
		}
		
		marketsWithVolume = append(marketsWithVolume, marketWithVolume{
			MarketListItem: MarketListItem{
				ID:               gammaMarket.ConditionID,
				Title:            gammaMarket.Question,
				Description:      gammaMarket.Question, // Gamma API doesn't have a separate description field
				ResolutionSource: gammaMarket.ResolutionSource,
				Slug:             gammaMarket.Slug,
				Category:         gammaMarket.Category,
				Liquidity:        gammaMarket.Liquidity,
				Volume:           gammaMarket.Volume, // Include volume for display
				EndDate:          gammaMarket.EndDate,
			},
			volumeNum: volNum,
		})
	}

	// Sort by volume (descending) to ensure top markets by volume
	// This ensures we always return top markets by volume even if API doesn't support volume ordering
	sort.Slice(marketsWithVolume, func(i, j int) bool {
		return marketsWithVolume[i].volumeNum > marketsWithVolume[j].volumeNum
	})

	// Extract MarketListItem from sorted slice and limit to requested amount
	markets := make([]MarketListItem, 0, len(marketsWithVolume))
	for i, m := range marketsWithVolume {
		if i >= limit {
			break // Respect the original limit requested by frontend
		}
		markets = append(markets, m.MarketListItem)
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

/**
 * @function getOfficialMarketPrices
 * @description A Gin handler that fetches official market prices directly from Polymarket's Gamma API.
 * Uses the Token.price field from the Gamma API which contains Polymarket's official average prices.
 * @param {gin.Context} c - The Gin context containing the request.
 */
func (server *Server) getOfficialMarketPrices(c *gin.Context) {
	// Get market ID from query parameter
	marketID := c.Query("market_id")
	if marketID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"message": "market_id parameter is required",
		})
		return
	}

	// Fetch market from Gamma API which includes the official prices in Token.Price field
	gammaMarket, err := server.gammaClient.GetMarketByConditionID(c.Request.Context(), marketID)
	if err != nil {
		server.logger.Warn("failed to fetch market from Gamma API", "market_id", marketID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"message": "Failed to fetch market data",
		})
		return
	}

	// Extract prices from Gamma API
	// Gamma API provides prices in outcomePrices field (JSON string array: ["NO_price", "YES_price"])
	// And token IDs in clobTokenIds field (JSON string array: ["NO_token_id", "YES_token_id"])
	prices := make([]polymarket.MarketPrice, 0, 2)

	// Parse clobTokenIds to get token IDs
	var tokenIDs []string
	if gammaMarket.ClobTokenIds != "" {
		if err := json.Unmarshal([]byte(gammaMarket.ClobTokenIds), &tokenIDs); err != nil {
			// Try comma-separated format as fallback
			parts := strings.Split(gammaMarket.ClobTokenIds, ",")
			for _, part := range parts {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					tokenIDs = append(tokenIDs, trimmed)
				}
			}
		}
		server.logger.Info("parsed clobTokenIds", "token_ids", tokenIDs, "raw", gammaMarket.ClobTokenIds)
	} else {
		server.logger.Warn("clobTokenIds is empty", "market_id", marketID)
	}

	// Parse outcomePrices to get prices
	var outcomePrices []string
	if gammaMarket.OutcomePrices != "" {
		if err := json.Unmarshal([]byte(gammaMarket.OutcomePrices), &outcomePrices); err != nil {
			server.logger.Warn("failed to parse outcomePrices", "outcomePrices", gammaMarket.OutcomePrices, "error", err)
		} else {
			server.logger.Info("parsed outcomePrices", "prices", outcomePrices, "raw", gammaMarket.OutcomePrices)
		}
	} else {
		server.logger.Warn("outcomePrices is empty", "market_id", marketID)
	}

	// If we have both token IDs and prices, create market price entries
	// Polymarket convention: [NO, YES] - index 0 is NO, index 1 is YES
	if len(tokenIDs) >= 2 && len(outcomePrices) >= 2 {
		// NO token (index 0)
		if noPrice, err := parseFloat(outcomePrices[0]); err == nil && noPrice >= 0 && noPrice <= 1 {
			marketPrice := polymarket.MarketPrice{
				TokenID:     tokenIDs[0],
				BestBid:     noPrice,
				BestAsk:     noPrice,
				Spread:      0,
				MarketPrice: noPrice,
				PriceSource: "gamma_api",
				LastUpdated: gammaMarket.UpdatedAt,
			}
			prices = append(prices, marketPrice)
			server.logger.Debug("added NO token price", "token_id", tokenIDs[0], "price", noPrice)
		} else if err != nil {
			server.logger.Warn("failed to parse NO price", "price_string", outcomePrices[0], "error", err)
		}

		// YES token (index 1)
		if yesPrice, err := parseFloat(outcomePrices[1]); err == nil && yesPrice >= 0 && yesPrice <= 1 {
			marketPrice := polymarket.MarketPrice{
				TokenID:     tokenIDs[1],
				BestBid:     yesPrice,
				BestAsk:     yesPrice,
				Spread:      0,
				MarketPrice: yesPrice,
				PriceSource: "gamma_api",
				LastUpdated: gammaMarket.UpdatedAt,
			}
			prices = append(prices, marketPrice)
			server.logger.Debug("added YES token price", "token_id", tokenIDs[1], "price", yesPrice)
		} else if err != nil {
			server.logger.Warn("failed to parse YES price", "price_string", outcomePrices[1], "error", err)
		}
	} else {
		server.logger.Warn("insufficient data to create prices", 
			"token_ids_count", len(tokenIDs), 
			"outcome_prices_count", len(outcomePrices),
			"market_id", marketID)
	}

	// Fallback: If outcomePrices didn't work, try using tokens array (if available)
	if len(prices) == 0 && len(gammaMarket.Tokens) > 0 {
		for _, token := range gammaMarket.Tokens {
			if token.TokenID == "" || token.Price == "" {
				continue
			}

			price, err := parseFloat(token.Price)
			if err != nil || price < 0 || price > 1 {
				continue
			}

			marketPrice := polymarket.MarketPrice{
				TokenID:     token.TokenID,
				BestBid:     price,
				BestAsk:     price,
				Spread:      0,
				MarketPrice: price,
				PriceSource: "gamma_api",
				LastUpdated: gammaMarket.UpdatedAt,
			}
			prices = append(prices, marketPrice)
		}
	}

	if len(prices) == 0 {
		server.logger.Warn("no valid prices found for market", "market_id", marketID, "tokens_count", len(gammaMarket.Tokens))
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"message": "No valid prices found for market",
		})
		return
	}

	server.logger.Info("successfully fetched official market prices from Gamma API", "market_id", marketID, "price_count", len(prices))

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   prices,
		"meta": gin.H{
			"market_id": marketID,
			"returned_prices": len(prices),
			"source": "gamma_api",
		},
	})
}

/**
 * @function parseTokenIDs
 * @description Parses token IDs from a comma-separated string.
 * @param {string} tokenIDsParam - Comma-separated string of token IDs.
 * @returns {[]string} Array of token IDs.
 */
func parseTokenIDs(tokenIDsParam string) []string {
	if tokenIDsParam == "" {
		return []string{}
	}

	// Split by comma and trim whitespace
	parts := strings.Split(tokenIDsParam, ",")
	tokenIDs := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			tokenIDs = append(tokenIDs, trimmed)
		}
	}

	return tokenIDs
}

// parseFloat converts a string to float64, returning 0 on error
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

