/**
 * @description
 * This file implements the HTTP client for interacting with Polymarket's Data API.
 * The Data API provides user-specific data including positions, balances, and trading history.
 *
 * Key features:
 * - User Balance Fetching: Retrieves USDC balance for a wallet address
 * - Position Data: Gets user's positions in markets
 * - Trading History: Access to trade history and activity
 *
 * @dependencies
 * - net/http: For HTTP requests
 * - encoding/json: For JSON parsing
 * - log/slog: For structured logging
 */

package polymarket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// DataAPIClient handles interactions with Polymarket's Data API
type DataAPIClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// UserBalance represents a user's USDC balance
type UserBalance struct {
	USDC string `json:"usdc"` // USDC balance as string
}

// DataAPIValueResponse represents the actual response from the Data API
type DataAPIValueResponse struct {
	User  string  `json:"user"`
	Value float64 `json:"value"`
}

// UserPositionsResponse represents the response from the positions endpoint
type UserPositionsResponse struct {
	Positions []UserPosition `json:"positions"`
}

// UserPosition represents a user's position in a market
type UserPosition struct {
	MarketID     string `json:"market_id"`
	TokenID      string `json:"token_id"`
	Side         string `json:"side"`          // "BUY" or "SELL"
	Size         string `json:"size"`          // Position size
	AvgPrice     string `json:"avg_price"`     // Average entry price
	MarketSlug   string `json:"market_slug"`
	TokenOutcome string `json:"token_outcome"` // "YES" or "NO"
}

// NewDataAPIClient creates a new Data API client
func NewDataAPIClient(baseURL string, logger *slog.Logger) *DataAPIClient {
	if baseURL == "" {
		baseURL = "https://data-api.polymarket.com"
	}

	return &DataAPIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// GetUserBalance fetches the USDC balance for a given wallet address
func (c *DataAPIClient) GetUserBalance(ctx context.Context, walletAddress string) (*UserBalance, error) {
	apiURL := fmt.Sprintf("%s/value?user=%s", c.baseURL, url.QueryEscape(walletAddress))

	c.logger.Info("fetching user balance from Data API", "wallet_address", walletAddress, "url", apiURL)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "poly-pro-backend/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("failed to fetch user balance from Data API", "error", err, "wallet_address", walletAddress)
		return nil, fmt.Errorf("failed to fetch balance: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error != "" {
			return nil, fmt.Errorf("Data API error: %s", errorResp.Error)
		}
		return nil, fmt.Errorf("Data API returned status %d: %s", resp.StatusCode, string(body))
	}

	// The API returns an array of value objects
	var valueResponses []DataAPIValueResponse
	if err := json.Unmarshal(body, &valueResponses); err != nil {
		return nil, fmt.Errorf("failed to parse balance response: %w", err)
	}

	if len(valueResponses) == 0 {
		return nil, fmt.Errorf("no balance data returned for wallet: %s", walletAddress)
	}

	// Take the first (and should be only) value response
	valueResp := valueResponses[0]
	balance := UserBalance{
		USDC: fmt.Sprintf("%.6f", valueResp.Value), // Format as string with 6 decimal places
	}

	c.logger.Info("successfully fetched user balance", "wallet_address", walletAddress, "usdc_balance", balance.USDC)
	return &balance, nil
}

// GetUserPositions fetches positions for a given wallet address
func (c *DataAPIClient) GetUserPositions(ctx context.Context, walletAddress string, limit int, offset int) (*UserPositionsResponse, error) {
	apiURL := fmt.Sprintf("%s/positions?user=%s&limit=%d&offset=%d",
		c.baseURL, url.QueryEscape(walletAddress), limit, offset)

	c.logger.Info("fetching user positions from Data API",
		"wallet_address", walletAddress, "limit", limit, "offset", offset, "url", apiURL)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "poly-pro-backend/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("failed to fetch user positions from Data API", "error", err, "wallet_address", walletAddress)
		return nil, fmt.Errorf("failed to fetch positions: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Error != "" {
			return nil, fmt.Errorf("Data API error: %s", errorResp.Error)
		}
		return nil, fmt.Errorf("Data API returned status %d: %s", resp.StatusCode, string(body))
	}

	var positionsResponse UserPositionsResponse
	if err := json.Unmarshal(body, &positionsResponse); err != nil {
		return nil, fmt.Errorf("failed to parse positions response: %w", err)
	}

	c.logger.Info("successfully fetched user positions",
		"wallet_address", walletAddress,
		"position_count", len(positionsResponse.Positions))

	return &positionsResponse, nil
}
