/**
 * @description
 * This file contains the gRPC client implementation for communicating with the
 * isolated remote-signer service. It is responsible for establishing a connection
 * and providing a method to request transaction signatures.
 *
 * Key features:
 * - gRPC Client: Manages the connection to the remote-signer gRPC server.
 * - Abstraction: Provides a simple `SignTransaction` method that hides the
 *   underlying gRPC call details.
 * - Secure Communication: Configured to use an insecure connection for local
 *   development. In production, this should be updated with TLS credentials
 *   for secure communication over a private network.
 * - Context Propagation: Forwards the context of the incoming request to the
 *   gRPC call, enabling timeout and cancellation propagation.
 *
 * @dependencies
 * - google.golang.org/grpc: The Go gRPC library.
 * - github.com/poly-pro/backend/proto: The generated protobuf client stubs.
 */

package services

import (
	"context"
	"log/slog"

	"github.com/poly-pro/backend/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// SignerClient provides an interface for communicating with the remote signer service.
type SignerClient interface {
	SignTransaction(ctx context.Context, userID, payloadJSON string) (string, error)
	Close() error
}

// grpcSignerClient is the concrete implementation of the SignerClient.
type grpcSignerClient struct {
	conn   *grpc.ClientConn
	client proto.SignerClient
	logger *slog.Logger
}

/**
 * @description
 * NewSignerClient creates a new gRPC client and connects to the remote-signer service.
 *
 * @param address The network address of the remote-signer service (e.g., "localhost:8081").
 * @param logger A structured logger.
 * @returns A SignerClient interface and an error if the connection fails.
 *
 * @notes
 * - For local development, it uses an insecure connection. In production, this
 *   MUST be configured with TLS credentials.
 */
func NewSignerClient(address string, logger *slog.Logger) (SignerClient, error) {
	logger.Info("connecting to remote signer service", "address", address)

	// In a production environment, you would use grpc.WithTransportCredentials()
	// to establish a secure TLS connection.
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("failed to connect to remote signer service", "error", err)
		return nil, err
	}

	client := proto.NewSignerClient(conn)
	return &grpcSignerClient{
		conn:   conn,
		client: client,
		logger: logger,
	}, nil
}

/**
 * @description
 * SignTransaction sends a request to the remote-signer service to sign an
 * EIP-712 payload.
 *
 * @param ctx The context for the RPC call.
 * @param userID The ID of the user for whom the transaction is being signed.
 * @param payloadJSON The EIP-712 payload as a JSON string.
 * @returns The signature as a hexadecimal string.
 * @returns An error if the RPC call fails.
 */
func (c *grpcSignerClient) SignTransaction(ctx context.Context, userID, payloadJSON string) (string, error) {
	req := &proto.SignRequest{
		UserId:      userID,
		PayloadJson: payloadJSON,
	}

	c.logger.Info("sending sign request to remote signer", "user_id", userID)
	resp, err := c.client.SignTransaction(ctx, req)
	if err != nil {
		c.logger.Error("remote signer returned an error", "error", err, "user_id", userID)
		return "", err
	}

	return resp.Signature, nil
}

/**
 * @description
 * Close terminates the gRPC connection to the remote-signer service.
 * It should be called during graceful shutdown of the application.
 */
func (c *grpcSignerClient) Close() error {
	c.logger.Info("closing connection to remote signer service")
	return c.conn.Close()
}

