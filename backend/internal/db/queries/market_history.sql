/**
 * @description
 * This file contains all the SQL queries for interacting with the 'market_price_history' table.
 * These queries are used by sqlc to generate type-safe Go code for our database access layer.
 */

-- name: GetMarketPriceHistory :many
-- @description Retrieves historical OHLCV data for a given market within a time range and resolution.
-- The data is ordered by time ascending and filtered by resolution.
-- @param market_id The market ID to fetch data for.
-- @param from_time The start time (inclusive) for the query.
-- @param to_time The end time (inclusive) for the query.
-- @param resolution The resolution/interval (e.g., '1', '5', '15', '60', 'D').
SELECT 
  time,
  market_id,
  open,
  high,
  low,
  close,
  volume,
  resolution
FROM market_price_history
WHERE market_id = $1
  AND time >= $2
  AND time <= $3
  AND resolution = $4
ORDER BY time ASC;

-- name: InsertMarketPriceHistory :exec
-- @description Inserts a new OHLCV bar into the market_price_history table.
-- This uses the insert_market_price_history() function which automatically creates partitions.
-- @param time The timestamp for this bar.
-- @param market_id The market ID.
-- @param open The opening price.
-- @param high The highest price in the period.
-- @param low The lowest price in the period.
-- @param close The closing price.
-- @param volume The trading volume.
-- @param resolution The resolution/interval (e.g., '1', '5', '15', '60', 'D').
SELECT insert_market_price_history($1, $2, $3, $4, $5, $6, $7, $8);

