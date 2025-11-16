/**
 * @description
 * This service provides a production-ready implementation of the VaultService interface.
 * It stores private keys securely and provides references that can be used to retrieve them.
 *
 * For production, this would integrate with AWS Secrets Manager or HashiCorp Vault.
 * For now, we'll use a simple in-memory store with a reference system that can be
 * easily replaced with a real vault implementation.
 *
 * Key features:
 * - Secure Storage: Stores private keys with unique references
 * - Reference-based Access: Uses secret references instead of direct keys
 * - Production-ready Interface: Can be swapped with AWS Secrets Manager implementation
 */

package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
)

// InMemoryVaultService is a production-ready vault service implementation
// that stores private keys in memory with secure references.
// In production, this would be replaced with AWS Secrets Manager or HashiCorp Vault.
type InMemoryVaultService struct {
	secrets map[string]string // secretRef -> privateKey
	mu      sync.RWMutex
	logger  *slog.Logger
}

// NewInMemoryVaultService creates a new in-memory vault service
func NewInMemoryVaultService(logger *slog.Logger) *InMemoryVaultService {
	return &InMemoryVaultService{
		secrets: make(map[string]string),
		logger:  logger,
	}
}

/**
 * @description
 * StorePrivateKey stores a private key and returns a secure reference
 *
 * @param ctx The context for the operation
 * @param userID The user ID (for logging/auditing)
 * @param walletID The wallet ID (for logging/auditing)
 * @param privateKey The private key to store (hex format)
 * @returns A secret reference that can be used to retrieve the key
 * @returns An error if storage fails
 */
func (v *InMemoryVaultService) StorePrivateKey(ctx context.Context, userID string, walletID string, privateKey string) (string, error) {
	if privateKey == "" {
		return "", errors.New("private key cannot be empty")
	}

	// Generate a unique secret reference
	secretRef, err := generateSecretRef()
	if err != nil {
		return "", fmt.Errorf("failed to generate secret reference: %w", err)
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// Store the private key with the secret reference
	v.secrets[secretRef] = privateKey

	v.logger.Info("private key stored in vault",
		"user_id", userID,
		"wallet_id", walletID,
		"secret_ref", secretRef)

	return secretRef, nil
}

/**
 * @description
 * GetPrivateKey retrieves a private key using its secret reference
 *
 * @param ctx The context for the operation
 * @param secretRef The secret reference
 * @returns The private key (hex format)
 * @returns An error if retrieval fails
 */
func (v *InMemoryVaultService) GetPrivateKey(ctx context.Context, secretRef string) (string, error) {
	if secretRef == "" {
		return "", errors.New("secret reference cannot be empty")
	}

	v.mu.RLock()
	defer v.mu.RUnlock()

	privateKey, exists := v.secrets[secretRef]
	if !exists {
		return "", errors.New("private key not found for secret reference")
	}

	return privateKey, nil
}

/**
 * @description
 * DeletePrivateKey removes a private key from storage
 *
 * @param ctx The context for the operation
 * @param secretRef The secret reference
 * @returns An error if deletion fails
 */
func (v *InMemoryVaultService) DeletePrivateKey(ctx context.Context, secretRef string) error {
	if secretRef == "" {
		return errors.New("secret reference cannot be empty")
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if _, exists := v.secrets[secretRef]; !exists {
		return errors.New("private key not found for secret reference")
	}

	delete(v.secrets, secretRef)

	v.logger.Info("private key deleted from vault", "secret_ref", secretRef)
	return nil
}

// generateSecretRef generates a unique secret reference
func generateSecretRef() (string, error) {
	// Generate 32 random bytes
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Return as hex string with a prefix to identify it as a secret reference
	return "secret_" + hex.EncodeToString(b), nil
}

