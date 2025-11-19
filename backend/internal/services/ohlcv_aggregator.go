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
	"fmt"
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
	totalUpdates     int64
	totalBarsSaved   int64
	lastStatusLog    time.Time
	lastCleanupTime  time.Time
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
	
	// Test database connection by running a simple query
	testCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	
	// Try to query for any existing data to verify connection
	testParams := db.GetMarketPriceHistoryParams{
		MarketID:   "test-connection",
		Time:       pgtype.Timestamptz{Time: time.Now().Add(-24 * time.Hour), Valid: true},
		Time_2:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Resolution: "1",
	}
	_, err := store.GetMarketPriceHistory(testCtx, testParams)
	if err != nil {
		// Log error but don't fail - the query might fail if table doesn't exist or no data
		// The important thing is that we can connect to the database
		logger.Warn("⚠️  OHLCV aggregator: database connection test query failed (this may be normal if no data exists)",
			"error", err,
			"note", "This is just a connection test, not a critical error")
	} else {
		logger.Info("✅ OHLCV aggregator: database connection verified")
	}
	
	// Start periodic status logging
	go agg.periodicStatusLog()
	
	// Start periodic flush of completed bars
	go agg.periodicFlush()
	
	return agg
}

// UpdatePrice processes a price update for a market and updates the current bar.
// It extracts the mid-price from the order book (average of best bid and ask).
func (a *OHLCVAggregator) UpdatePrice(marketID string, price float64, volume float64, timestamp time.Time) error {
	a.totalUpdates++

	// Log first few updates to confirm function is being called
	if a.totalUpdates <= 10 || a.totalUpdates%100 == 0 {
		a.logger.Info("OHLCV aggregator: processing price update",
			"update", a.totalUpdates,
			"market_id", marketID,
			"price", price,
			"volume", volume,
			"timestamp", timestamp.Format(time.RFC3339))
	}

	// Periodic cleanup: run cleanup every 24 hours to prevent old data accumulation
	if time.Since(a.lastCleanupTime) > 24*time.Hour {
		if err := a.cleanupOldData(); err != nil {
			a.logger.Error("failed to cleanup old data", "error", err)
		}
	}

	// Update key resolutions for this market (15m and 1d for chart visualization)
	resolutions := []string{"15", "D"}

	for _, resolution := range resolutions {
		if err := a.updateBarForResolution(marketID, resolution, price, volume, timestamp); err != nil {
			a.logger.Error("failed to update bar", "market_id", marketID, "resolution", resolution, "error", err)
			return err
		}
	}

	return nil
}

// updateBarForResolution updates the bar for a specific market and resolution.
func (a *OHLCVAggregator) updateBarForResolution(marketID string, resolution string, price float64, volume float64, timestamp time.Time) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Get or create the bar map for this market
	if a.bars[marketID] == nil {
		a.bars[marketID] = make(map[string]*CurrentBar)
	}

	// Calculate the start time for this bar based on resolution
	barStartTime := a.getBarStartTime(timestamp, resolution)

	// Log basic timestamp info only for first few updates
	if a.totalUpdates <= 5 {
		a.logger.Info("🔍 timestamp basics",
			"update", a.totalUpdates,
			"market_id", marketID,
			"resolution", resolution,
			"input_timestamp", timestamp.Format("2006-01-02 15:04:05"),
			"bar_start_time", barStartTime.Format("2006-01-02 15:04:05"))
	}

	// Additional validation: if the bar start time is more than 1 day old, log a warning
	// This helps catch cases where stale timestamps are creating bars with old dates
	now := time.Now().UTC()
	barDateDiff := now.Sub(barStartTime)
	if barDateDiff > 24*time.Hour && a.totalUpdates <= 20 {
		a.logger.Warn("⚠️  bar start time is more than 1 day old",
			"bar_start_time", barStartTime.Format(time.RFC3339),
			"bar_start_time_date", barStartTime.Format("2006-01-02"),
			"current_time", now.Format(time.RFC3339),
			"current_time_date", now.Format("2006-01-02"),
			"age", barDateDiff,
			"market_id", marketID,
			"resolution", resolution)
	}

	// Get or create the current bar
	bar, exists := a.bars[marketID][resolution]
	needsNewBar := !exists || bar.StartTime.Before(barStartTime)
	
	if needsNewBar {
		// If the bar doesn't exist or we've moved to a new time period, save the old bar and create a new one
		if exists {
			// Log when transitioning to a new time period
			if a.totalBarsSaved < 20 || a.totalUpdates%100 == 0 {
				a.logger.Info("🔄 transitioning to new time period",
					"market_id", marketID,
					"resolution", resolution,
					"old_bar_start", bar.StartTime.Format("2006-01-02 15:04:05"),
					"new_bar_start", barStartTime.Format("2006-01-02 15:04:05"),
					"old_bar_count", bar.Count,
					"old_bar_close", bar.Close)
			}
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
			Volume:     volume, // Initialize with the first volume update
			Count:      0,
		}
		a.bars[marketID][resolution] = bar

		// Log new bar creation (more frequently to debug)
		if a.totalBarsSaved < 20 || a.totalUpdates%100 == 0 {
			barEndTime := a.getBarEndTime(barStartTime, resolution)
			a.logger.Info("🆕 created new OHLCV bar",
				"market_id", marketID,
				"resolution", resolution,
				"start_time", barStartTime.Format("2006-01-02 15:04:05"),
				"end_time", barEndTime.Format("2006-01-02 15:04:05"),
				"initial_price", price,
				"initial_volume", volume)
		}
	}

	// Update the bar with the new price and accumulate volume
	bar.Close = price
	if price > bar.High {
		bar.High = price
	}
	if price < bar.Low {
		bar.Low = price
	}
	oldVolume := bar.Volume
	bar.Volume += volume // Accumulate volume
	bar.Count++
	
	// Log volume accumulation for debugging (first few updates or when volume changes)
	if a.totalUpdates <= 20 || volume > 0 || (a.totalUpdates%100 == 0 && bar.Volume > 0) {
		a.logger.Info("💰 volume accumulation",
			"market_id", marketID,
			"resolution", resolution,
			"incoming_volume", volume,
			"previous_total_volume", oldVolume,
			"new_total_volume", bar.Volume,
			"update_count", a.totalUpdates,
			"bar_count", bar.Count)
	}

	return nil
}

// getBarStartTime calculates the start time of the bar for a given timestamp and resolution.
func (a *OHLCVAggregator) getBarStartTime(timestamp time.Time, resolution string) time.Time {
	switch resolution {
	case "15": // 15 minutes
		return timestamp.Truncate(15 * time.Minute)
	case "D": // 1 day
		// Ensure we use UTC for daily bars to avoid timezone issues
		utcTimestamp := timestamp.UTC()
		return time.Date(utcTimestamp.Year(), utcTimestamp.Month(), utcTimestamp.Day(), 0, 0, 0, 0, time.UTC)
	default:
		// Default to 15 minutes for any unknown resolution
		return timestamp.Truncate(15 * time.Minute)
	}
}

// saveBar saves a completed bar to the database.
func (a *OHLCVAggregator) saveBar(bar *CurrentBar) error {
	// Ensure the timestamp is in UTC before storing
	// This prevents timezone-related issues when storing timestamps
	utcTime := bar.StartTime.UTC()
	
	// Convert to database types
	var timeVal pgtype.Timestamptz
	if err := timeVal.Scan(utcTime); err != nil {
		return err
	}
	
	// Log timestamp conversion only on errors (removed verbose logging)

	// Helper function to convert float64 to pgtype.Numeric
	// pgtype.Numeric.Scan() doesn't accept float64 directly, so we convert to string first
	convertToNumeric := func(val float64) (pgtype.Numeric, error) {
		var num pgtype.Numeric
		// Convert float64 to string with sufficient precision (10 decimal places)
		// Use 'f' format to avoid scientific notation for large numbers, which pgtype.Numeric.Scan doesn't accept
		valStr := strconv.FormatFloat(val, 'f', 10, 64)
		if err := num.Scan(valStr); err != nil {
			return num, fmt.Errorf("failed to scan %f as numeric: %w", val, err)
		}
		return num, nil
	}

	openVal, err := convertToNumeric(bar.Open)
	if err != nil {
		return fmt.Errorf("failed to convert open: %w", err)
	}
	highVal, err := convertToNumeric(bar.High)
	if err != nil {
		return fmt.Errorf("failed to convert high: %w", err)
	}
	lowVal, err := convertToNumeric(bar.Low)
	if err != nil {
		return fmt.Errorf("failed to convert low: %w", err)
	}
	closeVal, err := convertToNumeric(bar.Close)
	if err != nil {
		return fmt.Errorf("failed to convert close: %w", err)
	}
	volumeVal, err := convertToNumeric(bar.Volume)
	if err != nil {
		return fmt.Errorf("failed to convert volume: %w", err)
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

	// Log bar being saved with volume information
	a.logger.Info("💾 saving OHLCV bar to database",
		"market_id", bar.MarketID,
		"resolution", bar.Resolution,
		"time", timeVal.Time.Format("2006-01-02 15:04:05"),
		"open", bar.Open,
		"high", bar.High,
		"low", bar.Low,
		"close", bar.Close,
		"volume", bar.Volume,
		"update_count", bar.Count,
		"total_bars_saved", a.totalBarsSaved+1)

	if err := a.store.InsertMarketPriceHistory(a.ctx, arg); err != nil {
		// Log detailed error information
		a.logger.Error("❌ failed to insert market price history",
			"error", err,
			"error_type", fmt.Sprintf("%T", err),
			"market_id", bar.MarketID,
			"resolution", bar.Resolution,
			"start_time_original", bar.StartTime,
			"start_time_utc", utcTime,
			"start_time_rfc3339", utcTime.Format(time.RFC3339),
			"time_valid", timeVal.Valid,
			"open", bar.Open,
			"high", bar.High,
			"low", bar.Low,
			"close", bar.Close)
		return fmt.Errorf("database insert failed: %w", err)
	}

	// Simplified verification - only log on errors
	verifyCtx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()

	var verifyTimeStart, verifyTimeEnd pgtype.Timestamptz
	verifyTimeStart.Scan(utcTime.Add(-1 * time.Second))
	verifyTimeEnd.Scan(utcTime.Add(1 * time.Second))

	verifyParams := db.GetMarketPriceHistoryParams{
		MarketID:   bar.MarketID,
		Time:       verifyTimeStart,
		Time_2:     verifyTimeEnd,
		Resolution: bar.Resolution,
	}
	verifyResults, verifyErr := a.store.GetMarketPriceHistory(verifyCtx, verifyParams)
	if verifyErr != nil {
		a.logger.Warn("⚠️  insert verification failed", "error", verifyErr, "market_id", bar.MarketID)
	} else if len(verifyResults) == 0 {
		a.logger.Error("❌ insert succeeded but data not found", "market_id", bar.MarketID, "resolution", bar.Resolution)
	}

	a.totalBarsSaved++

	// Log successful bar save (simplified)
	a.logger.Info("✅ OHLCV bar saved",
		"market_id", bar.MarketID,
		"resolution", bar.Resolution,
		"start_time", utcTime.Format("2006-01-02 15:04:05"),
		"total_saved", a.totalBarsSaved)
	
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
// According to Polymarket docs: "The prices displayed are the midpoint of the bid-ask spread 
// in the orderbook — unless that spread is over $0.10, in which case the last traded price is used."
// Returns the calculated price, or 0 if no data is available.
func ExtractMidPrice(bids []interface{}, asks []interface{}) (float64, float64) {
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

	// Calculate mid-price and spread
	if hasBid && hasAsk {
		spread := bestAsk - bestBid
		midPrice := (bestBid + bestAsk) / 2.0
		return midPrice, spread
	} else if hasBid {
		return bestBid, 0
	} else if hasAsk {
		return bestAsk, 0
	}

	return 0, 0
}

// ExtractVolume extracts volume from order book data.
// IMPORTANT: For OHLCV, volume should represent actual trading volume, not order book depth.
// Order book updates don't represent trades, so we use minimal volume (or 0) for price tracking.
// Actual trade volume comes from last_trade_price events which have the trade size.
// This function checks if the message represents a trade (same price and size in bid/ask) 
// or just an order book update.
func ExtractVolume(bids []interface{}, asks []interface{}) float64 {
	// Check if this looks like a trade event (same price and size in both bid and ask)
	// Trade events from last_trade_price are converted to book messages with identical bid/ask
	if len(bids) == 1 && len(asks) == 1 {
		bidMap, bidOk := bids[0].(map[string]interface{})
		askMap, askOk := asks[0].(map[string]interface{})
		
		if bidOk && askOk {
			bidPrice, bidPriceOk := bidMap["price"].(string)
			bidSize, bidSizeOk := bidMap["size"].(string)
			askPrice, askPriceOk := askMap["price"].(string)
			askSize, askSizeOk := askMap["size"].(string)
			
			// If bid and ask have same price and size, this is likely a trade event
			if bidPriceOk && bidSizeOk && askPriceOk && askSizeOk &&
				bidPrice == askPrice && bidSize == askSize {
				// This is a trade event - use the trade size as volume
				if size, err := parseFloat(bidSize); err == nil {
					return size
				}
			}
		}
	}
	
	// For regular order book updates, return 0 volume
	// We only track price changes, not order book depth as volume
	// Actual volume should come from trade events
	return 0
}

// ExtractVolumeWithLogging is a version of ExtractVolume with detailed logging for debugging
// This should only be used temporarily to diagnose volume extraction issues
func ExtractVolumeWithLogging(bids []interface{}, asks []interface{}, logger *slog.Logger, messageCount int) float64 {
	volume := ExtractVolume(bids, asks)
	
	// Log detailed information for first few messages or when volume is detected
	if messageCount <= 10 || volume > 0 {
		logger.Info("🔍 ExtractVolume debugging",
			"bids_count", len(bids),
			"asks_count", len(asks),
			"extracted_volume", volume,
			"message_count", messageCount)
		
		if len(bids) == 1 && len(asks) == 1 {
			bidMap, bidOk := bids[0].(map[string]interface{})
			askMap, askOk := asks[0].(map[string]interface{})
			
			if bidOk && askOk {
				bidPrice := bidMap["price"]
				bidSize := bidMap["size"]
				askPrice := askMap["price"]
				askSize := askMap["size"]
				
				logger.Info("🔍 ExtractVolume: single bid/ask details",
					"bid_price", bidPrice,
					"bid_size", bidSize,
					"ask_price", askPrice,
					"ask_size", askSize,
					"prices_match", bidPrice == askPrice,
					"sizes_match", bidSize == askSize,
					"message_count", messageCount)
			}
		}
	}
	
	return volume
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

	a.logger.Info("📊 OHLCV aggregator status check",
		"total_updates", a.totalUpdates,
		"total_bars_saved", a.totalBarsSaved,
		"active_markets", len(a.bars))

	if len(a.bars) == 0 {
		a.logger.Warn("⚠️  OHLCV aggregator: no bars in memory",
			"total_updates", a.totalUpdates,
			"total_bars_saved", a.totalBarsSaved)
		return
	}

	// Count bars by resolution
	barCounts := make(map[string]int)
	totalBars := 0

	for _, resolutions := range a.bars {
		for resolution := range resolutions {
			barCounts[resolution]++
			totalBars++
		}
	}

	a.logger.Info("📊 OHLCV aggregator status",
		"updates", a.totalUpdates,
		"bars_saved", a.totalBarsSaved,
		"markets", len(a.bars),
		"active_bars", totalBars,
		"by_resolution", barCounts)
}

// periodicFlush periodically checks for completed bars and saves them to the database.
// This ensures bars are saved even if no new price updates arrive after a time period ends.
func (a *OHLCVAggregator) periodicFlush() {
	ticker := time.NewTicker(15 * time.Second) // Check every 15 seconds
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.flushCompletedBars()
		}
	}
}

// flushCompletedBars checks all bars in memory and saves any that have completed their time period.
func (a *OHLCVAggregator) flushCompletedBars() {
	now := time.Now()
	var barsToSave []*CurrentBar
	var barsToRemove []struct {
		marketID  string
		resolution string
	}

	a.mu.Lock()
	// First pass: identify completed bars
	totalBarsChecked := 0
	for marketID, resolutions := range a.bars {
		for resolution, bar := range resolutions {
			totalBarsChecked++
			// Calculate when this bar's time period ends
			barEndTime := a.getBarEndTime(bar.StartTime, bar.Resolution)
			
			// Use a small tolerance (1 second) to account for timing differences
			// If the current time is past the bar's end time (with tolerance), it's completed
			if now.After(barEndTime.Add(-time.Second)) {
				barsToSave = append(barsToSave, bar)
				barsToRemove = append(barsToRemove, struct {
					marketID   string
					resolution string
				}{marketID: marketID, resolution: resolution})
			}
		}
	}
	a.mu.Unlock()

	// Log periodic flush activity only when bars are saved
	if len(barsToSave) > 0 {
		a.logger.Info("🔄 periodic flush",
			"bars_saved", len(barsToSave),
			"total_active", totalBarsChecked)
	}

	// Second pass: save completed bars (outside the lock to avoid holding it during DB operations)
	if len(barsToSave) > 0 {
		a.logger.Info("💾 flushing completed bars", "count", len(barsToSave))
		saveErrors := 0
		for _, bar := range barsToSave {
			if err := a.saveBar(bar); err != nil {
				saveErrors++
				a.logger.Error("failed to flush bar",
					"error", err,
					"market_id", bar.MarketID,
					"resolution", bar.Resolution)
			}
		}
		if saveErrors == 0 {
			a.logger.Info("✅ flush completed successfully", "bars_flushed", len(barsToSave))
		} else {
			a.logger.Warn("⚠️  flush completed with errors",
				"bars_flushed", len(barsToSave)-saveErrors,
				"errors", saveErrors)
		}

		// Third pass: remove saved bars from memory
		a.mu.Lock()
		for _, toRemove := range barsToRemove {
			if resolutions, ok := a.bars[toRemove.marketID]; ok {
				delete(resolutions, toRemove.resolution)
				// If no more bars for this market, remove the market entry
				if len(resolutions) == 0 {
					delete(a.bars, toRemove.marketID)
				}
			}
		}
		a.mu.Unlock()
	}
}

// getBarEndTime calculates when a bar's time period ends based on its start time and resolution.
func (a *OHLCVAggregator) getBarEndTime(startTime time.Time, resolution string) time.Time {
	switch resolution {
	case "15": // 15 minutes
		return startTime.Add(15 * time.Minute)
	case "D": // 1 day
		return startTime.Add(24 * time.Hour)
	default:
		return startTime.Add(15 * time.Minute)
	}
}

// cleanupOldData removes OHLCV data older than 90 days to prevent database bloat.
// This prevents accumulation of test data and old historical data while keeping useful history.
func (a *OHLCVAggregator) cleanupOldData() error {
	// Calculate cutoff time: 90 days ago (keeps 3 months of historical data for charts)
	cutoffTime := time.Now().UTC().Add(-90 * 24 * time.Hour)

	// Convert to pgtype.Timestamptz
	var cutoff pgtype.Timestamptz
	if err := cutoff.Scan(cutoffTime); err != nil {
		return fmt.Errorf("failed to convert cutoff time: %w", err)
	}

	// Delete old data
	rowsAffected, err := a.store.DeleteOldMarketPriceHistory(a.ctx, cutoff)
	if err != nil {
		return fmt.Errorf("failed to delete old market price history: %w", err)
	}

	// Log the cleanup
	if rowsAffected > 0 {
		a.logger.Info("🧹 cleaned up old OHLCV data",
			"cutoff_date", cutoffTime.Format("2006-01-02"),
			"records_deleted", rowsAffected)
	}

	// Update last cleanup time
	a.lastCleanupTime = time.Now()

	return nil
}

// ManualCleanup allows manual triggering of old data cleanup.
// This can be called via admin endpoints or maintenance scripts.
func (a *OHLCVAggregator) ManualCleanup() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.logger.Info("🧹 manual cleanup triggered")
	return a.cleanupOldData()
}

