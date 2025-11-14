/**
 * @description
 * This file defines the complete database schema for Poly-Pro Analytics.
 * It serves as a central reference for all tables, indexes, and relationships.
 * The schema is designed to use PostgreSQL 17's native partitioning for time-series data.
 *
 * ⚠️  IMPORTANT: PARTITIONED TABLE USAGE ⚠️
 * 
 * The following tables are PARTITIONED and require special handling for inserts:
 * - market_price_history
 * - market_sentiment_history
 * 
 * ❌ DO NOT use direct INSERT statements on these tables!
 *    Example: INSERT INTO market_price_history (...) VALUES (...)
 *    This will FAIL if the partition doesn't exist.
 * 
 * ✅ USE the wrapper functions instead (they auto-create partitions):
 *    - insert_market_price_history(...)
 *    - insert_market_sentiment_history(...)
 * 
 * See the wrapper function definitions below for usage examples.
 *
 * Tables:
 * - users: Stores user profile information, linking back to their Clerk authentication ID.
 * - wallets: Manages user's Polymarket funder addresses and references to their secure signing keys.
 * - trades: A log of all trades executed by users through the platform.
 * - market_price_history: A native PostgreSQL partitioned table for storing OHLCV (Open, High, Low, Close, Volume) data.
 * - market_sentiment_history: A native PostgreSQL partitioned table for storing aggregated sentiment scores and key drivers.
 * - news_events: Stores information about news articles and events related to specific markets, used to power the AI Insight Hub.
 */

-- Enable necessary extensions if they don't exist
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Table: users
-- Stores core user information, linking our internal user ID to the Clerk authentication provider ID.
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    clerk_user_id VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Table: wallets
-- Associates a user with their Polymarket funder address and a secure reference
-- to their private key for the remote signing service.
CREATE TABLE wallets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    polymarket_funder_address VARCHAR(42) UNIQUE NOT NULL,
    -- This stores the ARN or ID of the secret in the vault, NOT the key itself.
    signer_secret_ref VARCHAR(255) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_wallets_user_id ON wallets(user_id);

-- Table: orders
-- Records all placed orders (pending, filled, cancelled) for tracking and order management.
CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    market_id VARCHAR(255) NOT NULL,
    token_id VARCHAR(255) NOT NULL,
    polymarket_order_id VARCHAR(255) UNIQUE, -- From Polymarket API response after submission
    side VARCHAR(4) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    size DECIMAL NOT NULL,
    price DECIMAL NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'open', 'filled', 'cancelled', 'rejected')),
    signed_order JSONB, -- Store the full signed order JSON for reference
    submitted_at TIMESTAMPTZ, -- When order was submitted to Polymarket
    filled_at TIMESTAMPTZ, -- When order was filled (if applicable)
    cancelled_at TIMESTAMPTZ, -- When order was cancelled (if applicable)
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_market_id ON orders(market_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_created_at ON orders(created_at DESC);

-- Table: trades
-- Records every trade executed by a user, providing a complete history for portfolio tracking.
CREATE TABLE trades (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    order_id UUID REFERENCES orders(id) ON DELETE SET NULL, -- Link to the order that resulted in this trade
    market_id VARCHAR(255) NOT NULL,
    polymarket_trade_id VARCHAR(255) UNIQUE, -- From Polymarket API response
    side VARCHAR(4) NOT NULL CHECK (side IN ('BUY', 'SELL')),
    size DECIMAL NOT NULL,
    price DECIMAL NOT NULL,
    executed_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_trades_user_market ON trades(user_id, market_id);
CREATE INDEX idx_trades_executed_at ON trades(executed_at DESC);
CREATE INDEX idx_trades_order_id ON trades(order_id);

-- Table: market_price_history (Native PostgreSQL Partitioned Table)
-- Stores time-series price data for market charts. Optimized for fast time-based queries.
-- Partitioned by month using RANGE partitioning on the 'time' column.
--
-- ⚠️  WARNING: This is a PARTITIONED table!
--    DO NOT use direct INSERT statements. Use insert_market_price_history() function instead.
--    Direct INSERT will fail with: "no partition of relation found for row"
CREATE TABLE market_price_history (
    time TIMESTAMPTZ NOT NULL,
    market_id VARCHAR(255) NOT NULL,
    open DECIMAL NOT NULL,
    high DECIMAL NOT NULL,
    low DECIMAL NOT NULL,
    close DECIMAL NOT NULL,
    volume DECIMAL NOT NULL,
    resolution VARCHAR(10) NOT NULL DEFAULT '15',
    PRIMARY KEY (market_id, time, resolution)
) PARTITION BY RANGE (time);

-- Create initial partitions for current month (November 2025) and next month (December 2025)
-- Using explicit UTC timestamps to avoid timezone issues
CREATE TABLE market_price_history_y2025m11 PARTITION OF market_price_history
    FOR VALUES FROM ('2025-11-01 00:00:00+00') TO ('2025-12-01 00:00:00+00');
CREATE TABLE market_price_history_y2025m12 PARTITION OF market_price_history
    FOR VALUES FROM ('2025-12-01 00:00:00+00') TO ('2026-01-01 00:00:00+00');

-- Index on partitioned table (applies to all partitions)
CREATE INDEX idx_market_price_history_market_time_resolution ON market_price_history(market_id, time DESC, resolution);

-- Table: market_sentiment_history (Native PostgreSQL Partitioned Table)
-- Stores aggregated sentiment analysis data over time for each market.
-- Partitioned by month using RANGE partitioning on the 'time' column.
--
-- ⚠️  WARNING: This is a PARTITIONED table!
--    DO NOT use direct INSERT statements. Use insert_market_sentiment_history() function instead.
--    Direct INSERT will fail with: "no partition of relation found for row"
CREATE TABLE market_sentiment_history (
    time TIMESTAMPTZ NOT NULL,
    market_id VARCHAR(255) NOT NULL,
    sentiment_score FLOAT NOT NULL, -- e.g., -1.0 (negative) to 1.0 (positive)
    key_drivers TEXT[], -- Array of keywords or phrases driving the sentiment
    PRIMARY KEY (market_id, time)
) PARTITION BY RANGE (time);

-- Create initial partitions for current month (November 2025) and next month (December 2025)
CREATE TABLE market_sentiment_history_y2025m11 PARTITION OF market_sentiment_history
    FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');
CREATE TABLE market_sentiment_history_y2025m12 PARTITION OF market_sentiment_history
    FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');

-- Table: news_events
-- Stores metadata for external news articles that are relevant to a market.
-- This data is used to plot events on charts and provide context in the AI Insight Hub.
CREATE TABLE news_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    market_id VARCHAR(255) NOT NULL,
    url VARCHAR(512) UNIQUE NOT NULL,
    title TEXT NOT NULL,
    source VARCHAR(255) NOT NULL,
    published_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_news_events_market_published ON news_events(market_id, published_at DESC);

-- ============================================================================
-- Automatic Partition Creation Functions
-- ============================================================================
-- These functions ensure partitions exist before inserting data.
-- PostgreSQL requires partitions to exist before INSERT, so these helper
-- functions should be called before inserts or used in wrapper functions.

/**
 * @description
 * Helper function to ensure a partition exists for market_price_history
 * for the given timestamp's month. Creates the partition if it doesn't exist.
 * 
 * Usage: Call this function before inserting data, or use the wrapper
 * insert function below.
 * 
 * @param timestamp_val The timestamp to check/create partition for
 * @returns The partition name that was created or already exists
 */
CREATE OR REPLACE FUNCTION ensure_market_price_history_partition(timestamp_val TIMESTAMPTZ)
RETURNS TEXT AS $$
DECLARE
    partition_start TIMESTAMPTZ;
    partition_end TIMESTAMPTZ;
    partition_name TEXT;
BEGIN
    -- Calculate the start and end of the month for the given timestamp
    partition_start := date_trunc('month', timestamp_val);
    partition_end := partition_start + INTERVAL '1 month';
    partition_name := 'market_price_history_y' || to_char(partition_start, 'YYYY') || 'm' || LPAD(to_char(partition_start, 'MM'), 2, '0');
    
    -- Check if partition already exists
    IF NOT EXISTS (
        SELECT 1 FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relname = partition_name
        AND n.nspname = current_schema()
    ) THEN
        -- Create the partition
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF market_price_history FOR VALUES FROM (%L) TO (%L)',
            partition_name,
            partition_start,
            partition_end
        );
        RAISE NOTICE 'Created partition: %', partition_name;
    END IF;
    
    RETURN partition_name;
END;
$$ LANGUAGE plpgsql;

/**
 * @description
 * Helper function to ensure a partition exists for market_sentiment_history
 * for the given timestamp's month. Creates the partition if it doesn't exist.
 * 
 * Usage: Call this function before inserting data, or use the wrapper
 * insert function below.
 * 
 * @param timestamp_val The timestamp to check/create partition for
 * @returns The partition name that was created or already exists
 */
CREATE OR REPLACE FUNCTION ensure_market_sentiment_history_partition(timestamp_val TIMESTAMPTZ)
RETURNS TEXT AS $$
DECLARE
    partition_start TIMESTAMPTZ;
    partition_end TIMESTAMPTZ;
    partition_name TEXT;
BEGIN
    -- Calculate the start and end of the month for the given timestamp
    partition_start := date_trunc('month', timestamp_val);
    partition_end := partition_start + INTERVAL '1 month';
    partition_name := 'market_sentiment_history_y' || to_char(partition_start, 'YYYY') || 'm' || LPAD(to_char(partition_start, 'MM'), 2, '0');
    
    -- Check if partition already exists
    IF NOT EXISTS (
        SELECT 1 FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relname = partition_name
        AND n.nspname = current_schema()
    ) THEN
        -- Create the partition
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF market_sentiment_history FOR VALUES FROM (%L) TO (%L)',
            partition_name,
            partition_start,
            partition_end
        );
        RAISE NOTICE 'Created partition: %', partition_name;
    END IF;
    
    RETURN partition_name;
END;
$$ LANGUAGE plpgsql;

/**
 * @description
 * Wrapper function to insert into market_price_history with automatic
 * partition creation. This function ensures the partition exists before inserting.
 * 
 * Usage: Use this function instead of direct INSERT INTO market_price_history
 * 
 * Example:
 *   SELECT insert_market_price_history(NOW(), 'market-123', 1.0, 1.1, 0.9, 1.0, 100.0);
 */
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
    
    -- Insert the data with conflict resolution
    -- If a bar already exists for this market_id, time, and resolution, update it
    INSERT INTO market_price_history (time, market_id, open, high, low, close, volume, resolution)
    VALUES (p_time, p_market_id, p_open, p_high, p_low, p_close, p_volume, p_resolution)
    ON CONFLICT (market_id, time, resolution) DO UPDATE SET
        open = EXCLUDED.open,
        high = EXCLUDED.high,
        low = EXCLUDED.low,
        close = EXCLUDED.close,
        volume = EXCLUDED.volume;
END;
$$ LANGUAGE plpgsql;

/**
 * @description
 * Wrapper function to insert into market_sentiment_history with automatic
 * partition creation. This function ensures the partition exists before inserting.
 * 
 * Usage: Use this function instead of direct INSERT INTO market_sentiment_history
 * 
 * Example:
 *   SELECT insert_market_sentiment_history(NOW(), 'market-123', 0.75, ARRAY['positive', 'trending']);
 */
CREATE OR REPLACE FUNCTION insert_market_sentiment_history(
    p_time TIMESTAMPTZ,
    p_market_id VARCHAR(255),
    p_sentiment_score FLOAT,
    p_key_drivers TEXT[]
)
RETURNS VOID AS $$
BEGIN
    -- Ensure partition exists
    PERFORM ensure_market_sentiment_history_partition(p_time);
    
    -- Insert the data
    INSERT INTO market_sentiment_history (time, market_id, sentiment_score, key_drivers)
    VALUES (p_time, p_market_id, p_sentiment_score, p_key_drivers)
    ON CONFLICT (market_id, time) DO UPDATE SET
        sentiment_score = EXCLUDED.sentiment_score,
        key_drivers = EXCLUDED.key_drivers;
END;
$$ LANGUAGE plpgsql;

/**
 * @description
 * Function to create partitions for the next N months proactively.
 * This is useful to run via pg_cron or as a scheduled job.
 * 
 * Usage: Call this periodically (e.g., monthly) to create partitions in advance.
 * 
 * Example: SELECT create_future_partitions(3); -- Creates next 3 months
 */
CREATE OR REPLACE FUNCTION create_future_partitions(months_ahead INT DEFAULT 3)
RETURNS TEXT[] AS $$
DECLARE
    created_partitions TEXT[] := ARRAY[]::TEXT[];
    current_month TIMESTAMPTZ;
    i INT;
BEGIN
    current_month := date_trunc('month', NOW());
    
    FOR i IN 0..months_ahead LOOP
        PERFORM ensure_market_price_history_partition(current_month + (i || ' months')::INTERVAL);
        PERFORM ensure_market_sentiment_history_partition(current_month + (i || ' months')::INTERVAL);
        created_partitions := array_append(created_partitions, 
            to_char(current_month + make_interval(months => i), 'YYYY-MM'));
    END LOOP;
    
    RETURN created_partitions;
END;
$$ LANGUAGE plpgsql;
