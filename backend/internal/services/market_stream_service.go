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
	redisClient      *redis.Client
	logger           *slog.Logger
	ctx              context.Context
	wsClient         *polymarket.CLOBWebSocketClient
	config           config.Config
	ohlcvAggregator  *OHLCVAggregator
	gammaClient      *polymarket.GammaAPIClient
	tokenMap         map[string]map[string]string // condition_id -> token_id -> token_type ("yes" or "no")
	lastTradedPrices map[string]float64            // asset_id -> last_traded_price (tracked from last_trade_price events)
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
		redisClient:      redisClient,
		logger:           logger,
		ctx:              ctx,
		wsClient:         wsClient,
		config:           cfg,
		ohlcvAggregator:  ohlcvAggregator,
		gammaClient:      gammaClient,
		tokenMap:         make(map[string]map[string]string),
		lastTradedPrices: make(map[string]float64),
	}
}

// identifyTokenType determines if a token is YES or NO based on market data
func (s *MarketStreamService) identifyTokenType(ctx context.Context, conditionID, tokenID string) string {
	// Check if we already have this mapping cached (this should be the common case after initialization)
	if tokenTypes, exists := s.tokenMap[conditionID]; exists {
		if tokenType, found := tokenTypes[tokenID]; found {
			return tokenType
		}
		// Token not in cache but condition exists - log for debugging
		s.logger.Debug("token not in cache for condition", "condition_id", conditionID, "token_id", tokenID, "cached_tokens", len(tokenTypes))
	}

	// Fallback: Fetch market data to identify token types (should rarely happen if initialization worked)
	// This is a fallback for markets that weren't in the initial fetch or for new markets
	market, err := s.gammaClient.GetMarketByConditionID(ctx, conditionID)
	if err != nil {
		s.logger.Warn("failed to fetch market data for token identification", "condition_id", conditionID, "token_id", tokenID, "error", err)
		return "unknown"
	}

	// Parse clobTokenIds - format is ["NO_TOKEN_ID", "YES_TOKEN_ID"]
	if market.ClobTokenIds != "" {
		var tokenIds []string
		// Try parsing as JSON array first
		if err := json.Unmarshal([]byte(market.ClobTokenIds), &tokenIds); err != nil {
			// Try parsing as comma-separated string
			parts := strings.Split(market.ClobTokenIds, ",")
			for _, part := range parts {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					tokenIds = append(tokenIds, trimmed)
				}
			}
		}
		
		if len(tokenIds) >= 2 {
			noTokenID := tokenIds[0]
			yesTokenID := tokenIds[1]

			// Initialize token map for this condition
			if s.tokenMap[conditionID] == nil {
				s.tokenMap[conditionID] = make(map[string]string)
			}

			// Store mappings
			s.tokenMap[conditionID][noTokenID] = "no"
			s.tokenMap[conditionID][yesTokenID] = "yes"

			// Return the type for the requested token
			if tokenID == noTokenID {
				return "no"
			} else if tokenID == yesTokenID {
				return "yes"
			} else {
				s.logger.Warn("token not found in parsed IDs", "token_id", tokenID, "condition_id", conditionID, "parsed_tokens", tokenIds)
			}
		} else {
			s.logger.Warn("ClobTokenIds has insufficient tokens", "condition_id", conditionID, "token_count", len(tokenIds), "clob_token_ids", market.ClobTokenIds)
		}
	} else {
		s.logger.Warn("ClobTokenIds is empty", "condition_id", conditionID, "token_id", tokenID)
	}

	return "unknown"
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
 * - If WebSocket connection fails, existing OHLCV data persists and connection is retried.
 * - No mock data is generated - only real market data is used.
 */
func (s *MarketStreamService) RunStream() {
	if s.wsClient == nil {
		s.logger.Error("CLOB WebSocket client not configured - cannot stream market data")
		s.logger.Info("existing OHLCV data will persist, but no new updates will be received until WebSocket is configured")
		return
	}

	s.logger.Info("starting Polymarket CLOB WebSocket stream service...")

	// Retry connection with exponential backoff
	maxRetries := 5
	retryDelay := 5 * time.Second
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Connect to WebSocket
		if err := s.wsClient.Connect(); err != nil {
			s.logger.Error("failed to connect to CLOB WebSocket", 
				"error", err, 
				"attempt", attempt, 
				"max_retries", maxRetries)
			
			if attempt < maxRetries {
				s.logger.Info("retrying connection", 
					"attempt", attempt+1, 
					"delay", retryDelay)
				time.Sleep(retryDelay)
				retryDelay *= 2 // Exponential backoff
				continue
			} else {
				s.logger.Error("failed to connect after all retries - existing data will persist, but no new updates will be received")
				s.logger.Info("will retry connection in background")
				// Start a background goroutine to retry connection periodically
				go s.retryConnection()
				return
			}
		}
		
		// Connection successful, break out of retry loop
		break
	}
	defer s.wsClient.Close()

	// Fetch active markets from Gamma API and extract token IDs
	// Also create a mapping from asset/token ID to condition ID for publishing to correct Redis channels
	var assetIDs []string
	assetIDToConditionID := make(map[string]string) // Map asset ID -> condition ID
	
	if s.gammaClient != nil {
		s.logger.Info("fetching active markets from Gamma API to subscribe to WebSocket...")
		
		// Fetch top active markets by volume (Gamma API already sorts by volumeNum desc)
		// Get more markets and take the top 100 by volume for monitoring
		markets, err := s.gammaClient.ListActiveMarkets(s.ctx, 500, 0) // Fetch more to ensure we get top volume markets
		if err == nil && len(markets) > 100 {
			markets = markets[:100] // Take only top 100 by volume
		}
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
				// Initialize token map for this condition if not already done
				if s.tokenMap[market.ConditionID] == nil {
					s.tokenMap[market.ConditionID] = make(map[string]string)
				}
				
				// Parse token types: ClobTokenIds format is ["NO_TOKEN_ID", "YES_TOKEN_ID"]
				// So index 0 is NO, index 1 is YES
				for idx, tokenID := range tokenIDs {
					assetIDToConditionID[tokenID] = market.ConditionID
					
					// Determine token type based on position in array
					// Polymarket convention: [NO, YES] or sometimes just 2 tokens where first is NO, second is YES
					if len(tokenIDs) >= 2 {
						if idx == 0 {
							s.tokenMap[market.ConditionID][tokenID] = "no"
						} else if idx == 1 {
							s.tokenMap[market.ConditionID][tokenID] = "yes"
						} else {
							// For additional tokens, default to unknown
							s.tokenMap[market.ConditionID][tokenID] = "unknown"
						}
					} else {
						// If we only have one token, we can't determine type
						s.tokenMap[market.ConditionID][tokenID] = "unknown"
					}
				}
				// Add all token IDs found for this market
				assetIDs = append(assetIDs, tokenIDs...)
			}
		}
	// Count YES and NO tokens for logging
	yesTokenCount := 0
	noTokenCount := 0
	unknownTokenCount := 0
	for _, tokenTypes := range s.tokenMap {
		for _, tokenType := range tokenTypes {
			switch tokenType {
			case "yes":
				yesTokenCount++
			case "no":
				noTokenCount++
			default:
				unknownTokenCount++
			}
		}
	}
	
	s.logger.Info("✅ extracted token IDs from Gamma API markets", 
		"market_count", len(markets),
		"markets_with_tokens", marketsWithTokens,
		"markets_without_tokens", marketsWithoutTokens,
		"total_token_ids", len(assetIDs),
		"mapping_size", len(assetIDToConditionID),
		"yes_tokens", yesTokenCount,
		"no_tokens", noTokenCount,
		"unknown_tokens", unknownTokenCount)
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

		// Filter out bids/asks with empty or invalid prices before processing
		// This prevents NaN values from being sent to the frontend
		validBids := make([]polymarket.OrderLevel, 0, len(bookMsg.Bids))
		for _, bid := range bookMsg.Bids {
			if bid.Price != "" && bid.Size != "" {
				// Validate that price can be parsed as a number
				if _, err := strconv.ParseFloat(bid.Price, 64); err == nil {
					validBids = append(validBids, bid)
				}
			}
		}
		
		validAsks := make([]polymarket.OrderLevel, 0, len(bookMsg.Asks))
		for _, ask := range bookMsg.Asks {
			if ask.Price != "" && ask.Size != "" {
				// Validate that price can be parsed as a number
				if _, err := strconv.ParseFloat(ask.Price, 64); err == nil {
					validAsks = append(validAsks, ask)
				}
			}
		}

		// Convert valid bids/asks to interface{} for ExtractMidPrice
		bids := make([]interface{}, len(validBids))
		for i, bid := range validBids {
			bids[i] = map[string]interface{}{
				"price": bid.Price,
				"size":  bid.Size,
			}
		}
		asks := make([]interface{}, len(validAsks))
		for i, ask := range validAsks {
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

		// Identify token type BEFORE any processing
		tokenType := s.identifyTokenType(s.ctx, conditionID, bookMsg.AssetID)

		// CRITICAL FIX: Only aggregate OHLCV for YES tokens to prevent price mixing
		shouldAggregateOHLCV := tokenType == "yes"

		// Extract mid-price, spread, and volume from order book
		// According to Polymarket: use mid-price unless spread > $0.10, then use last traded price
		midPrice, spread := ExtractMidPrice(bids, asks)
		volume := ExtractVolumeWithLogging(bids, asks, s.logger, messageCount) // Extract volume (0 for order book updates, actual size for trades)

		// Detect if this is a trade event (last_trade_price converted to BookMessage)
		// Trade events have identical bid and ask prices/sizes
		isTradeEvent := volume > 0 && len(validBids) == 1 && len(validAsks) == 1 &&
			validBids[0].Price == validAsks[0].Price && validBids[0].Size == validAsks[0].Size
		
		// Track last traded price from trade events
		// This is used when spread > 0.10 per Polymarket's pricing rules
		if isTradeEvent && shouldAggregateOHLCV {
			if tradePrice, err := strconv.ParseFloat(validBids[0].Price, 64); err == nil {
				s.lastTradedPrices[bookMsg.AssetID] = tradePrice
				// Only log first few or occasionally to reduce noise
				if messageCount <= 10 || messageCount%5000 == 0 {
					s.logger.Debug("💰 tracked last traded price from trade event",
						"condition_id", conditionID,
						"asset_id", bookMsg.AssetID,
						"last_traded_price", tradePrice,
						"message_count", messageCount)
				}
			}
		}

		// CRITICAL: For YES tokens in active markets, we MUST have both bid and ask to calculate valid mid-price
		// Single-sided order books are unreliable and shouldn't be used for OHLCV aggregation
		hasBothSides := len(validBids) > 0 && len(validAsks) > 0
		
		// CRITICAL DEBUGGING: Log raw prices for YES tokens, especially when price is suspicious (0.5 or near 0.5)
		// This helps identify if we're processing wrong tokens or getting invalid prices
		// Reduced verbosity: only log first 10 messages or when there's an actual issue
		if shouldAggregateOHLCV && (messageCount <= 10 || (midPrice >= 0.49 && midPrice <= 0.51 && spread > 0.10)) {
			var bestBidStr, bestAskStr string
			if len(validBids) > 0 {
				bestBidStr = validBids[0].Price
			}
			if len(validAsks) > 0 {
				bestAskStr = validAsks[0].Price
			}
			s.logger.Warn("🔍 DEBUG: YES token price extraction",
				"condition_id", conditionID,
				"asset_id", bookMsg.AssetID,
				"token_type", tokenType,
				"best_bid", bestBidStr,
				"best_ask", bestAskStr,
				"bids_count", len(validBids),
				"asks_count", len(validAsks),
				"has_both_sides", hasBothSides,
				"calculated_mid_price", midPrice,
				"spread", spread,
				"message_count", messageCount,
				"is_suspicious_0_5", midPrice >= 0.49 && midPrice <= 0.51)
		}

		// Validate price: Polymarket prices should be between 0.001 and 0.999 (probabilities)
		// Reject extreme prices that might be from incorrect token parsing
		// CRITICAL: For YES tokens, require both bid and ask for valid price calculation
		// Note: If spread > 0.10, we should ideally use last traded price, but for now we'll use mid-price
		// as we're tracking order book updates. Last traded price would come from trade events.
		if midPrice > 0 && midPrice >= 0.001 && midPrice <= 0.999 {
			// For YES tokens, reject prices calculated from single-sided order books
			if shouldAggregateOHLCV && !hasBothSides {
				if messageCount <= 20 || messageCount%1000 == 0 {
					s.logger.Warn("⚠️  REJECTED: YES token price from single-sided order book (missing bid or ask)",
						"condition_id", conditionID,
						"asset_id", bookMsg.AssetID,
						"bids_count", len(validBids),
						"asks_count", len(validAsks),
						"calculated_price", midPrice,
						"message_count", messageCount)
				}
				// Skip this update - don't aggregate OHLCV for single-sided prices
				return nil
			}
				// Parse timestamp (Polymarket CLOB WebSocket sends timestamps in MILLISECONDS since epoch)
				timestampMs, err := strconv.ParseInt(bookMsg.Timestamp, 10, 64)
				if err == nil {
					// Convert milliseconds to time.Time
					// time.UnixMilli() is available in Go 1.17+, but we'll use the compatible approach
					timestamp := time.Unix(timestampMs/1000, (timestampMs%1000)*1000000).UTC()
					
					// Validate timestamp: only replace timestamps that are clearly in the future
					// Polymarket timestamps should be current, but allow some tolerance for network delays
					now := time.Now().UTC()
					timeDiff := now.Sub(timestamp)
					
					// Log timestamp details for debugging (first few messages only)
					if messageCount <= 5 {
						s.logger.Info("🔍 timestamp debugging",
							"raw_timestamp_string", bookMsg.Timestamp,
							"parsed_as_milliseconds", timestampMs,
							"interpreted_timestamp", timestamp.Format(time.RFC3339),
							"current_time_utc", now.Format(time.RFC3339),
							"time_diff", timeDiff,
							"message_count", messageCount)
					}

				// Only replace timestamps that are more than 1 minute in the future
				if timeDiff < -1*time.Minute {
					if messageCount <= 10 || messageCount%1000 == 0 {
						s.logger.Warn("⚠️  WebSocket timestamp is in future, replaced with current time",
							"websocket_timestamp", timestamp.Format(time.RFC3339),
							"time_diff", timeDiff,
							"using_current_time", now.Format(time.RFC3339),
							"message_count", messageCount)
					}
					timestamp = now
				}
				
				// CRITICAL: Only aggregate OHLCV for YES tokens to prevent price mixing
				// YES token represents the market's implied probability
				if shouldAggregateOHLCV {
					// POLYMARKET RULE: Use last traded price when spread > $0.10
					// Per Polymarket docs: "The prices displayed are the midpoint of the bid-ask spread
					// in the orderbook — unless that spread is over $0.10, in which case the last traded price is used."
					finalPrice := midPrice
					priceSource := "midpoint"
					
					if spread > 0.10 {
						// Wide spread detected - use last traded price if available
						// Use the most recent last traded price we've seen (even if from a previous message)
						if lastTradedPrice, hasLastTrade := s.lastTradedPrices[bookMsg.AssetID]; hasLastTrade && lastTradedPrice > 0 {
							finalPrice = lastTradedPrice
							priceSource = "last_traded"
							// Only log occasionally to reduce noise
							if messageCount <= 10 || messageCount%5000 == 0 {
								s.logger.Info("✅ using last traded price due to wide spread (Polymarket rule)",
									"condition_id", conditionID,
									"asset_id", bookMsg.AssetID,
									"spread", spread,
									"mid_price", midPrice,
									"last_traded_price", lastTradedPrice,
									"message_count", messageCount)
							}
						} else {
							// Wide spread but no last traded price available yet
							// This is normal at startup - we'll get last traded prices from trade events soon
							// Only log occasionally to reduce noise
							if messageCount <= 10 || messageCount%5000 == 0 {
								s.logger.Debug("⏳ wide spread detected, waiting for last traded price",
									"condition_id", conditionID,
									"asset_id", bookMsg.AssetID,
									"spread", spread,
									"mid_price", midPrice,
									"message_count", messageCount,
									"note", "This is normal at startup - last traded prices will be available after trade events")
							}
							// Skip this update - don't use unreliable mid-price when spread is too wide
							return nil
						}
					}
					
					// CRITICAL VALIDATION: Reject prices exactly at 0.5 for YES tokens in active markets
					// Top 100 markets by volume should NOT have YES prices at exactly 0.5 (perfectly balanced)
					// This is a strong indicator of incorrect token identification or invalid price data
					// BUT: Allow 0.5 if it came from a valid last traded price (unlikely but possible)
					if finalPrice >= 0.499 && finalPrice <= 0.501 && priceSource == "midpoint" {
						var bestBidStr, bestAskStr string
						if len(validBids) > 0 {
							bestBidStr = validBids[0].Price
						} else {
							bestBidStr = "none"
						}
						if len(validAsks) > 0 {
							bestAskStr = validAsks[0].Price
						} else {
							bestAskStr = "none"
						}
						s.logger.Error("❌ REJECTED: YES token price is exactly 0.5 from midpoint - likely invalid data or wrong token",
							"condition_id", conditionID,
							"asset_id", bookMsg.AssetID,
							"token_type", tokenType,
							"mid_price", midPrice,
							"best_bid", bestBidStr,
							"best_ask", bestAskStr,
							"bids_count", len(validBids),
							"asks_count", len(validAsks),
							"message_count", messageCount)
						// Skip this update - don't aggregate OHLCV for suspicious 0.5 prices
						return nil
					}
					
					// Log first few OHLCV updates to confirm aggregation is working
					if messageCount <= 10 || messageCount%100 == 0 {
						s.logger.Info("📊 aggregating OHLCV for YES token", 
							"condition_id", conditionID, 
							"asset_id", bookMsg.AssetID, 
							"price", finalPrice,
							"price_source", priceSource,
							"mid_price", midPrice,
							"spread", spread,
							"volume", volume,
							"is_trade", volume > 0,
							"token_type", tokenType,
							"message_count", messageCount)
					}
					if err := s.ohlcvAggregator.UpdatePrice(conditionID, finalPrice, volume, timestamp); err != nil {
						s.logger.Error("failed to update OHLCV", "error", err, "condition_id", conditionID, "asset_id", bookMsg.AssetID, "token_type", tokenType)
					}
				} else {
					// Only log first few skips to avoid spam
					if messageCount <= 10 {
						s.logger.Info("⏭️  skipping OHLCV aggregation for non-YES token", 
							"asset_id", bookMsg.AssetID, 
							"token_type", tokenType, 
							"condition_id", conditionID,
							"message_count", messageCount)
					}
				}
			} else {
				s.logger.Warn("failed to parse timestamp", "timestamp", bookMsg.Timestamp, "error", err)
			}
		} else {
			if messageCount <= 10 {
				s.logger.Debug("mid-price out of valid range, skipping OHLCV update", "price", midPrice, "message_count", messageCount)
			}
		}

		// Convert valid bids/asks to the format expected by frontend
		frontendBids := make([]map[string]interface{}, len(validBids))
		for i, bid := range validBids {
			frontendBids[i] = map[string]interface{}{
				"price": bid.Price,
				"size":  bid.Size,
			}
		}
		frontendAsks := make([]map[string]interface{}, len(validAsks))
		for i, ask := range validAsks {
			frontendAsks[i] = map[string]interface{}{
				"price": ask.Price,
				"size":  ask.Size,
			}
		}

		// Convert to our format with token separation
		data := map[string]interface{}{
			"event_type": bookMsg.EventType,
			"asset_id":   bookMsg.AssetID,
			"token_type": tokenType, // "yes", "no", or "unknown"
			"market":     conditionID, // Use condition ID for the market field
			"bids":       frontendBids,
			"asks":       frontendAsks,
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
		s.logger.Info("existing OHLCV data will persist, attempting to reconnect...")
		// Attempt to reconnect after a delay
		time.Sleep(5 * time.Second)
		s.RunStream() // Recursive call to reconnect
	}
}

// retryConnection periodically attempts to reconnect to the WebSocket
// This runs in the background when initial connection fails
func (s *MarketStreamService) retryConnection() {
	ticker := time.NewTicker(30 * time.Second) // Retry every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("stopping connection retry")
			return
		case <-ticker.C:
			s.logger.Info("attempting to reconnect to WebSocket...")
			s.RunStream() // This will handle its own retries
			// If RunStream returns, connection was successful or we should stop
			return
		}
	}
}

/**
 * @description
 * RunMockStream simulates a connection to an external market data feed.
 * It periodically generates fake order book data for a predefined set of markets
 * and publishes it to the corresponding Redis channels.
 *
 * @notes
 * - DEPRECATED: This function is no longer used. We persist existing data instead of using mock data.
 * - Kept for reference but should not be called.
 * - It runs in an infinite loop and should be started as a goroutine.
 * - It gracefully handles shutdown via the context.
 */
func (s *MarketStreamService) RunMockStream() {
	s.logger.Warn("RunMockStream called but mock streams are disabled - existing data will persist")
	// Mock streams are disabled - we persist existing OHLCV data instead of generating fake data
	// This ensures data integrity and prevents contamination of real market data
	return
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

