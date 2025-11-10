/**
 * @description
 * This file implements the HTTP client for interacting with Polymarket's Gamma API.
 * The Gamma API provides market data including market details, events, and metadata.
 *
 * Key features:
 * - Market Data Fetching: Retrieves market information by condition ID, slug, or ID
 * - Public API: No authentication required for market data endpoints
 * - Error Handling: Proper error handling for API responses
 * - Rate Limiting: Respects API rate limits
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

// GammaAPIClient handles interactions with Polymarket's Gamma API
type GammaAPIClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewGammaAPIClient creates a new Gamma API client
func NewGammaAPIClient(baseURL string, logger *slog.Logger) *GammaAPIClient {
	if baseURL == "" {
		baseURL = "https://gamma-api.polymarket.com"
	}

	return &GammaAPIClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// GammaMarket represents a market from the Gamma API
type GammaMarket struct {
	ID               string    `json:"id"`
	Question         string    `json:"question"`
	ConditionID      string    `json:"conditionId"`
	Slug             string    `json:"slug"`
	ResolutionSource string    `json:"resolutionSource"`
	EndDate          *string   `json:"endDate"`
	StartDate        *string   `json:"startDate"`
	Category         string    `json:"category"`
	AMMType          string    `json:"ammType"`
	Liquidity        string    `json:"liquidity"`
	Image            string    `json:"image"`
	Icon             string    `json:"icon"`
	Tokens           []Token   `json:"tokens"`
	CreatedAt        string    `json:"createdAt"`
	UpdatedAt        string    `json:"updatedAt"`
}

// Token represents a token in a market
type Token struct {
	TokenID string `json:"tokenId"`
	Outcome string `json:"outcome"`
	Price   string `json:"price"`
}

// GammaError represents an error response from the Gamma API
type GammaError struct {
	Message string `json:"message"`
	Error   string `json:"error"`
}

// GetMarketByConditionID fetches a market by its condition ID
func (c *GammaAPIClient) GetMarketByConditionID(ctx context.Context, conditionID string) (*GammaMarket, error) {
	// Use the markets endpoint with conditionId query parameter
	apiURL := fmt.Sprintf("%s/markets?conditionId=%s", c.baseURL, url.QueryEscape(conditionID))

	c.logger.Info("fetching market from Gamma API", "condition_id", conditionID, "url", apiURL)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "poly-pro-backend/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("failed to fetch market from Gamma API", "error", err, "condition_id", conditionID)
		return nil, fmt.Errorf("failed to fetch market: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var gammaErr GammaError
		if err := json.Unmarshal(body, &gammaErr); err == nil {
			return nil, fmt.Errorf("Gamma API error: %s", gammaErr.Error)
		}
		return nil, fmt.Errorf("Gamma API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Gamma API returns an array of markets
	var markets []GammaMarket
	if err := json.Unmarshal(body, &markets); err != nil {
		return nil, fmt.Errorf("failed to parse market response: %w", err)
	}

	if len(markets) == 0 {
		return nil, fmt.Errorf("market not found for condition ID: %s", conditionID)
	}

	// Return the first market (should only be one for a specific condition ID)
	return &markets[0], nil
}

// GetMarketBySlug fetches a market by its slug
func (c *GammaAPIClient) GetMarketBySlug(ctx context.Context, slug string) (*GammaMarket, error) {
	apiURL := fmt.Sprintf("%s/markets/slug/%s", c.baseURL, url.PathEscape(slug))

	c.logger.Info("fetching market from Gamma API by slug", "slug", slug)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "poly-pro-backend/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("failed to fetch market from Gamma API", "error", err, "slug", slug)
		return nil, fmt.Errorf("failed to fetch market: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var gammaErr GammaError
		if err := json.Unmarshal(body, &gammaErr); err == nil {
			return nil, fmt.Errorf("Gamma API error: %s", gammaErr.Error)
		}
		return nil, fmt.Errorf("Gamma API returned status %d: %s", resp.StatusCode, string(body))
	}

	var market GammaMarket
	if err := json.Unmarshal(body, &market); err != nil {
		return nil, fmt.Errorf("failed to parse market response: %w", err)
	}

	return &market, nil
}

// ListActiveMarkets fetches all active markets from the Gamma API
// It supports pagination and filtering to get only active (non-closed) markets
func (c *GammaAPIClient) ListActiveMarkets(ctx context.Context, limit int, offset int) ([]GammaMarket, error) {
	apiURL := fmt.Sprintf("%s/markets?closed=false&limit=%d&offset=%d&order=id&ascending=false", 
		c.baseURL, limit, offset)

	c.logger.Info("fetching active markets from Gamma API", "limit", limit, "offset", offset, "url", apiURL)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "poly-pro-backend/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("failed to fetch markets from Gamma API", "error", err)
		return nil, fmt.Errorf("failed to fetch markets: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var gammaErr GammaError
		if err := json.Unmarshal(body, &gammaErr); err == nil {
			return nil, fmt.Errorf("Gamma API error: %s", gammaErr.Error)
		}
		return nil, fmt.Errorf("Gamma API returned status %d: %s", resp.StatusCode, string(body))
	}

	var markets []GammaMarket
	if err := json.Unmarshal(body, &markets); err != nil {
		return nil, fmt.Errorf("failed to parse markets response: %w", err)
	}

	c.logger.Info("fetched markets from Gamma API", "count", len(markets))
	return markets, nil
}

// GetAllActiveMarkets fetches all active markets by paginating through the API
// It continues fetching until no more markets are returned
func (c *GammaAPIClient) GetAllActiveMarkets(ctx context.Context) ([]GammaMarket, error) {
	var allMarkets []GammaMarket
	limit := 100 // Maximum per page
	offset := 0

	for {
		markets, err := c.ListActiveMarkets(ctx, limit, offset)
		if err != nil {
			return nil, err
		}

		if len(markets) == 0 {
			break // No more markets
		}

		allMarkets = append(allMarkets, markets...)
		offset += limit

		// If we got fewer than the limit, we've reached the end
		if len(markets) < limit {
			break
		}

		// Add a small delay to respect rate limits
		time.Sleep(100 * time.Millisecond)
	}

	c.logger.Info("fetched all active markets", "total_count", len(allMarkets))
	return allMarkets, nil
}

