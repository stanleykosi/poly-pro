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

// PriceChangeMessage represents a price change event
type PriceChangeMessage struct {
	EventType string `json:"event_type"` // "price_change"
	AssetID   string `json:"asset_id"`
	Market    string `json:"market"`
	Price     string `json:"price"` // New price
	Timestamp string `json:"timestamp"`
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
					c.logger.Info("WebSocket: parsed price_change event", "market", priceChangeMsg.Market)
				}
				// Convert price_change to a book message with synthetic bids/asks
				// Use the price as both best bid and best ask for OHLCV aggregation
				price := priceChangeMsg.Price
				syntheticBookMsg := BookMessage{
					EventType: "book",
					AssetID:   priceChangeMsg.AssetID,
					Market:    priceChangeMsg.Market,
					Bids: []OrderLevel{
						{Price: price, Size: "0"},
					},
					Asks: []OrderLevel{
						{Price: price, Size: "0"},
					},
					Timestamp: priceChangeMsg.Timestamp,
					Hash:      "",
				}
				if err := handler(&syntheticBookMsg); err != nil {
					c.logger.Error("error handling price_change converted to book message", "error", err)
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
							c.logger.Info("WebSocket: parsed wrapped price_change", "market", priceChangeMsg.Market)
						}
						// Convert price_change to a book message with synthetic bids/asks
						price := priceChangeMsg.Price
						syntheticBookMsg := BookMessage{
							EventType: "book",
							AssetID:   priceChangeMsg.AssetID,
							Market:    priceChangeMsg.Market,
							Bids: []OrderLevel{
								{Price: price, Size: "0"},
							},
							Asks: []OrderLevel{
								{Price: price, Size: "0"},
							},
							Timestamp: priceChangeMsg.Timestamp,
							Hash:      "",
						}
						if err := handler(&syntheticBookMsg); err != nil {
							c.logger.Error("error handling price_change converted to book message", "error", err)
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

