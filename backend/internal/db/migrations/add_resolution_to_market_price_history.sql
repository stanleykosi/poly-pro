/**
 * @description
 * Migration to add resolution column to market_price_history table.
 * This allows us to store and query OHLCV bars at different resolutions (1m, 5m, 15m, 1h, 1d, etc.).
 * 
 * Run this in Supabase SQL Editor:
 * 1. Copy the contents of this file
 * 2. Paste into Supabase SQL Editor
 * 3. Execute
 */

-- Add resolution column to market_price_history table
ALTER TABLE market_price_history
ADD COLUMN resolution VARCHAR(10) NOT NULL DEFAULT '15';

-- Update the index to include resolution for better query performance
DROP INDEX IF EXISTS idx_market_price_history_market_time;
CREATE INDEX idx_market_price_history_market_time_resolution ON market_price_history(market_id, time DESC, resolution);

-- Update the insert function to accept resolution parameter
CREATE OR REPLACE FUNCTION insert_market_price_history(
    p_time TIMESTAMPTZ,
    p_market_id VARCHAR(255),
    p_open DECIMAL,
    p_high DECIMAL,
    p_low DECIMAL,
    p_close DECIMAL,
    p_volume DECIMAL,
    p_resolution VARCHAR(10) DEFAULT '15'
)
RETURNS VOID AS $$
BEGIN
    -- Ensure partition exists
    PERFORM ensure_market_price_history_partition(p_time);
    
    -- Insert the data
    INSERT INTO market_price_history (time, market_id, open, high, low, close, volume, resolution)
    VALUES (p_time, p_market_id, p_open, p_high, p_low, p_close, p_volume, p_resolution);
END;
$$ LANGUAGE plpgsql;

