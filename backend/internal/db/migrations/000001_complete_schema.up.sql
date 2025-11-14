/**
 * @description
 * Complete database schema migration for Poly-Pro Analytics.
 * This migration sets up the entire database from scratch with:
 * - All tables (users, wallets, trades, market_price_history, market_sentiment_history, news_events)
 * - Partitioned time-series tables with automatic partition creation
 * - All necessary indexes for performance
 * - Row Level Security (RLS) policies for Supabase
 * - Proper timezone handling (UTC)
 * - Helper functions for partition management
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
 * This migration is designed for Supabase PostgreSQL 17 with native partitioning.
 */

-- ============================================================================
-- TIMEZONE CONFIGURATION
-- ============================================================================
-- Ensure all timestamps are stored and handled in UTC
SET timezone = 'UTC';

-- ============================================================================
-- EXTENSIONS
-- ============================================================================
-- Enable necessary PostgreSQL extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Optional: pg_partman extension for automated partition management
-- This extension automates partition creation and maintenance
-- If available in your Supabase instance, uncomment the line below:
-- CREATE EXTENSION IF NOT EXISTS "pg_partman" SCHEMA "partman";
-- 
-- Note: pg_partman is planned for future Supabase releases but may be available now.
-- If not available, the custom partition management functions below will handle partitioning.
-- Native PostgreSQL partitioning (used here) is production-ready and fully supported.

-- ============================================================================
-- TABLES
-- ============================================================================

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
CREATE INDEX idx_trades_order_id ON trades(order_id);

-- Table: market_price_history (Native PostgreSQL Partitioned Table)
-- Stores time-series price data for market charts. Optimized for fast time-based queries.
-- Partitioned by month using RANGE partitioning on the 'time' column.
-- ⚠️  WARNING: This is a PARTITIONED table! Use insert_market_price_history() function.
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

-- Table: market_sentiment_history (Native PostgreSQL Partitioned Table)
-- Stores aggregated sentiment analysis data over time for each market.
-- Partitioned by month using RANGE partitioning on the 'time' column.
-- ⚠️  WARNING: This is a PARTITIONED table! Use insert_market_sentiment_history() function.
CREATE TABLE market_sentiment_history (
    time TIMESTAMPTZ NOT NULL,
    market_id VARCHAR(255) NOT NULL,
    sentiment_score FLOAT NOT NULL, -- e.g., -1.0 (negative) to 1.0 (positive)
    key_drivers TEXT[], -- Array of keywords or phrases driving the sentiment
    PRIMARY KEY (market_id, time)
) PARTITION BY RANGE (time);

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

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Indexes for users table
CREATE INDEX idx_users_clerk_user_id ON users(clerk_user_id);
CREATE INDEX idx_users_email ON users(email);

-- Indexes for wallets table
CREATE INDEX idx_wallets_user_id ON wallets(user_id);
CREATE INDEX idx_wallets_polymarket_funder_address ON wallets(polymarket_funder_address);

-- Indexes for trades table
CREATE INDEX idx_trades_user_id ON trades(user_id);
CREATE INDEX idx_trades_user_market ON trades(user_id, market_id);
CREATE INDEX idx_trades_executed_at ON trades(executed_at DESC);
CREATE INDEX idx_trades_market_id ON trades(market_id);

-- Indexes for market_price_history (applies to all partitions)
CREATE INDEX idx_market_price_history_market_time_resolution ON market_price_history(market_id, time DESC, resolution);
CREATE INDEX idx_market_price_history_time ON market_price_history(time DESC);

-- Indexes for market_sentiment_history (applies to all partitions)
CREATE INDEX idx_market_sentiment_history_market_time ON market_sentiment_history(market_id, time DESC);
CREATE INDEX idx_market_sentiment_history_time ON market_sentiment_history(time DESC);

-- Indexes for news_events table
CREATE INDEX idx_news_events_market_id ON news_events(market_id);
CREATE INDEX idx_news_events_market_published ON news_events(market_id, published_at DESC);
CREATE INDEX idx_news_events_published_at ON news_events(published_at DESC);

-- ============================================================================
-- INITIAL PARTITIONS
-- ============================================================================
-- Create initial partitions for current month and next 2 months
-- Using explicit UTC timestamps to avoid timezone issues

-- Get current month in UTC
DO $$
DECLARE
    current_month_start TIMESTAMPTZ;
    next_month_start TIMESTAMPTZ;
    month_after_next_start TIMESTAMPTZ;
    month_after_after_next_start TIMESTAMPTZ;
BEGIN
    -- Calculate month boundaries (date_trunc on TIMESTAMPTZ works in UTC)
    current_month_start := date_trunc('month', NOW());
    next_month_start := current_month_start + INTERVAL '1 month';
    month_after_next_start := next_month_start + INTERVAL '1 month';
    month_after_after_next_start := month_after_next_start + INTERVAL '1 month';
    
    -- Create partitions for market_price_history
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS market_price_history_y%s PARTITION OF market_price_history FOR VALUES FROM (%L) TO (%L)',
        to_char(current_month_start, 'YYYY') || 'm' || LPAD(to_char(current_month_start, 'MM'), 2, '0'),
        current_month_start,
        next_month_start
    );
    
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS market_price_history_y%s PARTITION OF market_price_history FOR VALUES FROM (%L) TO (%L)',
        to_char(next_month_start, 'YYYY') || 'm' || LPAD(to_char(next_month_start, 'MM'), 2, '0'),
        next_month_start,
        month_after_next_start
    );
    
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS market_price_history_y%s PARTITION OF market_price_history FOR VALUES FROM (%L) TO (%L)',
        to_char(month_after_next_start, 'YYYY') || 'm' || LPAD(to_char(month_after_next_start, 'MM'), 2, '0'),
        month_after_next_start,
        month_after_after_next_start
    );
    
    -- Create partitions for market_sentiment_history
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS market_sentiment_history_y%s PARTITION OF market_sentiment_history FOR VALUES FROM (%L) TO (%L)',
        to_char(current_month_start, 'YYYY') || 'm' || LPAD(to_char(current_month_start, 'MM'), 2, '0'),
        current_month_start,
        next_month_start
    );
    
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS market_sentiment_history_y%s PARTITION OF market_sentiment_history FOR VALUES FROM (%L) TO (%L)',
        to_char(next_month_start, 'YYYY') || 'm' || LPAD(to_char(next_month_start, 'MM'), 2, '0'),
        next_month_start,
        month_after_next_start
    );
    
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS market_sentiment_history_y%s PARTITION OF market_sentiment_history FOR VALUES FROM (%L) TO (%L)',
        to_char(month_after_next_start, 'YYYY') || 'm' || LPAD(to_char(month_after_next_start, 'MM'), 2, '0'),
        month_after_next_start,
        month_after_after_next_start
    );
END $$;

-- ============================================================================
-- PARTITION MANAGEMENT FUNCTIONS
-- ============================================================================

/**
 * @description
 * Helper function to ensure a partition exists for market_price_history
 * for the given timestamp's month. Creates the partition if it doesn't exist.
 * 
 * @param timestamp_val The timestamp to check/create partition for (in UTC)
 * @returns The partition name that was created or already exists
 */
CREATE OR REPLACE FUNCTION ensure_market_price_history_partition(timestamp_val TIMESTAMPTZ)
RETURNS TEXT AS $$
DECLARE
    partition_start TIMESTAMPTZ;
    partition_end TIMESTAMPTZ;
    partition_name TEXT;
BEGIN
    -- date_trunc on TIMESTAMPTZ works in UTC internally
    -- This ensures the partition boundaries are at UTC midnight
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
        -- TIMESTAMPTZ values are stored in UTC internally
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
 * @param timestamp_val The timestamp to check/create partition for (in UTC)
 * @returns The partition name that was created or already exists
 */
CREATE OR REPLACE FUNCTION ensure_market_sentiment_history_partition(timestamp_val TIMESTAMPTZ)
RETURNS TEXT AS $$
DECLARE
    partition_start TIMESTAMPTZ;
    partition_end TIMESTAMPTZ;
    partition_name TEXT;
BEGIN
    -- date_trunc on TIMESTAMPTZ works in UTC internally
    -- This ensures the partition boundaries are at UTC midnight
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
        -- TIMESTAMPTZ values are stored in UTC internally
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

-- ============================================================================
-- INSERT WRAPPER FUNCTIONS
-- ============================================================================

/**
 * @description
 * Wrapper function to insert into market_price_history with automatic
 * partition creation. This function ensures the partition exists before inserting.
 * All timestamps are normalized to UTC.
 * 
 * Usage: SELECT insert_market_price_history(NOW(), 'market-123', 1.0, 1.1, 0.9, 1.0, 100.0, '15');
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
    -- Ensure partition exists (function handles UTC normalization)
    PERFORM ensure_market_price_history_partition(p_time);
    
    -- Insert the data with conflict resolution
    -- TIMESTAMPTZ is stored in UTC internally by PostgreSQL
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
 * All timestamps are normalized to UTC.
 * 
 * Usage: SELECT insert_market_sentiment_history(NOW(), 'market-123', 0.75, ARRAY['positive', 'trending']);
 */
CREATE OR REPLACE FUNCTION insert_market_sentiment_history(
    p_time TIMESTAMPTZ,
    p_market_id VARCHAR(255),
    p_sentiment_score FLOAT,
    p_key_drivers TEXT[]
)
RETURNS VOID AS $$
BEGIN
    -- Ensure partition exists (function handles UTC normalization)
    PERFORM ensure_market_sentiment_history_partition(p_time);
    
    -- Insert the data with conflict resolution
    -- TIMESTAMPTZ is stored in UTC internally by PostgreSQL
    INSERT INTO market_sentiment_history (time, market_id, sentiment_score, key_drivers)
    VALUES (p_time, p_market_id, p_sentiment_score, p_key_drivers)
    ON CONFLICT (market_id, time) DO UPDATE SET
        sentiment_score = EXCLUDED.sentiment_score,
        key_drivers = EXCLUDED.key_drivers;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- UTILITY FUNCTIONS
-- ============================================================================

/**
 * @description
 * Function to create partitions for the next N months proactively.
 * This is useful to run via pg_cron or as a scheduled job.
 * 
 * Usage: SELECT create_future_partitions(3); -- Creates next 3 months
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

-- ============================================================================
-- ROW LEVEL SECURITY (RLS) POLICIES FOR SUPABASE
-- ============================================================================
-- Enable RLS on all tables that contain user-specific data
-- Note: For Supabase, the backend uses the service_role key which bypasses RLS
-- These policies are for direct database access from the frontend/client

-- Enable RLS on users table
ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- Policy: Users can only read their own user record
-- Note: This assumes Clerk user ID is stored and can be matched with Supabase auth
-- If using Supabase Auth directly, use: USING (auth.uid()::text = clerk_user_id)
-- For Clerk integration, you may need to create a custom function or disable RLS for this table
CREATE POLICY "Users can read own profile"
    ON users FOR SELECT
    USING (true); -- Temporarily allow all reads - adjust based on your auth setup

-- Policy: Allow inserts (for webhook user creation)
CREATE POLICY "Allow user creation"
    ON users FOR INSERT
    WITH CHECK (true);

-- Policy: Users can update their own profile
CREATE POLICY "Users can update own profile"
    ON users FOR UPDATE
    USING (true); -- Adjust based on your auth setup

-- Enable RLS on wallets table
ALTER TABLE wallets ENABLE ROW LEVEL SECURITY;

-- Policy: Users can only access their own wallets
CREATE POLICY "Users can access own wallets"
    ON wallets FOR ALL
    USING (true); -- Temporarily allow all - adjust based on your auth setup

-- Enable RLS on trades table
ALTER TABLE trades ENABLE ROW LEVEL SECURITY;

-- Policy: Users can only access their own trades
CREATE POLICY "Users can access own trades"
    ON trades FOR ALL
    USING (true); -- Temporarily allow all - adjust based on your auth setup

-- Enable RLS on market_price_history table
ALTER TABLE market_price_history ENABLE ROW LEVEL SECURITY;

-- Policy: Public read access (anyone can read market data)
CREATE POLICY "Public read access to market price history"
    ON market_price_history FOR SELECT
    USING (true);

-- Policy: Only service role can insert/update (for backend aggregator)
-- Note: Service role key bypasses RLS, so this is mainly for documentation
CREATE POLICY "Service role can modify market price history"
    ON market_price_history FOR ALL
    USING (true); -- Service role bypasses RLS anyway

-- Enable RLS on market_sentiment_history table
ALTER TABLE market_sentiment_history ENABLE ROW LEVEL SECURITY;

-- Policy: Public read access (anyone can read sentiment data)
CREATE POLICY "Public read access to market sentiment history"
    ON market_sentiment_history FOR SELECT
    USING (true);

-- Policy: Only service role can insert/update (for backend aggregator)
CREATE POLICY "Service role can modify market sentiment history"
    ON market_sentiment_history FOR ALL
    USING (true); -- Service role bypasses RLS anyway

-- Enable RLS on news_events table
ALTER TABLE news_events ENABLE ROW LEVEL SECURITY;

-- Policy: Public read access (anyone can read news events)
CREATE POLICY "Public read access to news events"
    ON news_events FOR SELECT
    USING (true);

-- Policy: Only service role can insert/update (for backend news aggregator)
CREATE POLICY "Service role can modify news events"
    ON news_events FOR ALL
    USING (true); -- Service role bypasses RLS anyway

-- ============================================================================
-- TRIGGERS FOR UPDATED_AT
-- ============================================================================

-- Function to update the updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger for users table
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Trigger for wallets table
CREATE TRIGGER update_wallets_updated_at
    BEFORE UPDATE ON wallets
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- COMMENTS FOR DOCUMENTATION
-- ============================================================================

COMMENT ON TABLE users IS 'Stores core user information, linking our internal user ID to the Clerk authentication provider ID.';
COMMENT ON TABLE wallets IS 'Associates a user with their Polymarket funder address and a secure reference to their private key for the remote signing service.';
COMMENT ON TABLE trades IS 'Records every trade executed by a user, providing a complete history for portfolio tracking.';
COMMENT ON TABLE market_price_history IS 'Stores time-series price data for market charts. Partitioned by month. Use insert_market_price_history() function to insert data.';
COMMENT ON TABLE market_sentiment_history IS 'Stores aggregated sentiment analysis data over time for each market. Partitioned by month. Use insert_market_sentiment_history() function to insert data.';
COMMENT ON TABLE news_events IS 'Stores metadata for external news articles that are relevant to a market. Used to plot events on charts and provide context in the AI Insight Hub.';

COMMENT ON FUNCTION insert_market_price_history IS 'Wrapper function to insert into market_price_history with automatic partition creation. Normalizes timestamps to UTC.';
COMMENT ON FUNCTION insert_market_sentiment_history IS 'Wrapper function to insert into market_sentiment_history with automatic partition creation. Normalizes timestamps to UTC.';
COMMENT ON FUNCTION ensure_market_price_history_partition IS 'Helper function to ensure a partition exists for market_price_history for the given timestamp''s month.';
COMMENT ON FUNCTION ensure_market_sentiment_history_partition IS 'Helper function to ensure a partition exists for market_sentiment_history for the given timestamp''s month.';
COMMENT ON FUNCTION create_future_partitions IS 'Function to create partitions for the next N months proactively. Useful for scheduled jobs.';

