/**
 * @description
 * This file defines the abstraction for retrieving secrets, such as private keys.
 * It introduces a `Vault` interface to decouple the signing logic from the specific
 * implementation of the secret store.
 *
 * Key features:
 * - Interface-based Design: The `Vault` interface allows for interchangeable secret
 *   management backends (e.g., a local mock, AWS Secrets Manager, HashiCorp Vault).
 * - Mock Implementation: A `MockVault` is provided for local development. It securely
 *   fetches a private key from an environment variable.
 *
 * @notes
 * - For the MVP, the `MockVault`'s `GetPrivateKey` method ignores the `userID` and
 *   returns the same dummy key for all requests. A production implementation would
 *   use the `userID` to fetch the correct user-specific key.
 * - This design is crucial for security and testability.
 */

package vault

import (
	"context"
	"errors"
	"log/slog"
)

// Vault defines the interface for a secret store.
type Vault interface {
	// GetPrivateKey retrieves the private key for a given user.
	// In a production system, this would fetch the key from a secure store like
	// AWS Secrets Manager or HashiCorp Vault using the user's ID as a reference.
	GetPrivateKey(ctx context.Context, userID string) (string, error)
}

// MockVault is an implementation of the Vault interface for local development.
// It retrieves a single dummy private key from an environment variable.
type MockVault struct {
	privateKey string
	logger     *slog.Logger
}

/**
 * @description
 * NewMockVault creates a new instance of the MockVault.
 *
 * @param privateKey The dummy private key to be used for all signing operations.
 * @param logger A structured logger for logging vault-related events.
 * @returns A pointer to a new MockVault instance.
 * @returns An error if the provided private key is empty.
 */
func NewMockVault(privateKey string, logger *slog.Logger) (*MockVault, error) {
	if privateKey == "" {
		return nil, errors.New("private key cannot be empty for mock vault")
	}
	logger.Warn("initializing mock vault with a dummy private key. THIS IS NOT FOR PRODUCTION USE.")
	return &MockVault{
		privateKey: privateKey,
		logger:     logger,
	}, nil
}

/**
 * @description
 * GetPrivateKey for the MockVault returns the pre-configured dummy private key.
 *
 * @param ctx The context for the operation (unused in mock).
 * @param userID The user's ID (unused in mock).
 * @returns The dummy private key.
 * @returns An error (always nil for the mock implementation).
 */
func (v *MockVault) GetPrivateKey(ctx context.Context, userID string) (string, error) {
	v.logger.Info("retrieving dummy private key from mock vault", "for_user_id", userID)
	// In this mock implementation, we return the same key for every user.
	// A real implementation would use the userID to look up the correct key.
	return v.privateKey, nil
}

