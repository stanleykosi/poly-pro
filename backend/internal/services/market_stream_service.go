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
 * - Mock Implementation: For the initial MVP and local development, this service includes
 *   a mock data generator that simulates live market data.
 *
 * @dependencies
 * - github.com/redis/go-redis/v9: The Redis client library.
 * - log/slog: For structured logging.
 *
 * @notes
 * - The `RunMockStream` function should be started as a goroutine at application launch.
 * - In a production environment, `RunMockStream` would be replaced with a function that
 *   establishes a persistent connection to Polymarket's actual WSS feed.
 */
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
)

// MarketStreamService is responsible for streaming market data and publishing it.
type MarketStreamService struct {
	redisClient *redis.Client
	logger      *slog.Logger
	ctx         context.Context
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
func NewMarketStreamService(ctx context.Context, logger *slog.Logger, redisClient *redis.Client) *MarketStreamService {
	return &MarketStreamService{
		redisClient: redisClient,
		logger:      logger,
		ctx:         ctx,
	}
}

/**
 * @description
 * RunMockStream simulates a connection to an external market data feed.
 * It periodically generates fake order book data for a predefined set of markets
 * and publishes it to the corresponding Redis channels.
 *
 * @notes
 * - This function is intended for development and testing to validate the WebSocket
 *   and Redis Pub/Sub architecture.
 * - It runs in an infinite loop and should be started as a goroutine.
 * - It gracefully handles shutdown via the context.
 */
func (s *MarketStreamService) RunMockStream() {
	s.logger.Info("starting mock market data stream service...")
	// Ticker to generate data every 2 seconds.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// A predefined list of markets (condition IDs) and their corresponding asset IDs (token IDs).
	// In Polymarket, each market has two tokens: one for YES and one for NO.
	// For simplicity, we'll use the YES token for each market.
	mockMarkets := []struct {
		Market  string // Condition ID (market identifier)
		AssetID string // Token ID (YES token for this market)
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
			// For each mock market, generate and publish new data.
			for _, market := range mockMarkets {
				data := s.generateMockOrderBook(market.Market, market.AssetID)
				payload, err := json.Marshal(data)
				if err != nil {
					s.logger.Error("failed to marshal mock order book data", "error", err, "market", market.Market)
					continue
				}

				// Publish the data to the Redis channel for this market.
				// Using condition ID (market) as the channel identifier.
				channel := "market:" + market.Market
				if err := s.redisClient.Publish(s.ctx, channel, payload).Err(); err != nil {
					s.logger.Error("failed to publish data to redis", "error", err, "channel", channel)
				}
			}
		}
	}
}

// generateMockOrderBook creates a randomized order book for a given market and asset ID.
// This matches Polymarket's book message format from their WebSocket API.
func (s *MarketStreamService) generateMockOrderBook(market string, assetID string) MockOrderBookData {
	// Generate a random base price to simulate market movement.
	basePrice := 0.45 + rand.Float64()*0.1 // Random price between 0.45 and 0.55

	bids := make([]OrderBookLevel, 5)
	asks := make([]OrderBookLevel, 5)

	// Generate 5 bid levels below the base price.
	for i := 0; i < 5; i++ {
		price := basePrice - float64(i+1)*0.01
		size := 100 + rand.Float64()*100
		bids[i] = OrderBookLevel{
			Price: fmt.Sprintf("%.2f", price),
			Size:  fmt.Sprintf("%.2f", size),
		}
	}

	// Generate 5 ask levels above the base price.
	for i := 0; i < 5; i++ {
		price := basePrice + float64(i+1)*0.01
		size := 100 + rand.Float64()*100
		asks[i] = OrderBookLevel{
			Price: fmt.Sprintf("%.2f", price),
			Size:  fmt.Sprintf("%.2f", size),
		}
	}

	// Generate a mock hash for the orderbook content.
	// In production, this would be computed from the actual orderbook state.
	// For mock purposes, we'll generate a simple hash-like string.
	hash := fmt.Sprintf("0x%x", time.Now().UnixNano()%1000000000000)

	return MockOrderBookData{
		EventType: "book",
		AssetID:   assetID,
		Market:    market,
		Bids:      bids,
		Asks:      asks,
		Timestamp: fmt.Sprintf("%d", time.Now().UnixMilli()),
		Hash:      hash,
	}
}

