/**
 * @description
 * This file contains all the SQL queries for interacting with the 'users' table.
 * These queries are used by sqlc to generate type-safe Go code for our database access layer.
 */

-- name: CreateUser :one
-- @description Creates a new user in the database.
-- This is typically called after a 'user.created' webhook event from Clerk.
INSERT INTO users (
  clerk_user_id,
  email
) VALUES (
  $1, $2
)
RETURNING *;

-- name: GetUserByClerkID :one
-- @description Retrieves a single user from the database based on their unique Clerk User ID.
-- This will be used frequently in authentication middleware to identify the requesting user.
SELECT * FROM users
WHERE clerk_user_id = $1
LIMIT 1;

