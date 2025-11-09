/**
 * @description
 * This service is responsible for connecting to external real-time data sources,
 * such as Polymarket's WebSocket (WSS) feed, processing the data, and publishing
 * it to Redis for consumption by the WebSocket hub.
 *
 * Key features:
 * - Data Ingestion: Provides a central point for managing connections to third-party
 *   real-time streams.
 * - Data Processing: Can be used to transform or aggregate incoming data before it's
 *   broadcast to clients.
 * - Redis Publishing: Publishes processed data to specific Redis channels, allowing
 *   the WebSocket hub to fan it out to many clients efficiently.
 * - Real-time Connection: Connects to Polymarket's CLOB WebSocket for live order book data.
 *
 * @dependencies
 * - github.com/redis/go-redis/v9: The Redis client library.
 * - github.com/poly-pro/backend/internal/polymarket: For WebSocket client.
 * - log/slog: For structured logging.
 */
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/poly-pro/backend/internal/config"
	"github.com/poly-pro/backend/internal/polymarket"
	"github.com/redis/go-redis/v9"
)

// MarketStreamService is responsible for streaming market data and publishing it.
type MarketStreamService struct {
	redisClient *redis.Client
	logger      *slog.Logger
	ctx         context.Context
	wsClient    *polymarket.CLOBWebSocketClient
	config      config.Config
}

// OrderBookLevel represents a single price level in the order book.
type OrderBookLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

// MockOrderBookData represents the structure of the mock order book data we'll generate.
// This structure matches Polymarket's book message format from their WebSocket API.
type MockOrderBookData struct {
	EventType string           `json:"event_type"` // "book"
	AssetID   string           `json:"asset_id"`    // Token ID (token identifier)
	Market    string           `json:"market"`      // Condition ID (market identifier)
	Bids      []OrderBookLevel  `json:"bids"`        // Array of bid levels
	Asks      []OrderBookLevel  `json:"asks"`        // Array of ask levels
	Timestamp string            `json:"timestamp"`   // Unix timestamp in milliseconds
	Hash      string            `json:"hash"`        // Hash summary of the orderbook content
}

// NewMarketStreamService creates a new MarketStreamService.
func NewMarketStreamService(ctx context.Context, logger *slog.Logger, redisClient *redis.Client, cfg config.Config) *MarketStreamService {
	// Initialize WebSocket client if credentials are provided
	var wsClient *polymarket.CLOBWebSocketClient
	if cfg.CLOBAPIKey != "" && cfg.CLOBAPISecret != "" && cfg.CLOBAPIPassphrase != "" {
		wsClient = polymarket.NewCLOBWebSocketClient(cfg.CLOBWSURL, cfg.CLOBAPIKey, cfg.CLOBAPISecret, cfg.CLOBAPIPassphrase, logger)
	}

	return &MarketStreamService{
		redisClient: redisClient,
		logger:      logger,
		ctx:         ctx,
		wsClient:    wsClient,
		config:      cfg,
	}
}

/**
 * @description
 * RunStream connects to Polymarket's CLOB WebSocket and streams real-time order book data.
 * It subscribes to order book updates for specific tokens and publishes them to Redis.
 *
 * @notes
 * - This function connects to Polymarket's real WebSocket feed.
 * - It runs in an infinite loop and should be started as a goroutine.
 * - It gracefully handles shutdown via the context.
 * - If WebSocket client is not configured, it falls back to a mock stream.
 */
func (s *MarketStreamService) RunStream() {
	if s.wsClient == nil {
		s.logger.Warn("CLOB WebSocket client not configured, falling back to mock stream")
		s.RunMockStream()
		return
	}

	s.logger.Info("starting Polymarket CLOB WebSocket stream service...")

	// Connect to WebSocket
	if err := s.wsClient.Connect(); err != nil {
		s.logger.Error("failed to connect to CLOB WebSocket", "error", err)
		// Fall back to mock stream on connection failure
		s.RunMockStream()
		return
	}
	defer s.wsClient.Close()

	// Subscribe to order book updates for known markets
	// In a production system, you might want to make this configurable or dynamic
	assetIDs := []string{
		"114304586861386186441621124384163963092522056897081085884483958561365015034812", // Xi Jinping market YES token
		"52114319501245915516055106046884209969926127482827954674443846427813813222426", // Fed rates market YES token
	}

	if err := s.wsClient.Subscribe(assetIDs); err != nil {
		s.logger.Error("failed to subscribe to market channel", "error", err)
		return
	}

	// Listen for incoming messages
	handler := func(bookMsg *polymarket.BookMessage) error {
		// Convert to our format
		data := map[string]interface{}{
			"event_type": bookMsg.EventType,
			"asset_id":   bookMsg.AssetID,
			"market":     bookMsg.Market,
			"bids":       bookMsg.Bids,
			"asks":       bookMsg.Asks,
			"timestamp":  bookMsg.Timestamp,
			"hash":       bookMsg.Hash,
		}

		payload, err := json.Marshal(data)
		if err != nil {
			s.logger.Error("failed to marshal order book data", "error", err)
			return err
		}

		// Publish to Redis channel
		channel := "market:" + bookMsg.Market
		if err := s.redisClient.Publish(s.ctx, channel, payload).Err(); err != nil {
			s.logger.Error("failed to publish data to redis", "error", err, "channel", channel)
			return err
		}

		s.logger.Debug("published order book update", "market", bookMsg.Market, "asset_id", bookMsg.AssetID)
		return nil
	}

	// Start listening (this blocks until connection closes)
	if err := s.wsClient.Listen(handler); err != nil {
		s.logger.Error("WebSocket listen error", "error", err)
		// Attempt to reconnect after a delay
		time.Sleep(5 * time.Second)
		s.RunStream() // Recursive call to reconnect
	}
}

/**
 * @description
 * RunMockStream simulates a connection to an external market data feed.
 * It periodically generates fake order book data for a predefined set of markets
 * and publishes it to the corresponding Redis channels.
 *
 * @notes
 * - This function is used as a fallback when WebSocket is not configured or fails.
 * - It runs in an infinite loop and should be started as a goroutine.
 * - It gracefully handles shutdown via the context.
 */
func (s *MarketStreamService) RunMockStream() {
	s.logger.Info("starting mock market data stream service...")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	mockMarkets := []struct {
		Market  string
		AssetID string
	}{
		{
			Market:  "0x1b6f76e5b8587ee896c35847e12d11e75290a8c3934c5952e8a9d6e4c6f03cfa",
			AssetID: "114304586861386186441621124384163963092522056897081085884483958561365015034812",
		},
		{
			Market:  "0xbd31dc8a20211944f6b70f31557f1001557b59905b7738480ca09bd4532f84af",
			AssetID: "52114319501245915516055106046884209969926127482827954674443846427813813222426",
		},
	}

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("stopping mock market data stream service.")
			return
		case <-ticker.C:
			for _, market := range mockMarkets {
				data := s.generateMockOrderBook(market.Market, market.AssetID)
				payload, err := json.Marshal(data)
				if err != nil {
					s.logger.Error("failed to marshal mock order book data", "error", err, "market", market.Market)
					continue
				}

				channel := "market:" + market.Market
				if err := s.redisClient.Publish(s.ctx, channel, payload).Err(); err != nil {
					s.logger.Error("failed to publish data to redis", "error", err, "channel", channel)
				}
			}
		}
	}
}

// generateMockOrderBook creates a randomized order book for a given market and asset ID.
func (s *MarketStreamService) generateMockOrderBook(market string, assetID string) map[string]interface{} {
	// This is a simplified mock - in production you'd use real data
	return map[string]interface{}{
		"event_type": "book",
		"asset_id":   assetID,
		"market":     market,
		"bids":       []interface{}{},
		"asks":       []interface{}{},
		"timestamp":  fmt.Sprintf("%d", time.Now().UnixMilli()),
		"hash":       fmt.Sprintf("0x%x", time.Now().UnixNano()%1000000000000),
	}
}

