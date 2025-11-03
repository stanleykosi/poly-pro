/**
 * @description
 * This file contains all the SQL queries for interacting with the 'wallets' table.
 * These queries are used by sqlc to generate type-safe Go code for our database access layer.
 */

-- name: CreateWallet :one
-- @description Associates a new Polymarket funder address with a user.
INSERT INTO wallets (
  user_id,
  polymarket_funder_address,
  signer_secret_ref
) VALUES (
  $1, $2, $3
)
RETURNING *;

-- name: GetActiveWalletByUserID :one
-- @description Retrieves the active wallet for a given user.
-- This is used to fetch the signer_secret_ref needed for transaction signing.
SELECT * FROM wallets
WHERE user_id = $1 AND is_active = TRUE
LIMIT 1;

