/**
 * @description
 * This file implements the HTTP client for interacting with Polymarket's CLOB API.
 * The CLOB API handles order placement, order book retrieval, and trading operations.
 *
 * Key features:
 * - Order Placement: Submit signed orders to the CLOB
 * - Order Book Retrieval: Fetch current order book state
 * - API Key Authentication: Uses L2 headers for authenticated requests
 * - Error Handling: Proper error handling for API responses
 *
 * @dependencies
 * - net/http: For HTTP requests
 * - encoding/json: For JSON parsing
 * - log/slog: For structured logging
 * - crypto/hmac: For API key authentication
 */

package polymarket

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// CLOBAPIClient handles interactions with Polymarket's CLOB API
type CLOBAPIClient struct {
	baseURL    string
	httpClient *http.Client
	apiKey     string
	apiSecret  string
	passphrase string
	logger     *slog.Logger
}

// NewCLOBAPIClient creates a new CLOB API client
func NewCLOBAPIClient(baseURL string, apiKey, apiSecret, passphrase string, logger *slog.Logger) *CLOBAPIClient {
	if baseURL == "" {
		baseURL = "https://clob.polymarket.com"
	}

	return &CLOBAPIClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		passphrase: passphrase,
		logger:     logger,
	}
}

// OrderBookSummary represents the order book response from CLOB API
type OrderBookSummary struct {
	Market        string        `json:"market"`
	AssetID       string        `json:"asset_id"`
	Timestamp     string        `json:"timestamp"`
	Hash          string        `json:"hash"`
	Bids          []OrderLevel  `json:"bids"`
	Asks          []OrderLevel  `json:"asks"`
	MinOrderSize string        `json:"min_order_size"`
	TickSize      string        `json:"tick_size"`
	NegRisk       bool          `json:"neg_risk"`
}

// OrderLevel represents a single price level in the order book
type OrderLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

// PostOrderRequest represents the request to place an order
type PostOrderRequest struct {
	Order    SignedOrder `json:"order"`
	Owner    string      `json:"owner"`
	OrderType string     `json:"orderType"` // "GTC", "GTD", "FOK", "FAK"
}

// PostOrderResponse represents the response from placing an order
type PostOrderResponse struct {
	Success     bool     `json:"success"`
	ErrorMsg    string   `json:"errorMsg"`
	OrderID     string   `json:"orderId"`
	OrderHashes []string `json:"orderHashes"`
	Status      string   `json:"status"` // "matched", "live", "delayed", "unmatched"
}

// CLOBError represents an error response from the CLOB API
type CLOBError struct {
	Error string `json:"error"`
}

// createAuthHeaders creates the L2 authentication headers for CLOB API
// The address parameter should be the maker address (funder address) from the order
func (c *CLOBAPIClient) createAuthHeaders(method, path, body, address string, timestamp int64) (map[string]string, error) {
	if c.apiKey == "" || c.apiSecret == "" || c.passphrase == "" {
		return nil, fmt.Errorf("API credentials not configured")
	}

	// Create the message to sign: timestamp + method + path + body
	message := strconv.FormatInt(timestamp, 10) + method + path + body

	// Create HMAC signature using the secret
	h := hmac.New(sha256.New, []byte(c.apiSecret))
	h.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	headers := map[string]string{
		"POLY_ADDRESS":    address,
		"POLY_API_KEY":    c.apiKey,
		"POLY_SIGNATURE":  signature,
		"POLY_TIMESTAMP":  strconv.FormatInt(timestamp, 10),
		"POLY_PASSPHRASE": c.passphrase,
	}

	return headers, nil
}

// GetOrderBook fetches the order book for a specific token
func (c *CLOBAPIClient) GetOrderBook(ctx context.Context, tokenID string) (*OrderBookSummary, error) {
	apiURL := fmt.Sprintf("%s/book?token_id=%s", c.baseURL, tokenID)

	c.logger.Info("fetching order book from CLOB API", "token_id", tokenID)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "poly-pro-backend/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("failed to fetch order book from CLOB API", "error", err, "token_id", tokenID)
		return nil, fmt.Errorf("failed to fetch order book: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var clobErr CLOBError
		if err := json.Unmarshal(body, &clobErr); err == nil {
			return nil, fmt.Errorf("CLOB API error: %s", clobErr.Error)
		}
		return nil, fmt.Errorf("CLOB API returned status %d: %s", resp.StatusCode, string(body))
	}

	var orderBook OrderBookSummary
	if err := json.Unmarshal(body, &orderBook); err != nil {
		return nil, fmt.Errorf("failed to parse order book response: %w", err)
	}

	return &orderBook, nil
}

// parseFloat converts a string to float64, returning 0 on error
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// GetOfficialMarketPrice calculates the official market price using Polymarket's formula
// Returns the midpoint of bid-ask spread, or last traded price if spread > $0.10
func (c *CLOBAPIClient) GetOfficialMarketPrice(ctx context.Context, tokenID string) (*MarketPrice, error) {
	orderBook, err := c.GetOrderBook(ctx, tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to get order book: %w", err)
	}

	// Find best bid and ask
	var bestBid, bestAsk float64
	var hasBid, hasAsk bool

	if len(orderBook.Bids) > 0 {
		if bidPrice, err := parseFloat(orderBook.Bids[0].Price); err == nil {
			bestBid = bidPrice
			hasBid = true
		}
	}

	if len(orderBook.Asks) > 0 {
		if askPrice, err := parseFloat(orderBook.Asks[0].Price); err == nil {
			bestAsk = askPrice
			hasAsk = true
		}
	}

	if !hasBid || !hasAsk {
		return nil, fmt.Errorf("insufficient order book data for token %s", tokenID)
	}

	// Calculate spread
	spread := bestAsk - bestBid

	// Polymarket's official price calculation:
	// Use midpoint unless spread > $0.10, then should use last traded price
	// For now, we'll use midpoint as primary method
	marketPrice := (bestBid + bestAsk) / 2.0

	// Check if we should use last traded price instead of midpoint
	// Polymarket uses last traded price when spread > $0.10
	finalPrice := marketPrice
	priceSource := "midpoint"
	lastTradedPrice := 0.0

	if spread > 0.10 {
		// Try to get last traded price
		if lastTrade, err := c.GetLastTradedPrice(ctx, tokenID); err == nil && lastTrade > 0 {
			finalPrice = lastTrade
			priceSource = "last_traded"
			lastTradedPrice = lastTrade
			c.logger.Info("using last traded price due to wide spread",
				"token_id", tokenID, "spread", spread, "last_trade", lastTrade)
		} else {
			c.logger.Warn("wide spread detected but could not get last traded price",
				"token_id", tokenID, "spread", spread, "error", err)
		}
	}

	return &MarketPrice{
		TokenID:         tokenID,
		BestBid:         bestBid,
		BestAsk:         bestAsk,
		Spread:          spread,
		MarketPrice:     finalPrice,
		LastTradedPrice: lastTradedPrice,
		PriceSource:     priceSource,
		LastUpdated:     orderBook.Timestamp,
	}, nil
}

// MarketPrice represents the official market price for a token
type MarketPrice struct {
	TokenID         string  `json:"token_id"`
	BestBid         float64 `json:"best_bid"`
	BestAsk         float64 `json:"best_ask"`
	Spread          float64 `json:"spread"`
	MarketPrice     float64 `json:"market_price"` // Official price (midpoint or last traded)
	LastTradedPrice float64 `json:"last_traded_price,omitempty"` // Last traded price if available
	PriceSource     string  `json:"price_source"` // "midpoint" or "last_traded"
	LastUpdated     string  `json:"last_updated"`
}

// Trade represents a trade from the Polymarket API
type Trade struct {
	ID             string `json:"id"`
	Market         string `json:"market"`
	AssetID        string `json:"asset_id"`
	Side           string `json:"side"`
	Size           string `json:"size"`
	Price          string `json:"price"`
	MatchTime      string `json:"match_time"`
	LastUpdate     string `json:"last_update"`
}

// GetLastTradedPrice fetches the most recent trade price for a token
func (c *CLOBAPIClient) GetLastTradedPrice(ctx context.Context, tokenID string) (float64, error) {
	// Use the /data/trades endpoint to get recent trades for this token
	// Get trades from the last hour to avoid too much data
	oneHourAgo := time.Now().Add(-time.Hour).Unix()
	apiURL := fmt.Sprintf("%s/data/trades?asset_id=%s&after=%d", c.baseURL, tokenID, oneHourAgo)

	c.logger.Debug("fetching last traded price from CLOB API", "token_id", tokenID, "after", oneHourAgo)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "poly-pro-backend/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch trades: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("trades API returned status %d: %s", resp.StatusCode, string(body))
	}

	var trades []Trade
	if err := json.Unmarshal(body, &trades); err != nil {
		return 0, fmt.Errorf("failed to parse trades response: %w", err)
	}

	if len(trades) == 0 {
		return 0, fmt.Errorf("no trades found for token %s", tokenID)
	}

	// Sort trades by match time to find the most recent
	// Assuming match_time is a unix timestamp string
	sort.Slice(trades, func(i, j int) bool {
		timeI, errI := strconv.ParseInt(trades[i].MatchTime, 10, 64)
		timeJ, errJ := strconv.ParseInt(trades[j].MatchTime, 10, 64)
		if errI != nil || errJ != nil {
			return false // Keep original order if parsing fails
		}
		return timeI > timeJ // Sort descending (most recent first)
	})

	// Get the most recent trade
	lastTrade := trades[0]
	price, err := parseFloat(lastTrade.Price)
	if err != nil {
		return 0, fmt.Errorf("failed to parse trade price: %w", err)
	}

	c.logger.Debug("found last traded price", "token_id", tokenID, "price", price, "trade_id", lastTrade.ID)
	return price, nil
}

// PostOrder submits a signed order to the CLOB API
func (c *CLOBAPIClient) PostOrder(ctx context.Context, signedOrder *SignedOrder, orderType string) (*PostOrderResponse, error) {
	if orderType == "" {
		orderType = "GTC" // Default to Good-Till-Cancelled
	}

	// The owner should be the API key
	owner := c.apiKey

	postReq := PostOrderRequest{
		Order:     *signedOrder,
		Owner:     owner,
		OrderType: orderType,
	}

	bodyBytes, err := json.Marshal(postReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal order request: %w", err)
	}

	path := "/order"
	apiURL := c.baseURL + path
	timestamp := time.Now().Unix()

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication headers - use the maker address from the order
	authHeaders, err := c.createAuthHeaders("POST", path, string(bodyBytes), signedOrder.Maker, timestamp)
	if err != nil {
		return nil, fmt.Errorf("failed to create auth headers: %w", err)
	}

	for k, v := range authHeaders {
		req.Header.Set(k, v)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "poly-pro-backend/1.0")

	c.logger.Info("submitting order to CLOB API", "token_id", signedOrder.TokenId, "side", signedOrder.Side, "order_type", orderType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Error("failed to submit order to CLOB API", "error", err)
		return nil, fmt.Errorf("failed to submit order: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var orderResp PostOrderResponse
	if err := json.Unmarshal(body, &orderResp); err != nil {
		return nil, fmt.Errorf("failed to parse order response: %w", err)
	}

	if !orderResp.Success {
		c.logger.Warn("order submission failed", "error_msg", orderResp.ErrorMsg, "status", orderResp.Status)
		return &orderResp, fmt.Errorf("order submission failed: %s", orderResp.ErrorMsg)
	}

	c.logger.Info("order successfully submitted", "order_id", orderResp.OrderID, "status", orderResp.Status)
	return &orderResp, nil
}

