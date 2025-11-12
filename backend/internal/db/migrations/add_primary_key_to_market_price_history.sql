/**
 * @description
 * Migration to add PRIMARY KEY to market_price_history table.
 * This ensures data integrity and allows for proper conflict resolution.
 * 
 * The primary key is a composite of (market_id, time, resolution) to ensure
 * uniqueness of OHLCV bars for each market at each time point and resolution.
 * 
 * Run this in Supabase SQL Editor:
 * 1. Copy the contents of this file
 * 2. Paste into Supabase SQL Editor
 * 3. Execute
 */

-- Step 1: Check for and remove any duplicate rows before adding the primary key
-- This query will show duplicates (run this first to see if there are any):
-- SELECT market_id, time, resolution, COUNT(*) as count
-- FROM market_price_history
-- GROUP BY market_id, time, resolution
-- HAVING COUNT(*) > 1;

-- Step 2: Remove duplicates if they exist (keeps the first occurrence)
-- Uncomment and run this if duplicates are found:
/*
DELETE FROM market_price_history
WHERE ctid NOT IN (
    SELECT MIN(ctid)
    FROM market_price_history
    GROUP BY market_id, time, resolution
);
*/

-- Step 3: Add PRIMARY KEY constraint to market_price_history
-- PostgreSQL 11+ supports PRIMARY KEY on partitioned tables if it includes the partition key
-- Since we're partitioning by 'time', and our primary key includes 'time', this should work
-- If this fails, we'll need to add the constraint to each partition individually (see alternative below)

ALTER TABLE market_price_history
ADD CONSTRAINT market_price_history_pkey PRIMARY KEY (market_id, time, resolution);

-- Alternative approach if the above fails (add PRIMARY KEY to each partition):
-- Uncomment this block if you get an error about partitioned tables not supporting primary keys
/*
DO $$
DECLARE
    partition_record RECORD;
BEGIN
    FOR partition_record IN
        SELECT schemaname, tablename
        FROM pg_tables
        WHERE schemaname = current_schema()
        AND tablename LIKE 'market_price_history_y%'
    LOOP
        BEGIN
            EXECUTE format('ALTER TABLE %I.%I ADD CONSTRAINT %I PRIMARY KEY (market_id, time, resolution)',
                partition_record.schemaname,
                partition_record.tablename,
                partition_record.tablename || '_pkey');
            RAISE NOTICE 'Added primary key to partition: %', partition_record.tablename;
        EXCEPTION
            WHEN duplicate_object THEN
                RAISE NOTICE 'Primary key already exists on partition: %', partition_record.tablename;
        END;
    END LOOP;
END $$;
*/

-- Also update the insert function to handle conflicts (ON CONFLICT DO UPDATE)
-- This prevents duplicate inserts and allows updates if needed
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

