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
func (a *OHLCVAggregator) UpdatePrice(marketID string, price float64, timestamp time.Time) error {
	a.totalUpdates++
	
	// Log first few updates to confirm function is being called
	if a.totalUpdates <= 3 {
		a.logger.Info("OHLCV aggregator: processing price update", 
			"update", a.totalUpdates,
			"market_id", marketID,
			"price", price)
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

	// Log timestamp details for first few updates to debug date issues
	now := time.Now().UTC()
	if a.totalUpdates <= 10 {
		a.logger.Info("🔍 timestamp flow in aggregator",
			"update", a.totalUpdates,
			"market_id", marketID,
			"resolution", resolution,
			"input_timestamp", timestamp.Format(time.RFC3339),
			"input_timestamp_date", timestamp.Format("2006-01-02"),
			"input_timestamp_unix", timestamp.Unix(),
			"bar_start_time", barStartTime.Format(time.RFC3339),
			"bar_start_time_date", barStartTime.Format("2006-01-02"),
			"bar_start_time_unix", barStartTime.Unix(),
			"current_time_utc", now.Format(time.RFC3339),
			"current_time_date", now.Format("2006-01-02"),
			"diff_from_now", now.Sub(timestamp),
			"bar_date_matches_current", barStartTime.Format("2006-01-02") == now.Format("2006-01-02"))
	}
	
	// Additional validation: if the bar start time is more than 1 day old, log a warning
	// This helps catch cases where stale timestamps are creating bars with old dates
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
	if !exists || bar.StartTime.Before(barStartTime) {
		// If the bar doesn't exist or we've moved to a new time period, save the old bar and create a new one
		if exists {
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
		barEndTime := a.getBarEndTime(barStartTime, resolution)
		a.logger.Info("🆕 created new OHLCV bar", 
			"market_id", marketID, 
			"resolution", resolution,
			"start_time", barStartTime,
			"start_time_rfc3339", barStartTime.Format(time.RFC3339),
			"start_time_date", barStartTime.Format("2006-01-02"),
			"end_time", barEndTime,
			"initial_price", price)
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

	// No need to log every update - too verbose

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
		// Ensure we use UTC for daily bars to avoid timezone issues
		utcTimestamp := timestamp.UTC()
		return time.Date(utcTimestamp.Year(), utcTimestamp.Month(), utcTimestamp.Day(), 0, 0, 0, 0, time.UTC)
	default:
		return timestamp.Truncate(time.Hour)
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
	
	// Log the exact pgtype.Timestamptz value being sent to the database
	// This helps debug timezone issues
	a.logger.Info("🔍 timestamp conversion details",
		"original_time", bar.StartTime.Format(time.RFC3339),
		"utc_time", utcTime.Format(time.RFC3339),
		"utc_time_unix", utcTime.Unix(),
		"pgtype_valid", timeVal.Valid,
		"pgtype_time", timeVal.Time.Format(time.RFC3339),
		"pgtype_time_unix", timeVal.Time.Unix(),
		"pgtype_infinity", timeVal.InfinityModifier,
		"pgtype_time_utc", timeVal.Time.UTC().Format(time.RFC3339))

	// Helper function to convert float64 to pgtype.Numeric
	// pgtype.Numeric.Scan() doesn't accept float64 directly, so we convert to string first
	convertToNumeric := func(val float64) (pgtype.Numeric, error) {
		var num pgtype.Numeric
		// Convert float64 to string with sufficient precision (10 decimal places)
		// Use 'g' format to avoid trailing zeros and handle large/small numbers
		valStr := strconv.FormatFloat(val, 'g', -1, 64)
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

	// Log the insert attempt with full details, including UTC timestamp
	a.logger.Debug("attempting to insert OHLCV bar",
		"market_id", bar.MarketID,
		"resolution", bar.Resolution,
		"start_time_original", bar.StartTime,
		"start_time_utc", utcTime,
		"start_time_rfc3339", utcTime.Format(time.RFC3339),
		"start_time_unix", utcTime.Unix(),
		"time_valid", timeVal.Valid,
		"open", bar.Open,
		"high", bar.High,
		"low", bar.Low,
		"close", bar.Close)

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

	// Verify the insert by querying the database
	// This helps catch cases where the insert appears to succeed but data isn't actually saved
	verifyCtx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
	defer cancel()
	
	// Use a small time range around the bar's start time for verification
	// The SQL query uses time >= $2 AND time <= $3, so we need a range
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
		a.logger.Warn("⚠️  insert succeeded but verification query failed",
			"error", verifyErr,
			"market_id", bar.MarketID,
			"resolution", bar.Resolution,
			"start_time_utc", utcTime,
			"start_time_rfc3339", utcTime.Format(time.RFC3339))
	} else if len(verifyResults) == 0 {
		a.logger.Error("❌ insert appeared to succeed but data not found in database",
			"market_id", bar.MarketID,
			"resolution", bar.Resolution,
			"start_time_utc", utcTime,
			"start_time_rfc3339", utcTime.Format(time.RFC3339),
			"this_indicates_a_database_issue")
	} else {
		// Successfully verified - log what was actually stored in the database
		storedTime := verifyResults[0].Time
		if storedTime.Valid {
			storedTimeUTC := storedTime.Time.UTC()
			storedDate := storedTimeUTC.Format("2006-01-02")
			sentDate := utcTime.Format("2006-01-02")
			datesMatch := storedDate == sentDate
			
			a.logger.Info("✅ verified insert - comparing stored vs sent",
				"market_id", bar.MarketID,
				"resolution", bar.Resolution,
				"sent_time_utc", utcTime.Format(time.RFC3339),
				"sent_time_date", sentDate,
				"stored_time_utc", storedTimeUTC.Format(time.RFC3339),
				"stored_time_date", storedDate,
				"stored_time_unix", storedTimeUTC.Unix(),
				"dates_match", datesMatch,
				"time_diff", storedTimeUTC.Sub(utcTime))
			
			if !datesMatch {
				a.logger.Error("❌ DATE MISMATCH: stored date differs from sent date",
					"sent_date", sentDate,
					"stored_date", storedDate,
					"sent_time", utcTime.Format(time.RFC3339),
					"stored_time", storedTimeUTC.Format(time.RFC3339),
					"market_id", bar.MarketID,
					"resolution", bar.Resolution)
			}
		} else {
			a.logger.Warn("⚠️  stored timestamp is invalid",
				"market_id", bar.MarketID,
				"resolution", bar.Resolution)
		}
	}

	a.totalBarsSaved++
	
	// Log the date being saved to help debug timestamp issues
	currentDate := time.Now().UTC().Format("2006-01-02")
	savedDate := utcTime.Format("2006-01-02")
	dateMatches := savedDate == currentDate
	
	a.logger.Info("✅ OHLCV bar saved to database", 
		"market_id", bar.MarketID, 
		"resolution", bar.Resolution,
		"start_time_utc", utcTime,
		"start_time_rfc3339", utcTime.Format(time.RFC3339),
		"start_time_date", savedDate,
		"start_time_unix", utcTime.Unix(),
		"current_date", currentDate,
		"date_matches_current", dateMatches,
		"open", bar.Open,
		"high", bar.High,
		"low", bar.Low,
		"close", bar.Close,
		"updates", bar.Count,
		"total_saved", a.totalBarsSaved,
		"verified", len(verifyResults) > 0)
	
	// Warn if the date doesn't match current date
	if !dateMatches {
		a.logger.Warn("⚠️  saved bar date does not match current date",
			"saved_date", savedDate,
			"current_date", currentDate,
			"market_id", bar.MarketID,
			"resolution", bar.Resolution,
			"start_time_utc", utcTime.Format(time.RFC3339))
	}
	
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

	// Log periodic flush activity
	if totalBarsChecked > 0 || len(barsToSave) > 0 {
		a.logger.Info("🔄 periodic flush check", 
			"bars_checked", totalBarsChecked,
			"bars_to_save", len(barsToSave),
			"now", now.Format(time.RFC3339))
	}

	// Second pass: save completed bars (outside the lock to avoid holding it during DB operations)
	if len(barsToSave) > 0 {
		a.logger.Info("💾 flushing completed bars", "count", len(barsToSave))
		for _, bar := range barsToSave {
			barEndTime := a.getBarEndTime(bar.StartTime, bar.Resolution)
			if err := a.saveBar(bar); err != nil {
				a.logger.Error("failed to flush completed bar", 
					"error", err, 
					"market_id", bar.MarketID, 
					"resolution", bar.Resolution,
					"start_time", bar.StartTime,
					"end_time", barEndTime,
					"now", now)
			} else {
				a.logger.Info("✅ flushed completed bar",
					"market_id", bar.MarketID,
					"resolution", bar.Resolution,
					"start_time", bar.StartTime,
					"end_time", barEndTime,
					"updates_in_bar", bar.Count)
			}
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
	case "1": // 1 minute
		return startTime.Add(1 * time.Minute)
	case "5": // 5 minutes
		return startTime.Add(5 * time.Minute)
	case "15": // 15 minutes
		return startTime.Add(15 * time.Minute)
	case "60": // 1 hour
		return startTime.Add(1 * time.Hour)
	case "D": // 1 day
		return startTime.Add(24 * time.Hour)
	default:
		return startTime.Add(1 * time.Hour)
	}
}

