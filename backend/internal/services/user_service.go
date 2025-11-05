/**
 * @description
 * This file contains the business logic for user-related operations.
 * It acts as an intermediary between the API handlers and the database layer,
 * encapsulating logic, validation, and error handling for user management.
 *
 * Key features:
 * - Separation of Concerns: Keeps business logic separate from the HTTP transport layer.
 * - Database Abstraction: Interacts with the database via the `db.Querier` interface,
 *   making it easy to mock for testing.
 * - Centralized Logging: Provides consistent logging for user-related actions.
 */

package services

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	db "github.com/poly-pro/backend/internal/db"
)

// Pre-defined errors for the user service to ensure consistent error handling.
var (
	ErrUserAlreadyExists = errors.New("a user with this clerk_id or email already exists")
)

// UserService provides methods for user-related business logic.
type UserService struct {
	store  db.Querier
	logger *slog.Logger
}

/**
 * @description
 * NewUserService creates a new instance of the UserService.
 *
 * @param store The database querier for database operations.
 * @param logger A structured logger for logging service-level events.
 * @returns A pointer to a new UserService instance.
 */
func NewUserService(store db.Querier, logger *slog.Logger) *UserService {
	return &UserService{
		store:  store,
		logger: logger,
	}
}

/**
 * @description
 * CreateUser creates a new user record in the database.
 *
 * @param ctx The context for the database operation.
 * @param clerkUserID The unique identifier for the user from Clerk.
 * @param email The user's primary email address.
 * @returns The newly created user record or an error.
 *
 * @notes
 * - It handles the specific database error for unique constraint violations
 *   (e.g., if a webhook is received twice) and returns a domain-specific error,
 *   `ErrUserAlreadyExists`.
 */
func (s *UserService) CreateUser(ctx context.Context, clerkUserID string, email string) (db.User, error) {
	s.logger.Info("attempting to create new user", "clerk_id", clerkUserID, "email", email)

	params := db.CreateUserParams{
		ClerkUserID: clerkUserID,
		Email:       email,
	}

	user, err := s.store.CreateUser(ctx, params)
	if err != nil {
		// Check for a unique constraint violation error. This is important for idempotency,
		// as Clerk might retry sending a webhook.
		if strings.Contains(err.Error(), "unique constraint") {
			s.logger.Warn("attempted to create a user that already exists", "clerk_id", clerkUserID)
			// Try to fetch the existing user - this might be a duplicate webhook
			existingUser, findErr := s.store.GetUserByClerkID(ctx, clerkUserID)
			if findErr != nil {
				// If we can't find the user (shouldn't happen with unique constraint), 
				// it might be a race condition or the constraint was on email instead of clerk_id
				// Still return ErrUserAlreadyExists to indicate this is an idempotent operation
				if errors.Is(findErr, pgx.ErrNoRows) {
					s.logger.Warn("unique constraint violation but user not found by clerk_id - may be email conflict", "clerk_id", clerkUserID)
					return db.User{}, ErrUserAlreadyExists
				}
				// For other database errors, log but still treat as idempotent
				s.logger.Error("failed to check for existing user after unique constraint violation", "error", findErr)
				return db.User{}, ErrUserAlreadyExists
			}
			// User exists - return it with ErrUserAlreadyExists to indicate idempotent operation
			return existingUser, ErrUserAlreadyExists
		}

		s.logger.Error("failed to create user in database", "error", err)
		return db.User{}, err
	}

	s.logger.Info("user created successfully", "user_id", user.ID, "clerk_id", user.ClerkUserID)
	return user, nil
}

/**
 * @description
 * GetUserByClerkID retrieves an existing user from the database by their Clerk user ID.
 *
 * @param ctx The context for the database operation.
 * @param clerkUserID The unique identifier for the user from Clerk.
 * @returns The user record or an error.
 */
func (s *UserService) GetUserByClerkID(ctx context.Context, clerkUserID string) (db.User, error) {
	user, err := s.store.GetUserByClerkID(ctx, clerkUserID)
	if err != nil {
		s.logger.Error("failed to get user by clerk ID", "error", err, "clerk_id", clerkUserID)
		return db.User{}, err
	}
	return user, nil
}

