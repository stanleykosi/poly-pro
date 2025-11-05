/**
 * @description
 * This file implements the gRPC server for the remote signing service.
 * It handles incoming RPC requests, orchestrates the signing process by using
 * the vault and crypto packages, and returns the response.
 *
 * Key features:
 * - gRPC Service Implementation: Implements the `SignerServer` interface generated
 *   from the `signer.proto` file.
 * - Dependency Injection: The server struct holds dependencies (logger, vault, signer),
 *   making it modular and easy to test.
 * - Robust Error Handling: Returns specific gRPC status codes (e.g., `InvalidArgument`,
 *   `Internal`, `Unauthenticated`) to provide clear error information to the client.
 * - Orchestration Logic: The `SignTransaction` method coordinates the flow:
 *   1. Validate input.
 *   2. Retrieve the private key from the vault.
 *   3. Perform the cryptographic signing.
 *   4. Return the signature.
 */

package server

import (
	"context"
	"log/slog"

	"github.com/poly-pro/remote-signer/internal/crypto"
	"github.com/poly-pro/remote-signer/internal/vault"
	"github.com/poly-pro/remote-signer/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the gRPC Signer service.
type Server struct {
	proto.UnimplementedSignerServer // Recommended for forward compatibility
	logger                          *slog.Logger
	vault                           vault.Vault
	signer                          *crypto.Signer
}

/**
 * @description
 * NewGRPCServer creates a new instance of the gRPC server.
 *
 * @param logger A structured logger.
 * @param v The vault implementation for fetching private keys.
 * @param s The crypto signer for performing signing operations.
 * @returns A pointer to a new Server instance.
 */
func NewGRPCServer(logger *slog.Logger, v vault.Vault, s *crypto.Signer) *Server {
	return &Server{
		logger: logger,
		vault:  v,
		signer: s,
	}
}

/**
 * @description
 * SignTransaction is the RPC handler for signing a transaction.
 * It validates the request, retrieves the appropriate private key, signs the payload,
 * and returns the signature.
 *
 * @param ctx The context of the gRPC request.
 * @param req The SignRequest message from the client.
 * @returns A SignResponse message containing the signature.
 * @returns An error with an appropriate gRPC status code if the process fails.
 */
func (s *Server) SignTransaction(ctx context.Context, req *proto.SignRequest) (*proto.SignResponse, error) {
	s.logger.Info("received sign transaction request", "user_id", req.UserId)

	// 1. Validate the incoming request.
	if req.UserId == "" {
		s.logger.Warn("sign request rejected: missing user_id")
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.PayloadJson == "" {
		s.logger.Warn("sign request rejected: missing payload_json", "user_id", req.UserId)
		return nil, status.Error(codes.InvalidArgument, "payload_json is required")
	}

	// 2. Fetch the private key from the vault.
	// This is a critical step where a real implementation would securely retrieve
	// the user-specific key. Our mock vault returns a dummy key.
	privateKey, err := s.vault.GetPrivateKey(ctx, req.UserId)
	if err != nil {
		s.logger.Error("failed to get private key from vault", "error", err, "user_id", req.UserId)
		// We return an `Unauthenticated` error because failing to get a key is an auth-level failure.
		return nil, status.Error(codes.Unauthenticated, "could not retrieve signing key for user")
	}

	// 3. Sign the payload using the cryptographic signer.
	signature, err := s.signer.SignTypedData(privateKey, req.PayloadJson)
	if err != nil {
		s.logger.Error("failed to sign typed data", "error", err, "user_id", req.UserId)
		// An `Internal` error is appropriate as this indicates a server-side processing failure.
		return nil, status.Error(codes.Internal, "failed to sign payload")
	}

	// 4. Return the successful response.
	s.logger.Info("successfully processed sign transaction request", "user_id", req.UserId)
	return &proto.SignResponse{
		Signature: signature,
	}, nil
}

