/**
 * @description
 * Rollback migration to remove the orders table and related changes.
 */

-- Remove order_id column from trades table
DO $$ 
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'trades' AND column_name = 'order_id'
    ) THEN
        DROP INDEX IF EXISTS idx_trades_order_id;
        ALTER TABLE trades DROP COLUMN order_id;
    END IF;
END $$;

-- Drop orders table indexes
DROP INDEX IF EXISTS idx_orders_created_at;
DROP INDEX IF EXISTS idx_orders_status;
DROP INDEX IF EXISTS idx_orders_market_id;
DROP INDEX IF EXISTS idx_orders_user_id;

-- Drop orders table
DROP TABLE IF EXISTS orders;

