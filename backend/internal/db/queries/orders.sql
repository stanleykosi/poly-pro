/**
 * @description
 * This file contains all the SQL queries for interacting with the 'orders' table.
 * These queries are used by sqlc to generate type-safe Go code for our database access layer.
 */

-- name: CreateOrder :one
-- @description Creates a new order in the database with status 'pending'.
-- This is called when an order is first placed, before it's submitted to Polymarket.
INSERT INTO orders (
  user_id,
  market_id,
  token_id,
  side,
  size,
  price,
  status,
  signed_order
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: UpdateOrderStatus :one
-- @description Updates the status of an order and sets the appropriate timestamp.
-- Status can be: 'pending', 'open', 'filled', 'cancelled', 'rejected'
UPDATE orders
SET 
  status = $2,
  updated_at = NOW(),
  submitted_at = CASE WHEN $2 = 'open' AND submitted_at IS NULL THEN NOW() ELSE submitted_at END,
  filled_at = CASE WHEN $2 = 'filled' AND filled_at IS NULL THEN NOW() ELSE filled_at END,
  cancelled_at = CASE WHEN $2 = 'cancelled' AND cancelled_at IS NULL THEN NOW() ELSE cancelled_at END
WHERE id = $1
RETURNING *;

-- name: UpdateOrderPolymarketID :one
-- @description Updates the Polymarket order ID after the order is submitted to Polymarket.
UPDATE orders
SET 
  polymarket_order_id = $2,
  updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: GetOrderByID :one
-- @description Retrieves a single order by its ID.
SELECT * FROM orders
WHERE id = $1
LIMIT 1;

-- name: GetOrdersByUserID :many
-- @description Retrieves all orders for a specific user, ordered by creation date (newest first).
SELECT * FROM orders
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: GetOrdersByUserIDAndStatus :many
-- @description Retrieves orders for a specific user filtered by status.
SELECT * FROM orders
WHERE user_id = $1 AND status = $2
ORDER BY created_at DESC;

-- name: GetOrdersByMarketID :many
-- @description Retrieves all orders for a specific market.
SELECT * FROM orders
WHERE market_id = $1
ORDER BY created_at DESC;

