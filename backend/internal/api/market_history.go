/**
 * @description
 * This file contains the HTTP handler for fetching historical market data,
 * which is required by the TradingView charting library.
 *
 * Key features:
 * - Historical Data Endpoint: Exposes an endpoint (`GET /api/v1/markets/:id/history`)
 *   to serve OHLCV (Open, High, Low, Close, Volume) data.
 * - Database Query: Queries the `market_price_history` partitioned table to retrieve
 *   actual historical OHLCV data filtered by market ID, time range, and resolution.
 * - TradingView Compatibility: The response format is structured specifically for
 *   TradingView's UDF (Unified Data Format) adapter, with fields for time, open, high,
 *   low, close, and volume.
 */

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/poly-pro/backend/internal/db"
)

// TradingViewBar represents a single OHLCV bar for the TradingView chart.
// The field names (t, o, h, l, c, v) are specific to the UDF format.
type TradingViewBar struct {
	Time   int64   `json:"t"` // Bar time, Unix timestamp (seconds)
	Open   float64 `json:"o"` // Open price
	High   float64 `json:"h"` // High price
	Low    float64 `json:"l"` // Low price
	Close  float64 `json:"c"` // Close price
	Volume float64 `json:"v"` // Volume
}

/**
 * @function getMarketHistory
 * @description A Gin handler that returns historical OHLCV data for a given market.
 * It uses query parameters `from`, `to`, and `resolution` to determine the data range.
 *
 * @param c *gin.Context The Gin context for the request.
 *
 * @notes
 * - This handler queries the `market_price_history` partitioned table for real historical data.
 * - The response structure is tailored for the TradingView charting library's UDF adapter.
 * - Data is filtered by market ID, time range, and resolution for efficient querying.
 */
func (server *Server) getMarketHistory(c *gin.Context) {
	marketID := c.Param("id")
	fromStr := c.Query("from")
	toStr := c.Query("to")
	resolution := c.Query("resolution") // e.g., "1", "5", "15", "60", "D"

	server.logger.Info("received market history request",
		"market_id", marketID,
		"from", fromStr,
		"to", toStr,
		"resolution", resolution,
	)

	// Validate marketID
	if marketID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"s":    "error",
			"errmsg": "market ID is required",
		})
		return
	}

	// Parse and validate timestamps
	from, err := strconv.ParseInt(fromStr, 10, 64)
	if err != nil || from <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"s":    "error",
			"errmsg": "invalid 'from' timestamp",
		})
		return
	}

	to, err := strconv.ParseInt(toStr, 10, 64)
	if err != nil || to <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"s":    "error",
			"errmsg": "invalid 'to' timestamp",
		})
		return
	}

	if from >= to {
		c.JSON(http.StatusBadRequest, gin.H{
			"s":    "error",
			"errmsg": "'from' timestamp must be less than 'to' timestamp",
		})
		return
	}

	// Convert Unix timestamps to time.Time
	fromTime := time.Unix(from, 0)
	toTime := time.Unix(to, 0)

	// Query the database for historical data
	var fromTimeVal pgtype.Timestamptz
	if err := fromTimeVal.Scan(fromTime); err != nil {
		server.logger.Error("failed to convert from time", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"s":    "error",
			"errmsg": "failed to process time range",
		})
		return
	}

	var toTimeVal pgtype.Timestamptz
	if err := toTimeVal.Scan(toTime); err != nil {
		server.logger.Error("failed to convert to time", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"s":    "error",
			"errmsg": "failed to process time range",
		})
		return
	}

	// Query database with resolution filter
	arg := db.GetMarketPriceHistoryParams{
		MarketID:   marketID,
		Time:       fromTimeVal,
		Time_2:     toTimeVal,
		Resolution: resolution,
	}

	dbBars, err := server.store.GetMarketPriceHistory(c.Request.Context(), arg)
	if err != nil {
		server.logger.Error("failed to query market price history", "error", err, "market_id", marketID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"s":    "error",
			"errmsg": "failed to fetch historical data",
		})
		return
	}

	// Convert database results to TradingViewBar format
	var bars []TradingViewBar
	for _, dbBar := range dbBars {
		// Convert database bar to TradingViewBar
		// Check if time is valid
		if !dbBar.Time.Valid {
			server.logger.Warn("bar time is invalid, skipping")
			continue
		}
		barTime := dbBar.Time.Time

		// Convert pgtype.Numeric to float64
		convertNumeric := func(n pgtype.Numeric) (float64, error) {
			if !n.Valid {
				return 0, nil
			}
			// Use Float64Value() which returns a Float8, then get its value
			float8Val, err := n.Float64Value()
			if err != nil {
				return 0, err
			}
			if !float8Val.Valid {
				return 0, nil
			}
			return float8Val.Float64, nil
		}

		open, err := convertNumeric(dbBar.Open)
		if err != nil {
			server.logger.Warn("failed to convert open price", "error", err)
			continue
		}

		high, err := convertNumeric(dbBar.High)
		if err != nil {
			server.logger.Warn("failed to convert high price", "error", err)
			continue
		}

		low, err := convertNumeric(dbBar.Low)
		if err != nil {
			server.logger.Warn("failed to convert low price", "error", err)
			continue
		}

		close, err := convertNumeric(dbBar.Close)
		if err != nil {
			server.logger.Warn("failed to convert close price", "error", err)
			continue
		}

		volume, err := convertNumeric(dbBar.Volume)
		if err != nil {
			server.logger.Warn("failed to convert volume", "error", err)
			continue
		}

		bars = append(bars, TradingViewBar{
			Time:   barTime.Unix(),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: volume,
		})
	}

	// TradingView UDF adapter expects `s: "ok"` for success and `s: "no_data"` if no bars are found.
	if len(bars) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"s": "no_data",
		})
		return
	}

	// Build response arrays safely without type assertions
	times := make([]int64, len(bars))
	opens := make([]float64, len(bars))
	highs := make([]float64, len(bars))
	lows := make([]float64, len(bars))
	closes := make([]float64, len(bars))
	volumes := make([]float64, len(bars))

	for i, b := range bars {
		times[i] = b.Time
		opens[i] = b.Open
		highs[i] = b.High
		lows[i] = b.Low
		closes[i] = b.Close
		volumes[i] = b.Volume
	}

	// The UDF format expects separate arrays for each field.
	response := gin.H{
		"s": "ok",
		"t": times,
		"o": opens,
		"h": highs,
		"l": lows,
		"c": closes,
		"v": volumes,
	}

	c.JSON(http.StatusOK, response)
}

