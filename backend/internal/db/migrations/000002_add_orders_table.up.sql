/**
 * @description
 * Migration to add the orders table for tracking placed orders.
 * This migration adds:
 * - orders table with status tracking
 * - Foreign key relationship from trades to orders
 * - Indexes for performance
 */

-- Table: orders
-- Records all placed orders (pending, filled, cancelled) for tracking and order management.
CREATE TABLE IF NOT EXISTS orders (
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

-- Indexes for orders table
CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
CREATE INDEX IF NOT EXISTS idx_orders_market_id ON orders(market_id);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders(created_at DESC);

-- Add order_id column to trades table if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'trades' AND column_name = 'order_id'
    ) THEN
        ALTER TABLE trades ADD COLUMN order_id UUID REFERENCES orders(id) ON DELETE SET NULL;
        CREATE INDEX IF NOT EXISTS idx_trades_order_id ON trades(order_id);
    END IF;
END $$;

