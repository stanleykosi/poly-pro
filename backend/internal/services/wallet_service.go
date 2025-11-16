/**
 * @description
 * This service handles wallet creation and management for users.
 * It generates Ethereum wallets, stores private keys securely in the vault,
 * and manages wallet records in the database.
 *
 * Key features:
 * - Wallet Generation: Creates new Ethereum wallets with private keys
 * - Secure Storage: Stores private keys in the vault using secure references
 * - Database Management: Creates and manages wallet records
 * - Polymarket Integration: Supports both new wallets and existing Polymarket proxy wallets
 *
 * @dependencies
 * - github.com/ethereum/go-ethereum/crypto: For wallet generation
 * - log/slog: For structured logging
 * - github.com/poly-pro/backend/internal/db: For database access
 */

package services

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/jackc/pgx/v5"
	db "github.com/poly-pro/backend/internal/db"
)

// WalletService provides methods for wallet management
type WalletService struct {
	store        db.Querier
	logger       *slog.Logger
	vaultService VaultService
	userService  UserServiceInterface // Add user service to create users if needed
}

// UserServiceInterface defines the interface for user operations needed by WalletService
type UserServiceInterface interface {
	CreateUser(ctx context.Context, clerkUserID string, email string) (db.User, error)
	GetUserByClerkID(ctx context.Context, clerkUserID string) (db.User, error)
}

// VaultService defines the interface for storing and retrieving private keys
type VaultService interface {
	StorePrivateKey(ctx context.Context, userID string, walletID string, privateKey string) (string, error) // Returns secret reference
	GetPrivateKey(ctx context.Context, secretRef string) (string, error)
	DeletePrivateKey(ctx context.Context, secretRef string) error
}

// CreateWalletParams defines parameters for creating a new wallet
type CreateWalletParams struct {
	UserID                  string
	PolymarketFunderAddress string // Optional: if provided, links existing Polymarket wallet
	PrivateKey              string // Optional: if provided, uses existing private key
}

// WalletInfo represents wallet information returned to the client
type WalletInfo struct {
	ID                      string `json:"id"`
	PolymarketFunderAddress string `json:"polymarket_funder_address"`
	IsActive                bool   `json:"is_active"`
	CreatedAt               string `json:"created_at"`
}

// NewWalletService creates a new instance of WalletService
func NewWalletService(store db.Querier, logger *slog.Logger, vaultService VaultService, userService UserServiceInterface) *WalletService {
	return &WalletService{
		store:        store,
		logger:       logger,
		vaultService: vaultService,
		userService:  userService,
	}
}

/**
 * @description
 * CreateWallet creates a new wallet for a user. It can either:
 * 1. Generate a new wallet with a random private key
 * 2. Link an existing Polymarket proxy wallet by providing the address and private key
 *
 * @param ctx The context for the operation
 * @param params The parameters for wallet creation
 * @returns WalletInfo with the created wallet details
 * @returns An error if wallet creation fails
 */
func (s *WalletService) CreateWallet(ctx context.Context, params CreateWalletParams) (WalletInfo, error) {
	s.logger.Info("creating wallet", "user_id", params.UserID)

	// 1. Get the user from the database, or create if they don't exist
	// This handles cases where the user exists in Clerk but the webhook hasn't created them yet
	user, err := s.store.GetUserByClerkID(ctx, params.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// User doesn't exist - try to create them automatically
			// Use a placeholder email since we don't have access to the user's email here
			// The email can be updated later via webhook
			placeholderEmail := params.UserID + "@clerk.placeholder"
			s.logger.Info("user not found in database, attempting to create user", "clerk_id", params.UserID)
			
			if s.userService != nil {
				createdUser, createErr := s.userService.CreateUser(ctx, params.UserID, placeholderEmail)
				if createErr != nil {
					// Check if user was created by another request (race condition)
					// The user_service returns ErrUserAlreadyExists for idempotent operations
					// We need to import the error from user_service, but since it's in the same package,
					// we can check the error message
					if strings.Contains(createErr.Error(), "already exists") {
						// User was created by another request, fetch it
						user, err = s.store.GetUserByClerkID(ctx, params.UserID)
						if err != nil {
							return WalletInfo{}, fmt.Errorf("failed to get user after creation: %w", err)
						}
						s.logger.Info("user was created by another request, fetched existing user", "user_id", user.ID, "clerk_id", params.UserID)
					} else {
						s.logger.Error("failed to create user automatically", "error", createErr, "clerk_id", params.UserID)
						return WalletInfo{}, fmt.Errorf("user not found and failed to create: %w", createErr)
					}
				} else {
					user = createdUser
					s.logger.Info("user created automatically", "user_id", user.ID, "clerk_id", params.UserID)
				}
			} else {
				// UserService not available - return error
				s.logger.Warn("user not found and userService not available", "clerk_id", params.UserID)
				return WalletInfo{}, errors.New("user not found in database. Please ensure you have completed sign-up and the user webhook has been processed")
			}
		} else {
			return WalletInfo{}, fmt.Errorf("failed to get user: %w", err)
		}
	}

	// 2. Check if user already has an active wallet
	existingWallet, err := s.store.GetActiveWalletByUserID(ctx, user.ID)
	if err == nil && existingWallet.ID.Valid {
		s.logger.Warn("user already has an active wallet", "user_id", user.ID, "wallet_id", existingWallet.ID)
		return WalletInfo{}, errors.New("user already has an active wallet")
	}

	var privateKeyHex string
	var funderAddress string

	// 3. Determine wallet creation method
	if params.PolymarketFunderAddress != "" && params.PrivateKey != "" {
		// Linking existing Polymarket wallet
		s.logger.Info("linking existing Polymarket wallet", "funder_address", params.PolymarketFunderAddress)
		
		// Validate the private key and derive address
		privateKeyHex = params.PrivateKey
		if !common.IsHexAddress(params.PolymarketFunderAddress) {
			return WalletInfo{}, errors.New("invalid funder address format")
		}
		funderAddress = common.HexToAddress(params.PolymarketFunderAddress).Hex()

		// Verify the private key matches the address
		privateKey, err := crypto.HexToECDSA(privateKeyHex)
		if err != nil {
			return WalletInfo{}, fmt.Errorf("invalid private key: %w", err)
		}
		publicKey := privateKey.Public()
		publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
		if !ok {
			return WalletInfo{}, errors.New("failed to cast public key to ECDSA")
		}
		derivedAddress := crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
		if derivedAddress != funderAddress {
			return WalletInfo{}, errors.New("private key does not match the provided address")
		}
	} else {
		// Generate new wallet
		s.logger.Info("generating new wallet")
		
		// Generate a random private key
		privateKey, err := crypto.GenerateKey()
		if err != nil {
			return WalletInfo{}, fmt.Errorf("failed to generate private key: %w", err)
		}

		// Convert to hex string
		privateKeyBytes := crypto.FromECDSA(privateKey)
		privateKeyHex = hexutil.Encode(privateKeyBytes)

		// Derive the Ethereum address from the private key
		publicKey := privateKey.Public()
		publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
		if !ok {
			return WalletInfo{}, errors.New("failed to cast public key to ECDSA")
		}
		funderAddress = crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
	}

	// 4. Store the private key in the vault and get the secret reference
	// We'll use a temporary wallet ID for now, then update it after wallet creation
	tempWalletID := generateTempID()
	secretRef, err := s.vaultService.StorePrivateKey(ctx, params.UserID, tempWalletID, privateKeyHex)
	if err != nil {
		return WalletInfo{}, fmt.Errorf("failed to store private key in vault: %w", err)
	}

	// 5. Create the wallet record in the database
	createParams := db.CreateWalletParams{
		UserID:                  user.ID,
		PolymarketFunderAddress: funderAddress,
		SignerSecretRef:         secretRef,
	}

	wallet, err := s.store.CreateWallet(ctx, createParams)
	if err != nil {
		// Clean up: delete the private key from vault if database insert fails
		_ = s.vaultService.DeletePrivateKey(ctx, secretRef)
		return WalletInfo{}, fmt.Errorf("failed to create wallet in database: %w", err)
	}

	s.logger.Info("wallet created successfully",
		"user_id", user.ID,
		"wallet_id", wallet.ID,
		"funder_address", funderAddress)

	// 6. Return wallet info (without private key)
	return WalletInfo{
		ID:                      wallet.ID.String(),
		PolymarketFunderAddress: wallet.PolymarketFunderAddress,
		IsActive:                wallet.IsActive,
		CreatedAt:               wallet.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

/**
 * @description
 * GetUserWallets retrieves all wallets for a user
 *
 * @param ctx The context for the operation
 * @param userID The Clerk user ID
 * @returns A slice of WalletInfo
 * @returns An error if retrieval fails
 */
func (s *WalletService) GetUserWallets(ctx context.Context, userID string) ([]WalletInfo, error) {
	// Get the user from the database
	user, err := s.store.GetUserByClerkID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// For now, we only support one active wallet per user
	// In the future, we could add a GetWalletsByUserID query
	wallet, err := s.store.GetActiveWalletByUserID(ctx, user.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return []WalletInfo{}, nil // No wallets found
		}
		return nil, fmt.Errorf("failed to get wallets: %w", err)
	}

	return []WalletInfo{
		{
			ID:                      wallet.ID.String(),
			PolymarketFunderAddress: wallet.PolymarketFunderAddress,
			IsActive:                wallet.IsActive,
			CreatedAt:               wallet.CreatedAt.Time.Format("2006-01-02T15:04:05Z07:00"),
		},
	}, nil
}

// generateTempID generates a temporary ID for vault storage before wallet creation
func generateTempID() string {
	// Generate a random 32-byte ID
	b := make([]byte, 32)
	rand.Read(b)
	return hexutil.Encode(b)
}

