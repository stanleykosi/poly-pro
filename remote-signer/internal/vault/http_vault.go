/**
 * @description
 * This file provides an HTTP-based vault implementation that retrieves private keys
 * from the backend's vault service via HTTP API calls.
 *
 * Key features:
 * - HTTP Client: Makes HTTP requests to the backend to retrieve private keys
 * - Secret Reference Support: Uses secret references to look up keys
 * - Production-ready: Can be used when backend and remote-signer are separate services
 */

package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// HTTPVault is an implementation of the Vault interface that retrieves
// private keys from the backend via HTTP API.
type HTTPVault struct {
	backendURL string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewHTTPVault creates a new HTTP-based vault client.
func NewHTTPVault(backendURL string, logger *slog.Logger) *HTTPVault {
	return &HTTPVault{
		backendURL: backendURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// vaultResponse represents the response from the backend vault API.
type vaultResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Data    struct {
		PrivateKey string `json:"private_key"`
	} `json:"data,omitempty"`
}

/**
 * @description
 * GetPrivateKey retrieves a private key from the backend vault service.
 *
 * @param ctx The context for the operation
 * @param secretRef The secret reference (from wallet's signer_secret_ref)
 * @returns The private key (hex format)
 * @returns An error if retrieval fails
 */
func (v *HTTPVault) GetPrivateKey(ctx context.Context, secretRef string) (string, error) {
	if secretRef == "" {
		return "", errors.New("secret reference cannot be empty")
	}

	// Construct the API endpoint URL
	url := fmt.Sprintf("%s/api/v1/vault/keys/%s", v.backendURL, secretRef)

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Make the HTTP request
	v.logger.Info("retrieving private key from backend vault", "secret_ref", secretRef, "url", url)
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve private key: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		var errorResp struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			return "", fmt.Errorf("vault API error: %s", errorResp.Message)
		}
		return "", fmt.Errorf("vault API returned status %d", resp.StatusCode)
	}

	// Parse response
	var vaultResp vaultResponse
	if err := json.Unmarshal(body, &vaultResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if vaultResp.Status != "success" {
		return "", fmt.Errorf("vault API returned error: %s", vaultResp.Message)
	}

	if vaultResp.Data.PrivateKey == "" {
		return "", errors.New("private key not found in response")
	}

	v.logger.Info("successfully retrieved private key from backend vault", "secret_ref", secretRef)
	return vaultResp.Data.PrivateKey, nil
}

