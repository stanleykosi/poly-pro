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
	"strconv"
	"strings"
	"time"

	"github.com/poly-pro/backend/internal/config"
	db "github.com/poly-pro/backend/internal/db"
	"github.com/poly-pro/backend/internal/polymarket"
	"github.com/redis/go-redis/v9"
)

// MarketStreamService is responsible for streaming market data and publishing it.
type MarketStreamService struct {
	redisClient     *redis.Client
	logger          *slog.Logger
	ctx             context.Context
	wsClient        *polymarket.CLOBWebSocketClient
	config          config.Config
	ohlcvAggregator *OHLCVAggregator
	gammaClient     *polymarket.GammaAPIClient
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
func NewMarketStreamService(ctx context.Context, logger *slog.Logger, redisClient *redis.Client, cfg config.Config, store db.Querier, gammaClient *polymarket.GammaAPIClient) *MarketStreamService {
	// Initialize WebSocket client if credentials are provided
	var wsClient *polymarket.CLOBWebSocketClient
	if cfg.CLOBAPIKey != "" && cfg.CLOBAPISecret != "" && cfg.CLOBAPIPassphrase != "" {
		wsClient = polymarket.NewCLOBWebSocketClient(cfg.CLOBWSURL, cfg.CLOBAPIKey, cfg.CLOBAPISecret, cfg.CLOBAPIPassphrase, logger)
	}

	// Initialize OHLCV aggregator
	ohlcvAggregator := NewOHLCVAggregator(ctx, logger, store)

	return &MarketStreamService{
		redisClient:     redisClient,
		logger:          logger,
		ctx:             ctx,
		wsClient:        wsClient,
		config:          cfg,
		ohlcvAggregator: ohlcvAggregator,
		gammaClient:     gammaClient,
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

	// Fetch active markets from Gamma API and extract token IDs
	// Also create a mapping from asset/token ID to condition ID for publishing to correct Redis channels
	var assetIDs []string
	assetIDToConditionID := make(map[string]string) // Map asset ID -> condition ID
	
	if s.gammaClient != nil {
		s.logger.Info("fetching active markets from Gamma API to subscribe to WebSocket...")
		
		// Fetch active markets (limit to first 100 for initial implementation)
		// You can increase this or use GetAllActiveMarkets() for all markets
		markets, err := s.gammaClient.ListActiveMarkets(s.ctx, 100, 0)
		if err != nil {
			s.logger.Error("failed to fetch markets from Gamma API", "error", err)
			return // No fallback, as per user request
		}
		
		// Log first market structure for debugging (only if needed)
		if len(markets) > 0 {
			firstMarket := markets[0]
			s.logger.Info("sample market from Gamma API", 
				"market_id", firstMarket.ConditionID,
				"has_clobTokenIds", firstMarket.ClobTokenIds != "",
				"tokens_count", len(firstMarket.Tokens))
		}
		
		// Extract token IDs from markets and create mapping
		// Try multiple methods to get token IDs:
		// 1. Use clobTokenIds field (preferred - comma-separated or JSON array)
		// 2. Fall back to Tokens array if clobTokenIds is empty
		marketsWithTokens := 0
		marketsWithoutTokens := 0
		for i, market := range markets {
			var tokenIDs []string
			
			// Method 1: Try clobTokenIds field (most reliable)
			if market.ClobTokenIds != "" {
				// Try parsing as JSON array first
				var jsonArray []string
				if err := json.Unmarshal([]byte(market.ClobTokenIds), &jsonArray); err == nil {
					// Successfully parsed as JSON array
					tokenIDs = jsonArray
				} else {
					// Try parsing as comma-separated string
					parts := strings.Split(market.ClobTokenIds, ",")
					for _, part := range parts {
						trimmed := strings.TrimSpace(part)
						if trimmed != "" {
							tokenIDs = append(tokenIDs, trimmed)
						}
					}
				}
			}
			
			// Method 2: Fall back to Tokens array if clobTokenIds didn't work
			if len(tokenIDs) == 0 && len(market.Tokens) > 0 {
				for _, token := range market.Tokens {
					if token.TokenID != "" {
						tokenIDs = append(tokenIDs, token.TokenID)
					}
				}
			}
			
			// Track statistics
			if len(tokenIDs) == 0 {
				marketsWithoutTokens++
				// Only log warning for first few markets without tokens to avoid spam
				if marketsWithoutTokens <= 3 {
					s.logger.Warn("no token IDs found for market",
						"market_id", market.ConditionID,
						"market_index", i)
				}
			} else {
				marketsWithTokens++
				// Create mapping from token ID to condition ID
				// This allows us to publish to Redis channels using condition ID when messages arrive
				for _, tokenID := range tokenIDs {
					assetIDToConditionID[tokenID] = market.ConditionID
				}
				// Add all token IDs found for this market
				assetIDs = append(assetIDs, tokenIDs...)
			}
		}
	s.logger.Info("✅ extracted token IDs from Gamma API markets", 
		"market_count", len(markets),
		"markets_with_tokens", marketsWithTokens,
		"markets_without_tokens", marketsWithoutTokens,
		"total_token_ids", len(assetIDs),
		"mapping_size", len(assetIDToConditionID))
	} else {
		s.logger.Error("Gamma client not available - cannot fetch markets")
		return
	}

	if len(assetIDs) == 0 {
		s.logger.Error("no asset IDs to subscribe to - markets fetched but no token IDs extracted")
		return
	}

	s.logger.Info("📡 proceeding to WebSocket subscription", "asset_count", len(assetIDs), "unique_markets", len(assetIDToConditionID))
	s.logger.Info("📡 subscribing to WebSocket channels", "asset_count", len(assetIDs))
	if err := s.wsClient.Subscribe(assetIDs); err != nil {
		s.logger.Error("❌ failed to subscribe to WebSocket channels", "error", err)
		return
	}
	s.logger.Info("✅ WebSocket subscription request sent", "asset_count", len(assetIDs))

	// Listen for incoming messages
	messageCount := 0
	handler := func(bookMsg *polymarket.BookMessage) error {
		messageCount++
		if messageCount == 1 {
			s.logger.Info("✅ WebSocket: first message received, subscription confirmed", 
				"market", bookMsg.Market,
				"asset_id", bookMsg.AssetID)
		}
		
		// Log every 1000th message to show continuous data flow
		if messageCount%1000 == 0 {
			s.logger.Info("📊 WebSocket: messages flowing", "total", messageCount)
		}

		// Convert bids/asks to interface{} for ExtractMidPrice
		bids := make([]interface{}, len(bookMsg.Bids))
		for i, bid := range bookMsg.Bids {
			bids[i] = map[string]interface{}{
				"price": bid.Price,
				"size":  bid.Size,
			}
		}
		asks := make([]interface{}, len(bookMsg.Asks))
		for i, ask := range bookMsg.Asks {
			asks[i] = map[string]interface{}{
				"price": ask.Price,
				"size":  ask.Size,
			}
		}

		// Map asset ID to condition ID FIRST (before OHLCV aggregation)
		// The frontend subscribes using condition IDs, and we need to use condition IDs for OHLCV storage too
		conditionID := bookMsg.Market // Default to bookMsg.Market (might already be condition ID)
		
		// Try to map asset ID to condition ID
		if mappedConditionID, ok := assetIDToConditionID[bookMsg.AssetID]; ok {
			conditionID = mappedConditionID
			s.logger.Debug("mapped asset ID to condition ID", "asset_id", bookMsg.AssetID, "condition_id", conditionID)
		} else if mappedConditionID, ok := assetIDToConditionID[bookMsg.Market]; ok {
			// bookMsg.Market might also be an asset ID
			conditionID = mappedConditionID
			s.logger.Debug("mapped market field to condition ID", "market", bookMsg.Market, "condition_id", conditionID)
		} else {
			// If no mapping found, log a warning but still use as-is (bookMsg.Market might already be condition ID)
			s.logger.Debug("no mapping found for asset/market ID, using as-is", 
				"asset_id", bookMsg.AssetID, 
				"market", bookMsg.Market,
				"using_as_condition_id", conditionID)
		}

		// Extract mid-price and aggregate OHLCV using condition ID
		midPrice := ExtractMidPrice(bids, asks)
		if midPrice > 0 {
			// Parse timestamp (assuming it's in milliseconds)
			timestampMs, err := strconv.ParseInt(bookMsg.Timestamp, 10, 64)
			if err == nil {
				// Log timestamp details for debugging (first few messages only)
				if messageCount <= 5 {
					// Try both interpretations: milliseconds and seconds
					timestampAsMs := time.Unix(timestampMs/1000, (timestampMs%1000)*1000000).UTC()
					timestampAsSec := time.Unix(timestampMs, 0).UTC()
					now := time.Now().UTC()
					
					s.logger.Info("🔍 timestamp debugging",
						"raw_timestamp_string", bookMsg.Timestamp,
						"parsed_as_int64", timestampMs,
						"interpreted_as_ms", timestampAsMs.Format(time.RFC3339),
						"interpreted_as_sec", timestampAsSec.Format(time.RFC3339),
						"current_time_utc", now.Format(time.RFC3339),
						"diff_from_now_ms", now.Sub(timestampAsMs),
						"diff_from_now_sec", now.Sub(timestampAsSec),
						"message_count", messageCount)
				}
				
				// time.Unix returns UTC, but we'll explicitly convert to UTC to be safe
				// Assuming milliseconds: divide by 1000 for seconds, remainder for nanoseconds
				timestamp := time.Unix(timestampMs/1000, (timestampMs%1000)*1000000).UTC()
				
				// Validate timestamp: if it's more than 1 hour old or more than 1 hour in the future, use current time
				// This handles cases where WebSocket timestamps might be stale or incorrectly formatted
				now := time.Now().UTC()
				timeDiff := now.Sub(timestamp)
				if timeDiff > 1*time.Hour || timeDiff < -1*time.Hour {
					if messageCount <= 5 {
						s.logger.Warn("⚠️  WebSocket timestamp seems invalid, using current time",
							"websocket_timestamp", timestamp.Format(time.RFC3339),
							"time_diff", timeDiff,
							"using_current_time", now.Format(time.RFC3339))
					}
					timestamp = now
				}
				
				// Use conditionID for OHLCV aggregation to ensure bars are stored under the correct market ID
				if err := s.ohlcvAggregator.UpdatePrice(conditionID, midPrice, timestamp); err != nil {
					s.logger.Error("failed to update OHLCV", "error", err, "condition_id", conditionID, "asset_id", bookMsg.AssetID)
				}
			} else {
				s.logger.Warn("failed to parse timestamp", "timestamp", bookMsg.Timestamp, "error", err)
			}
		} else {
			s.logger.Debug("mid-price is 0, skipping OHLCV update", "condition_id", conditionID)
		}

		// Convert to our format
		data := map[string]interface{}{
			"event_type": bookMsg.EventType,
			"asset_id":   bookMsg.AssetID,
			"market":     conditionID, // Use condition ID for the market field
			"bids":       bids,
			"asks":       asks,
			"timestamp":  bookMsg.Timestamp,
			"hash":       bookMsg.Hash,
		}

		payload, err := json.Marshal(data)
		if err != nil {
			s.logger.Error("failed to marshal order book data", "error", err)
			return err
		}
		
		// Publish to Redis channel using condition ID
		channel := "market:" + conditionID
		if err := s.redisClient.Publish(s.ctx, channel, payload).Err(); err != nil {
			s.logger.Error("failed to publish data to redis", "error", err, "channel", channel)
			return err
		}

		// Only log first few publishes to avoid spam
		if messageCount <= 3 {
			s.logger.Info("📤 published to Redis", 
				"condition_id", conditionID,
				"condition_id_length", len(conditionID),
				"asset_id", bookMsg.AssetID,
				"channel", channel,
				"channel_length", len(channel))
		}
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
				
				// Extract mid-price and aggregate OHLCV
				bids := data["bids"].([]interface{})
				asks := data["asks"].([]interface{})
				midPrice := ExtractMidPrice(bids, asks)
				if midPrice > 0 {
					timestamp := time.Now()
					if err := s.ohlcvAggregator.UpdatePrice(market.Market, midPrice, timestamp); err != nil {
						s.logger.Error("failed to update OHLCV", "error", err, "market", market.Market)
					}
				}

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

