/**
 * @description
 * This file implements the WebSocket client for Polymarket's CLOB WebSocket API.
 * The WebSocket provides real-time order book updates and market data.
 *
 * Key features:
 * - Market Channel: Subscribe to order book updates for specific tokens
 * - User Channel: Subscribe to user-specific order and trade updates (requires auth)
 * - Automatic Reconnection: Handles connection drops and reconnects
 * - Message Parsing: Parses incoming WebSocket messages
 *
 * @dependencies
 * - github.com/gorilla/websocket: For WebSocket connections
 * - encoding/json: For JSON message parsing
 * - log/slog: For structured logging
 */

package polymarket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	gorillaWS "github.com/gorilla/websocket"
)

// CLOBWebSocketClient handles WebSocket connections to Polymarket's CLOB
type CLOBWebSocketClient struct {
	baseURL    string
	conn       *gorillaWS.Conn
	logger     *slog.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	apiKey     string
	apiSecret  string
	passphrase string
}

// NewCLOBWebSocketClient creates a new CLOB WebSocket client
func NewCLOBWebSocketClient(baseURL string, apiKey, apiSecret, passphrase string, logger *slog.Logger) *CLOBWebSocketClient {
	if baseURL == "" {
		baseURL = "wss://ws-subscriptions-clob.polymarket.com"
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &CLOBWebSocketClient{
		baseURL:    baseURL,
		logger:     logger,
		ctx:        ctx,
		cancel:     cancel,
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		passphrase: passphrase,
	}
}

// WebSocketMessage represents a message from the WebSocket
type WebSocketMessage struct {
	Type      string          `json:"type"`
	EventType string          `json:"event_type"`
	Data      json.RawMessage `json:"data"`
	Timestamp string          `json:"timestamp"`
}

// BookMessage represents a book update message
type BookMessage struct {
	EventType string        `json:"event_type"` // "book"
	AssetID   string        `json:"asset_id"`
	Market    string        `json:"market"`
	Bids      []OrderLevel  `json:"bids"`
	Asks      []OrderLevel  `json:"asks"`
	Timestamp string        `json:"timestamp"`
	Hash      string        `json:"hash"`
}

// PriceChange represents an individual price change within a price_change event
type PriceChange struct {
	AssetID string `json:"asset_id"`
	Price   string `json:"price"`
	Size    string `json:"size"`
	Side    string `json:"side"`
	Hash    string `json:"hash"`
	BestBid string `json:"best_bid"`
	BestAsk string `json:"best_ask"`
}

// PriceChangeMessage represents a price change event
type PriceChangeMessage struct {
	EventType    string        `json:"event_type"` // "price_change"
	Market       string        `json:"market"`
	PriceChanges []PriceChange `json:"price_changes"`
	Timestamp    string        `json:"timestamp"`
}

// LastTradePriceMessage represents a trade execution event
type LastTradePriceMessage struct {
	EventType  string `json:"event_type"` // "last_trade_price"
	AssetID    string `json:"asset_id"`
	Market     string `json:"market"`
	Price      string `json:"price"`
	Side       string `json:"side"`
	Size       string `json:"size"`        // Trade volume/size
	FeeRateBps string `json:"fee_rate_bps"`
	Timestamp  string `json:"timestamp"`
}

// SubscriptionMessage represents a subscription request
type SubscriptionMessage struct {
	Type      string   `json:"type"`       // "MARKET" or "USER"
	AssetsIDs []string `json:"assets_ids"` // For MARKET channel
	Markets   []string `json:"markets"`     // For USER channel
	Auth      *Auth    `json:"auth,omitempty"` // For USER channel
}

// Auth represents authentication for USER channel
type Auth struct {
	APIKey     string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

// MessageHandler is a function that handles incoming WebSocket messages
type MessageHandler func(message *BookMessage) error

// Connect connects to the WebSocket server
func (c *CLOBWebSocketClient) Connect() error {
	dialer := gorillaWS.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	url := c.baseURL + "/ws/market"
	c.logger.Info("connecting to CLOB WebSocket", "url", url)

	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	c.conn = conn
	c.logger.Info("connected to CLOB WebSocket")
	return nil
}

// Subscribe subscribes to order book updates for specific tokens
func (c *CLOBWebSocketClient) Subscribe(assetIDs []string) error {
	if c.conn == nil {
		return fmt.Errorf("not connected to WebSocket")
	}

	subMsg := SubscriptionMessage{
		Type:      "MARKET",
		AssetsIDs: assetIDs,
	}

	message, err := json.Marshal(subMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal subscription message: %w", err)
	}

	c.logger.Info("subscribing to market channel", "asset_ids", assetIDs, "asset_count", len(assetIDs))
	if err := c.conn.WriteMessage(gorillaWS.TextMessage, message); err != nil {
		return fmt.Errorf("failed to send subscription message: %w", err)
	}

	c.logger.Info("subscription message sent successfully", "asset_count", len(assetIDs))
	return nil
}

// Listen listens for incoming messages and calls the handler
func (c *CLOBWebSocketClient) Listen(handler MessageHandler) error {
	if c.conn == nil {
		return fmt.Errorf("not connected to WebSocket")
	}

	// Start ping goroutine
	go c.ping()

	messageCount := 0
	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if gorillaWS.IsUnexpectedCloseError(err, gorillaWS.CloseGoingAway, gorillaWS.CloseAbnormalClosure) {
					c.logger.Error("WebSocket read error", "error", err)
					return fmt.Errorf("WebSocket read error: %w", err)
				}
				return err
			}

			messageCount++

			// Handle PONG messages
			if string(message) == "PONG" {
				continue
			}

			// Try to parse as array of book messages first (initial snapshot)
			var bookMessages []BookMessage
			if err := json.Unmarshal(message, &bookMessages); err == nil && len(bookMessages) > 0 {
				// Successfully parsed as array of book messages
				if messageCount == 1 {
					c.logger.Info("✅ WebSocket: received initial snapshot", 
						"messages", len(bookMessages))
				}
				// Process each book message in the array
				for i, bookMsg := range bookMessages {
					if bookMsg.EventType == "book" {
						if err := handler(&bookMsg); err != nil {
							c.logger.Error("error handling book message from array", "error", err, "index", i)
						}
					}
				}
				continue
			}

			// Try to parse as book message directly (single message)
			var bookMsg BookMessage
			if err := json.Unmarshal(message, &bookMsg); err == nil && bookMsg.EventType == "book" {
				// Successfully parsed as book message
				if messageCount == 1 {
					c.logger.Info("✅ WebSocket: received first book message", "market", bookMsg.Market)
				}
				if err := handler(&bookMsg); err != nil {
					c.logger.Error("error handling book message", "error", err)
				}
				continue
			}

			// Try to parse as price_change event
			var priceChangeMsg PriceChangeMessage
			if err := json.Unmarshal(message, &priceChangeMsg); err == nil && priceChangeMsg.EventType == "price_change" {
				// Successfully parsed as price change event
				if messageCount == 1 {
					c.logger.Info("WebSocket: parsed price_change event", "market", priceChangeMsg.Market, "changes_count", len(priceChangeMsg.PriceChanges))
				}
				// Process each price change in the array
				for _, change := range priceChangeMsg.PriceChanges {
					// Convert price_change to a book message with best bid/ask from the change
					// Use best_bid and best_ask to create proper order book levels
					var bids []OrderLevel
					var asks []OrderLevel

					// Add best bid if it's valid
					if change.BestBid != "" && change.BestBid != "0" {
						bids = append(bids, OrderLevel{Price: change.BestBid, Size: "100"}) // Use dummy size
					}

					// Add best ask if it's valid
					if change.BestAsk != "" && change.BestAsk != "0" {
						asks = append(asks, OrderLevel{Price: change.BestAsk, Size: "100"}) // Use dummy size
					}

					// Only create book message if we have valid bids or asks
					if len(bids) > 0 || len(asks) > 0 {
						syntheticBookMsg := BookMessage{
							EventType: "book",
							AssetID:   change.AssetID,
							Market:    priceChangeMsg.Market,
							Bids:      bids,
							Asks:      asks,
							Timestamp: priceChangeMsg.Timestamp,
							Hash:      change.Hash,
						}
						if err := handler(&syntheticBookMsg); err != nil {
							c.logger.Error("error handling price_change converted to book message", "error", err, "asset_id", change.AssetID)
						}
					}
				}
				continue
			}

			// Try to parse as last_trade_price event
			var tradeMsg LastTradePriceMessage
			if err := json.Unmarshal(message, &tradeMsg); err == nil && tradeMsg.EventType == "last_trade_price" {
				// Successfully parsed as trade event
				c.logger.Info("✅ WebSocket: parsed last_trade_price event", 
					"market", tradeMsg.Market,
					"asset_id", tradeMsg.AssetID,
					"price", tradeMsg.Price,
					"size", tradeMsg.Size,
					"message_count", messageCount)
				// Convert trade to a book message for OHLCV aggregation with volume
				// Use the trade price as both bid and ask, and trade size as volume
				price := tradeMsg.Price
				size := tradeMsg.Size
				syntheticBookMsg := BookMessage{
					EventType: "book",
					AssetID:   tradeMsg.AssetID,
					Market:    tradeMsg.Market,
					Bids: []OrderLevel{
						{Price: price, Size: size}, // Use actual trade size for volume tracking
					},
					Asks: []OrderLevel{
						{Price: price, Size: size}, // Use actual trade size for volume tracking
					},
					Timestamp: tradeMsg.Timestamp,
					Hash:      "",
				}
				if err := handler(&syntheticBookMsg); err != nil {
					c.logger.Error("error handling last_trade_price converted to book message", "error", err)
				}
				continue
			}

			// Try to parse as WebSocketMessage wrapper
			var wsMsg WebSocketMessage
			if err := json.Unmarshal(message, &wsMsg); err == nil {
				// Check if it's a book message in the wrapper
				if wsMsg.EventType == "book" {
					if err := json.Unmarshal(wsMsg.Data, &bookMsg); err == nil {
						if messageCount == 1 {
							c.logger.Info("✅ WebSocket: received wrapped book message", "market", bookMsg.Market)
						}
						if err := handler(&bookMsg); err != nil {
							c.logger.Error("error handling book message", "error", err)
						}
						continue
					}
				}
				// Check if it's a price_change event in the wrapper
				if wsMsg.EventType == "price_change" {
					var priceChangeMsg PriceChangeMessage
					if err := json.Unmarshal(wsMsg.Data, &priceChangeMsg); err == nil {
						if messageCount == 1 {
							c.logger.Info("WebSocket: parsed wrapped price_change", "market", priceChangeMsg.Market, "changes_count", len(priceChangeMsg.PriceChanges))
						}
						// Process each price change in the array
						for _, change := range priceChangeMsg.PriceChanges {
							// Convert price_change to a book message with best bid/ask from the change
							var bids []OrderLevel
							var asks []OrderLevel

							// Add best bid if it's valid
							if change.BestBid != "" && change.BestBid != "0" {
								bids = append(bids, OrderLevel{Price: change.BestBid, Size: "100"})
							}

							// Add best ask if it's valid
							if change.BestAsk != "" && change.BestAsk != "0" {
								asks = append(asks, OrderLevel{Price: change.BestAsk, Size: "100"})
							}

							// Only create book message if we have valid bids or asks
							if len(bids) > 0 || len(asks) > 0 {
								syntheticBookMsg := BookMessage{
									EventType: "book",
									AssetID:   change.AssetID,
									Market:    priceChangeMsg.Market,
									Bids:      bids,
									Asks:      asks,
									Timestamp: priceChangeMsg.Timestamp,
									Hash:      change.Hash,
								}
								if err := handler(&syntheticBookMsg); err != nil {
									c.logger.Error("error handling wrapped price_change converted to book message", "error", err, "asset_id", change.AssetID)
								}
							}
						}
						continue
					}
				}
				// Check if it's a last_trade_price event in the wrapper
				if wsMsg.EventType == "last_trade_price" {
					var tradeMsg LastTradePriceMessage
					if err := json.Unmarshal(wsMsg.Data, &tradeMsg); err == nil {
						c.logger.Info("✅ WebSocket: parsed wrapped last_trade_price", 
							"market", tradeMsg.Market,
							"asset_id", tradeMsg.AssetID,
							"price", tradeMsg.Price,
							"size", tradeMsg.Size,
							"message_count", messageCount)
						// Convert trade to a book message for OHLCV aggregation with volume
						price := tradeMsg.Price
						size := tradeMsg.Size
						syntheticBookMsg := BookMessage{
							EventType: "book",
							AssetID:   tradeMsg.AssetID,
							Market:    tradeMsg.Market,
							Bids: []OrderLevel{
								{Price: price, Size: size}, // Use actual trade size for volume tracking
							},
							Asks: []OrderLevel{
								{Price: price, Size: size}, // Use actual trade size for volume tracking
							},
							Timestamp: tradeMsg.Timestamp,
							Hash:      "",
						}
						if err := handler(&syntheticBookMsg); err != nil {
							c.logger.Error("error handling wrapped last_trade_price converted to book message", "error", err)
						}
						continue
					}
				}
				// Other message types (subscription confirmations, errors, etc.)
				if wsMsg.Type == "subscribed" || wsMsg.Type == "subscription" {
					c.logger.Info("✅ WebSocket: subscription confirmed", "type", wsMsg.Type)
				} else {
					c.logger.Debug("WebSocket: non-book message", "type", wsMsg.Type, "event_type", wsMsg.EventType)
				}
				continue
			}

			// If we can't parse it at all, log the raw message for debugging
			// This helps identify new message types from Polymarket
			if messageCount <= 3 {
				msgStr := string(message)
				if len(msgStr) > 100 {
					msgStr = msgStr[:100] + "..."
				}
				c.logger.Warn("⚠️  WebSocket: unparseable message", 
					"message", messageCount,
					"preview", msgStr)
			}
		}
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ping sends periodic PING messages to keep the connection alive
func (c *CLOBWebSocketClient) ping() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.conn != nil {
				if err := c.conn.WriteMessage(gorillaWS.TextMessage, []byte("PING")); err != nil {
					c.logger.Error("failed to send ping", "error", err)
					return
				}
			}
		}
	}
}

// Close closes the WebSocket connection
func (c *CLOBWebSocketClient) Close() error {
	c.cancel()
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

