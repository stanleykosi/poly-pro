/**
 * @description
 * This service is responsible for aggregating real-time order book data into OHLCV (Open, High, Low, Close, Volume) bars.
 * It maintains in-memory state for each market and time period, aggregating order book updates into bars.
 *
 * Key features:
 * - OHLCV Aggregation: Converts order book updates (bids/asks) into OHLCV bars.
 * - Time-based Bucketing: Groups price updates into time buckets (1m, 5m, 15m, 1h, 1d, etc.).
 * - In-memory State: Maintains current bar state for each market/resolution combination.
 * - Database Storage: Stores completed bars in the database.
 *
 * @dependencies
 * - github.com/poly-pro/backend/internal/db: For database access.
 * - log/slog: For structured logging.
 */

package services

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/poly-pro/backend/internal/db"
)

// OHLCVAggregator aggregates order book data into OHLCV bars.
type OHLCVAggregator struct {
	store  db.Querier
	logger *slog.Logger
	ctx    context.Context

	// In-memory state: market_id -> resolution -> current bar
	bars map[string]map[string]*CurrentBar
	mu   sync.RWMutex
	
	// Track statistics
	totalUpdates   int64
	totalBarsSaved int64
	lastStatusLog  time.Time
}

// CurrentBar represents a bar that is currently being aggregated.
type CurrentBar struct {
	MarketID   string
	Resolution string
	StartTime  time.Time
	Open        float64
	High        float64
	Low         float64
	Close       float64
	Volume      float64
	Count       int64 // Number of updates in this bar
}

// NewOHLCVAggregator creates a new OHLCV aggregator.
func NewOHLCVAggregator(ctx context.Context, logger *slog.Logger, store db.Querier) *OHLCVAggregator {
	agg := &OHLCVAggregator{
		store:        store,
		logger:       logger,
		ctx:          ctx,
		bars:         make(map[string]map[string]*CurrentBar),
		lastStatusLog: time.Now(),
	}
	
	// Start periodic status logging
	go agg.periodicStatusLog()
	
	return agg
}

// UpdatePrice processes a price update for a market and updates the current bar.
// It extracts the mid-price from the order book (average of best bid and ask).
func (a *OHLCVAggregator) UpdatePrice(marketID string, price float64, timestamp time.Time) error {
	a.totalUpdates++
	
	// Log first few updates to confirm function is being called
	if a.totalUpdates <= 5 {
		a.logger.Info("🔄 OHLCV aggregator UpdatePrice called", 
			"update_number", a.totalUpdates,
			"market_id", marketID,
			"price", price,
			"timestamp", timestamp.Format(time.RFC3339))
	}
	
	// Update all resolutions for this market
	resolutions := []string{"1", "5", "15", "60", "D"}

	for _, resolution := range resolutions {
		if err := a.updateBarForResolution(marketID, resolution, price, timestamp); err != nil {
			a.logger.Error("failed to update bar", "market_id", marketID, "resolution", resolution, "error", err)
			return err
		}
	}

	return nil
}

// updateBarForResolution updates the bar for a specific market and resolution.
func (a *OHLCVAggregator) updateBarForResolution(marketID string, resolution string, price float64, timestamp time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Get or create the bar map for this market
	if a.bars[marketID] == nil {
		a.bars[marketID] = make(map[string]*CurrentBar)
	}

	// Calculate the start time for this bar based on resolution
	barStartTime := a.getBarStartTime(timestamp, resolution)

	// Get or create the current bar
	bar, exists := a.bars[marketID][resolution]
	if !exists || bar.StartTime.Before(barStartTime) {
		// If the bar doesn't exist or we've moved to a new time period, save the old bar and create a new one
		if exists {
			a.logger.Info("bar period complete, saving to database", 
				"market_id", bar.MarketID, 
				"resolution", bar.Resolution, 
				"start_time", bar.StartTime.Format(time.RFC3339),
				"open", bar.Open,
				"high", bar.High,
				"low", bar.Low,
				"close", bar.Close,
				"updates_count", bar.Count)
			if err := a.saveBar(bar); err != nil {
				return err
			}
		}

		// Create a new bar
		bar = &CurrentBar{
			MarketID:   marketID,
			Resolution: resolution,
			StartTime:  barStartTime,
			Open:       price,
			High:       price,
			Low:        price,
			Close:      price,
			Volume:     0, // Volume would need to come from trade data
			Count:      0,
		}
		a.bars[marketID][resolution] = bar
		a.logger.Info("🆕 created new OHLCV bar", 
			"market_id", marketID, 
			"resolution", resolution, 
			"start_time", barStartTime.Format(time.RFC3339),
			"initial_price", price,
			"next_save_time", a.getNextSaveTime(barStartTime, resolution).Format(time.RFC3339))
	}

	// Update the bar with the new price
	bar.Close = price
	if price > bar.High {
		bar.High = price
	}
	if price < bar.Low {
		bar.Low = price
	}
	bar.Count++

	// Log first few updates per bar to show aggregation is working
	if bar.Count <= 3 {
		a.logger.Debug("updating OHLCV bar", 
			"market_id", marketID, 
			"resolution", resolution, 
			"update_count", bar.Count,
			"current_price", price,
			"bar_high", bar.High,
			"bar_low", bar.Low)
	}

	return nil
}

// getBarStartTime calculates the start time of the bar for a given timestamp and resolution.
func (a *OHLCVAggregator) getBarStartTime(timestamp time.Time, resolution string) time.Time {
	switch resolution {
	case "1": // 1 minute
		return timestamp.Truncate(time.Minute)
	case "5": // 5 minutes
		return timestamp.Truncate(5 * time.Minute)
	case "15": // 15 minutes
		return timestamp.Truncate(15 * time.Minute)
	case "60": // 1 hour
		return timestamp.Truncate(time.Hour)
	case "D": // 1 day
		return time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, timestamp.Location())
	default:
		return timestamp.Truncate(time.Hour)
	}
}

// saveBar saves a completed bar to the database.
func (a *OHLCVAggregator) saveBar(bar *CurrentBar) error {
	// Convert to database types
	var timeVal pgtype.Timestamptz
	if err := timeVal.Scan(bar.StartTime); err != nil {
		return err
	}

	var openVal, highVal, lowVal, closeVal, volumeVal pgtype.Numeric
	if err := openVal.Scan(bar.Open); err != nil {
		return err
	}
	if err := highVal.Scan(bar.High); err != nil {
		return err
	}
	if err := lowVal.Scan(bar.Low); err != nil {
		return err
	}
	if err := closeVal.Scan(bar.Close); err != nil {
		return err
	}
	if err := volumeVal.Scan(bar.Volume); err != nil {
		return err
	}

	// Insert into database
	arg := db.InsertMarketPriceHistoryParams{
		PTime:       timeVal,
		PMarketID:   bar.MarketID,
		POpen:       openVal,
		PHigh:       highVal,
		PLow:        lowVal,
		PClose:      closeVal,
		PVolume:     volumeVal,
		PResolution: bar.Resolution,
	}

	if err := a.store.InsertMarketPriceHistory(a.ctx, arg); err != nil {
		a.logger.Error("failed to insert market price history", "error", err, "market_id", bar.MarketID, "resolution", bar.Resolution)
		return err
	}

	a.totalBarsSaved++
	a.logger.Info("✅ OHLCV bar saved to database", 
		"market_id", bar.MarketID, 
		"resolution", bar.Resolution, 
		"time", bar.StartTime.Format(time.RFC3339),
		"open", bar.Open,
		"high", bar.High,
		"low", bar.Low,
		"close", bar.Close,
		"updates_count", bar.Count,
		"total_bars_saved", a.totalBarsSaved)
	return nil
}

// FlushAll flushes all current bars to the database.
// This should be called periodically or on shutdown.
func (a *OHLCVAggregator) FlushAll() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for marketID, resolutions := range a.bars {
		for resolution, bar := range resolutions {
			if err := a.saveBar(bar); err != nil {
				a.logger.Error("failed to flush bar", "market_id", marketID, "resolution", resolution, "error", err)
				return err
			}
		}
	}

	return nil
}

// ExtractMidPrice extracts the mid-price from order book data (bids and asks).
// Returns the average of the best bid and best ask, or 0 if no data is available.
func ExtractMidPrice(bids []interface{}, asks []interface{}) float64 {
	var bestBid, bestAsk float64
	var hasBid, hasAsk bool

	// Extract best bid (highest price)
	if len(bids) > 0 {
		if bidMap, ok := bids[0].(map[string]interface{}); ok {
			if priceStr, ok := bidMap["price"].(string); ok {
				if price, err := parseFloat(priceStr); err == nil {
					bestBid = price
					hasBid = true
				}
			}
		}
	}

	// Extract best ask (lowest price)
	if len(asks) > 0 {
		if askMap, ok := asks[0].(map[string]interface{}); ok {
			if priceStr, ok := askMap["price"].(string); ok {
				if price, err := parseFloat(priceStr); err == nil {
					bestAsk = price
					hasAsk = true
				}
			}
		}
	}

	// Calculate mid-price
	if hasBid && hasAsk {
		return (bestBid + bestAsk) / 2.0
	} else if hasBid {
		return bestBid
	} else if hasAsk {
		return bestAsk
	}

	return 0
}

// parseFloat is a helper to parse string to float64.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

// getNextSaveTime calculates when the next bar for this resolution will be saved.
func (a *OHLCVAggregator) getNextSaveTime(barStartTime time.Time, resolution string) time.Time {
	switch resolution {
	case "1": // 1 minute
		return barStartTime.Add(1 * time.Minute)
	case "5": // 5 minutes
		return barStartTime.Add(5 * time.Minute)
	case "15": // 15 minutes
		return barStartTime.Add(15 * time.Minute)
	case "60": // 1 hour
		return barStartTime.Add(1 * time.Hour)
	case "D": // 1 day
		return barStartTime.Add(24 * time.Hour)
	default:
		return barStartTime.Add(1 * time.Hour)
	}
}

// periodicStatusLog logs the current state of all bars periodically.
func (a *OHLCVAggregator) periodicStatusLog() {
	ticker := time.NewTicker(30 * time.Second) // Log every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.logStatus()
		}
	}
}

// logStatus logs the current state of all bars in memory.
func (a *OHLCVAggregator) logStatus() {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.bars) == 0 {
		a.logger.Warn("⚠️  OHLCV aggregator status: no bars in memory - no price updates received yet",
			"total_updates", a.totalUpdates,
			"total_bars_saved", a.totalBarsSaved)
		return
	}

	// Count bars by resolution
	barCounts := make(map[string]int)
	var barDetails []map[string]interface{}

	for marketID, resolutions := range a.bars {
		for resolution, bar := range resolutions {
			barCounts[resolution]++
			nextSaveTime := a.getNextSaveTime(bar.StartTime, resolution)
			timeUntilSave := time.Until(nextSaveTime)
			
			barDetails = append(barDetails, map[string]interface{}{
				"market_id":      marketID,
				"resolution":     resolution,
				"start_time":     bar.StartTime.Format(time.RFC3339),
				"next_save_time": nextSaveTime.Format(time.RFC3339),
				"time_until_save": timeUntilSave.String(),
				"updates_count":  bar.Count,
				"current_price":  bar.Close,
			})
		}
	}

	a.logger.Info("📊 OHLCV aggregator status",
		"total_updates", a.totalUpdates,
		"total_bars_saved", a.totalBarsSaved,
		"active_markets", len(a.bars),
		"active_bars", barCounts,
		"bars", barDetails)
}

