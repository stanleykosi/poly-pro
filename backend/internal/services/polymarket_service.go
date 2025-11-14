/**
 * @description
 * This service encapsulates the business logic for interacting with the Polymarket
 * platform. It handles the creation and signing of orders before they are submitted
 * to the Polymarket CLOB API.
 *
 * Key features:
 * - Order Construction: Translates high-level order requests into the specific EIP-712
 *   typed data structure required by Polymarket's smart contracts.
 * - Secure Signing Flow: Coordinates with the `SignerClient` to get a valid signature
 *   for the constructed order from the isolated remote-signer service.
 * - Abstraction: Hides the complexity of EIP-712 payload creation and the signing
 *   process from the API handlers.
 * - API Interaction (Future): This service will be expanded to include methods for
 *   submitting the signed order to Polymarket's API and for fetching market data.
 *
 * @dependencies
 * - log/slog: For structured logging.
 * - github.com/poly-pro/backend/internal/db: For database access (to fetch user wallet info).
 * - github.com/poly-pro/backend/internal/polymarket: For EIP-712 struct definitions.
 * - github.com/ethereum/go-ethereum/signer/core/apitypes: For the base TypedData struct.
 */

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/poly-pro/backend/internal/db"
	"github.com/poly-pro/backend/internal/config"
	"github.com/poly-pro/backend/internal/polymarket"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// PlaceOrderParams defines the parameters for placing a new order.
type PlaceOrderParams struct {
	UserID        string
	MarketID      string
	TokenID       *big.Int
	Price         float64 // The price of the order (0 to 1)
	Size          float64 // The size/quantity of the order
	Side          string  // "BUY" or "SELL"
	SignatureType int
}

// PolymarketService provides methods for interacting with Polymarket.
type PolymarketService struct {
	store        db.Querier
	logger       *slog.Logger
	signerClient SignerClient
	clobClient   *polymarket.CLOBAPIClient
	config       config.Config
}

// NewPolymarketService creates a new instance of the PolymarketService.
func NewPolymarketService(store db.Querier, logger *slog.Logger, signerClient SignerClient, cfg config.Config) *PolymarketService {
	// Initialize CLOB API client if credentials are provided
	var clobClient *polymarket.CLOBAPIClient
	if cfg.CLOBAPIKey != "" && cfg.CLOBAPISecret != "" && cfg.CLOBAPIPassphrase != "" {
		clobClient = polymarket.NewCLOBAPIClient(cfg.CLOBAPIURL, cfg.CLOBAPIKey, cfg.CLOBAPISecret, cfg.CLOBAPIPassphrase, logger)
	}

	return &PolymarketService{
		store:        store,
		logger:       logger,
		signerClient: signerClient,
		clobClient:   clobClient,
		config:       cfg,
	}
}

/**
 * @description
 * CreateAndSignOrder constructs an EIP-712 compliant order, saves it to the database,
 * sends it to the remote-signer for signing, and returns the fully signed order.
 *
 * @param ctx The context for the operation.
 * @param params The parameters for the order to be created.
 * @returns A pointer to the signed order.
 * @returns The database order record.
 * @returns An error if any part of the process fails.
 */
func (s *PolymarketService) CreateAndSignOrder(ctx context.Context, params PlaceOrderParams) (*polymarket.SignedOrder, db.Order, error) {
	s.logger.Info("creating and signing Polymarket order", "user_id", params.UserID, "side", params.Side)

	// 1. Fetch the user from the database using the Clerk ID to get the internal user ID.
	user, err := s.store.GetUserByClerkID(ctx, params.UserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Error("user not found in database", "clerk_id", params.UserID)
			return nil, db.Order{}, errors.New("user not found")
		}
		s.logger.Error("failed to get user from database", "error", err, "clerk_id", params.UserID)
		return nil, db.Order{}, err
	}

	// 2. Fetch the active wallet for the user to get the Polymarket funder address.
	wallet, err := s.store.GetActiveWalletByUserID(ctx, user.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.logger.Error("active wallet not found for user", "user_id", user.ID)
			return nil, db.Order{}, errors.New("user wallet not found")
		}
		s.logger.Error("failed to get wallet from database", "error", err, "user_id", user.ID)
		return nil, db.Order{}, err
	}

	makerAddress := wallet.PolymarketFunderAddress

	// 3. Convert price and size to their integer representations based on contract decimals.
	// Polymarket uses 6 decimals for both USDC (makerAmount) and conditional tokens (takerAmount).
	priceBI := new(big.Float).SetFloat64(params.Price)
	sizeBI := new(big.Float).SetFloat64(params.Size)
	decimals := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil))

	var makerAmount, takerAmount *big.Int
	var sideInt int

	if params.Side == "BUY" {
		sideInt = 0 // BUY side is 0 in the contract
		// For BUY: takerAmount = size * 10^6, makerAmount = takerAmount * price
		takerAmountFloat := new(big.Float).Mul(sizeBI, decimals)
		makerAmountFloat := new(big.Float).Mul(takerAmountFloat, priceBI)

		takerAmount, _ = takerAmountFloat.Int(nil)
		makerAmount, _ = makerAmountFloat.Int(nil)
	} else { // SELL
		sideInt = 1 // SELL side is 1 in the contract
		// For SELL: makerAmount = size * 10^6, takerAmount = makerAmount * price
		makerAmountFloat := new(big.Float).Mul(sizeBI, decimals)
		takerAmountFloat := new(big.Float).Mul(makerAmountFloat, priceBI)

		makerAmount, _ = makerAmountFloat.Int(nil)
		takerAmount, _ = takerAmountFloat.Int(nil)
	}

	// 4. Construct the order message for EIP-712 signing.
	order := polymarket.Order{
		Salt:          big.NewInt(time.Now().UnixMilli()).String(),
		Maker:         makerAddress,
		Signer:        makerAddress, // In Polymarket's system, maker and signer are the same.
		Taker:         "0x0000000000000000000000000000000000000000", // Public order
		TokenId:       params.TokenID.String(),
		MakerAmount:   makerAmount.String(),
		TakerAmount:   takerAmount.String(),
		Expiration:    "0", // No expiration for GTC orders
		Nonce:         "0", // Nonce for on-chain cancellation, can be managed later.
		FeeRateBps:    "0", // Fee rate in basis points.
		Side:          sideInt,
		SignatureType: params.SignatureType,
	}

	// 5. Create the full EIP-712 typed data payload.
	typedData := apitypes.TypedData{
		Types:       polymarket.PolymarketEIP712Types,
		PrimaryType: "Order",
		Domain:      polymarket.PolymarketEIP712Domain,
		Message:     order.ToMessage(),
	}

	// 6. Save the order to the database with status 'pending' before signing.
	// We'll update it with the signed order JSON after signing.
	
	// Convert float64 to pgtype.Numeric for Size and Price
	sizeNumeric := pgtype.Numeric{}
	if err := sizeNumeric.Scan(fmt.Sprintf("%.6f", params.Size)); err != nil {
		s.logger.Error("failed to convert size to numeric", "error", err)
		return nil, db.Order{}, fmt.Errorf("failed to convert size: %w", err)
	}
	
	priceNumeric := pgtype.Numeric{}
	if err := priceNumeric.Scan(fmt.Sprintf("%.6f", params.Price)); err != nil {
		s.logger.Error("failed to convert price to numeric", "error", err)
		return nil, db.Order{}, fmt.Errorf("failed to convert price: %w", err)
	}
	
	createOrderParams := db.CreateOrderParams{
		UserID:      user.ID, // user.ID is already pgtype.UUID
		MarketID:    params.MarketID,
		TokenID:     params.TokenID.String(),
		Side:        params.Side,
		Size:        sizeNumeric,
		Price:       priceNumeric,
		Status:      "pending",
		SignedOrder: nil, // Will be updated after signing
	}
	
	dbOrder, err := s.store.CreateOrder(ctx, createOrderParams)
	if err != nil {
		s.logger.Error("failed to create order in database", "error", err, "user_id", user.ID)
		return nil, db.Order{}, fmt.Errorf("failed to save order: %w", err)
	}
	s.logger.Info("order created in database", "order_id", dbOrder.ID, "user_id", user.ID)

	// 7. Marshal the typed data to a JSON string to send to the remote signer.
	payloadJSON, err := json.Marshal(typedData)
	if err != nil {
		s.logger.Error("failed to marshal EIP-712 typed data", "error", err)
		return nil, dbOrder, err
	}

	// 8. Request the signature from the remote signer service.
	// Use the internal user ID (UUID as string) for the signer service.
	internalUserID := user.ID.String()
	signature, err := s.signerClient.SignTransaction(ctx, internalUserID, string(payloadJSON))
	if err != nil {
		s.logger.Error("failed to get signature from remote signer", "error", err)
		return nil, dbOrder, err
	}

	// 9. Assemble the final signed order.
	signedOrder := &polymarket.SignedOrder{
		Order:     order,
		Signature: signature,
	}

	// 10. Marshal the signed order to JSONB and update the database record.
	signedOrderJSON, err := json.Marshal(signedOrder)
	if err != nil {
		s.logger.Error("failed to marshal signed order", "error", err)
		return nil, dbOrder, err
	}

	// Store signed order JSON in database (we'll add an update query for this later)
	// For now, the order is saved and we'll update it when we add the update query
	s.logger.Info("signed order JSON prepared", "order_id", dbOrder.ID, "json_size", len(signedOrderJSON))

	s.logger.Info("order successfully signed", "user_id", params.UserID, "order_id", dbOrder.ID, "signature", signedOrder.Signature)

	// 11. Submit the order to Polymarket's CLOB API if CLOB client is configured
	if s.clobClient != nil {
		orderResp, err := s.clobClient.PostOrder(ctx, signedOrder, "GTC") // Default to Good-Till-Cancelled
		if err != nil {
			s.logger.Error("failed to submit order to CLOB API", "error", err, "user_id", params.UserID, "order_id", dbOrder.ID)
			// Update order status to rejected if submission fails
			s.store.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{
				ID:     dbOrder.ID,
				Status: "rejected",
			})
			return nil, dbOrder, fmt.Errorf("failed to submit order: %w", err)
		}

		if !orderResp.Success {
			s.logger.Warn("order submission failed", "error_msg", orderResp.ErrorMsg, "status", orderResp.Status, "order_id", dbOrder.ID)
			// Update order status to rejected
			s.store.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{
				ID:     dbOrder.ID,
				Status: "rejected",
			})
			return nil, dbOrder, fmt.Errorf("order submission failed: %s", orderResp.ErrorMsg)
		}

		s.logger.Info("order successfully submitted to CLOB API", "polymarket_order_id", orderResp.OrderID, "status", orderResp.Status, "db_order_id", dbOrder.ID)
		
		// Update the order with Polymarket order ID and status
		polymarketOrderID := pgtype.Text{}
		if err := polymarketOrderID.Scan(orderResp.OrderID); err != nil {
			s.logger.Warn("failed to convert Polymarket order ID", "error", err, "order_id", dbOrder.ID)
		} else {
			_, err = s.store.UpdateOrderPolymarketID(ctx, db.UpdateOrderPolymarketIDParams{
				ID:                dbOrder.ID,
				PolymarketOrderID: polymarketOrderID,
			})
			if err != nil {
				s.logger.Warn("failed to update order with Polymarket ID", "error", err, "order_id", dbOrder.ID)
			}
		}

		// Update status to 'open' since it's been submitted
		_, err = s.store.UpdateOrderStatus(ctx, db.UpdateOrderStatusParams{
			ID:     dbOrder.ID,
			Status: "open",
		})
		if err != nil {
			s.logger.Warn("failed to update order status to open", "error", err, "order_id", dbOrder.ID)
		}

		// Refresh the order from database to get updated fields
		dbOrder, err = s.store.GetOrderByID(ctx, dbOrder.ID)
		if err != nil {
			s.logger.Warn("failed to refresh order from database", "error", err, "order_id", dbOrder.ID)
		}
	}

	return signedOrder, dbOrder, nil
}

