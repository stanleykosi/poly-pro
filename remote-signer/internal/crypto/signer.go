/**
 * @description
 * This file contains the core cryptographic logic for the remote signing service.
 * It is responsible for performing EIP-712 compliant typed data signing.
 *
 * Key features:
 * - EIP-712 Signing: Implements signing of structured data as per the EIP-712 standard,
 *   which is used by Polymarket for orders.
 * - Go-Ethereum Integration: Leverages the robust and widely-used `go-ethereum` library
 *   for all cryptographic operations, ensuring correctness and security.
 * - Abstraction: Encapsulates the complexity of parsing EIP-712 data and performing
 *   the signing operation into a single, clean function.
 *
 * @dependencies
 * - github.com/ethereum/go-ethereum/crypto: For key management and signing.
 * - github.com/ethereum/go-ethereum/signer/core/apitypes: For EIP-712 data structures.
 */

package crypto

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// Signer is responsible for cryptographic operations.
type Signer struct {
	logger *slog.Logger
}

/**
 * @description
 * NewSigner creates a new instance of the Signer.
 *
 * @param logger A structured logger for logging cryptographic operations.
 * @returns A pointer to a new Signer instance.
 */
func NewSigner(logger *slog.Logger) *Signer {
	return &Signer{
		logger: logger,
	}
}

/**
 * @description
 * SignTypedData signs an EIP-712 typed data payload with a given private key.
 *
 * @param privateKeyHex The hexadecimal string representation of the ECDSA private key.
 * @param payloadJSON The EIP-712 typed data payload, formatted as a JSON string.
 * @returns The resulting signature as a hexadecimal string.
 * @returns An error if any part of the process fails (key parsing, JSON parsing, or signing).
 */
func (s *Signer) SignTypedData(privateKeyHex string, payloadJSON string) (string, error) {
	// 1. Parse the private key from its hex string representation.
	// The "0x" prefix is optional for `crypto.HexToECDSA`.
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		s.logger.Error("failed to parse private key from hex", "error", err)
		return "", errors.New("invalid private key format")
	}

	// 2. Unmarshal the JSON payload into the EIP-712 TypedData structure.
	var typedData apitypes.TypedData
	if err := json.Unmarshal([]byte(payloadJSON), &typedData); err != nil {
		s.logger.Error("failed to unmarshal EIP-712 payload JSON", "error", err, "payload", payloadJSON)
		return "", errors.New("invalid EIP-712 payload JSON")
	}

	// 3. Get the EIP-712 domain and message hashes.
	// The `apitypes` package provides helper functions to correctly hash the data.
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		s.logger.Error("failed to hash EIP712 domain", "error", err)
		return "", errors.New("failed to hash EIP712 domain")
	}
	messageHash, err := typedData.HashStruct(typedData.PrimaryType, typedData.Message)
	if err != nil {
		s.logger.Error("failed to hash EIP712 message", "error", err)
		return "", errors.New("failed to hash EIP712 message")
	}

	// 4. Construct the final digest to be signed, as per the EIP-712 specification.
	// The format is: `\x19\x01` + `domainSeparator` + `messageHash`.
	prefixedData := []byte{0x19, 0x01}
	prefixedData = append(prefixedData, domainSeparator...)
	prefixedData = append(prefixedData, messageHash...)
	digest := crypto.Keccak256(prefixedData)

	s.logger.Debug("generated EIP-712 signing digest", "digest_hex", hexutil.Encode(digest))

	// 5. Sign the digest with the private key.
	// `crypto.Sign` produces a signature in the [R, S, V] format, where V is 0 or 1.
	signatureBytes, err := crypto.Sign(digest, privateKey)
	if err != nil {
		s.logger.Error("failed to sign the digest", "error", err)
		return "", errors.New("failed to sign the digest")
	}

	s.logger.Debug("raw signature from crypto.Sign", "length", len(signatureBytes), "signature_bytes", signatureBytes)

	// 6. Adjust the recovery ID (V).
	// For Ethereum transactions, V is typically 27 or 28.
	// The `crypto.Sign` function returns V as 0 or 1, so we add 27.
	// Important: The public key recovery is not performed here, but this is standard practice.
	if len(signatureBytes) == 65 {
		// signatureBytes[64] is V, which is 0 or 1.
		// Polymarket might expect V to be 27 or 28.
		signatureBytes[64] += 27
	} else {
		return "", errors.New("signature generated with incorrect length")
	}

	// 7. Return the signature as a hex-encoded string.
	signatureHex := hexutil.Encode(signatureBytes)
	s.logger.Info("successfully signed EIP-712 payload", "signature_hex", signatureHex)

	return signatureHex, nil
}

/**
 * @description
 * VerifySignature verifies an EIP-712 signature against a public key.
 *
 * @param publicKey The ECDSA public key to verify against.
 * @param payloadJSON The EIP-712 typed data payload that was signed.
 * @param signatureHex The signature to verify, as a hexadecimal string.
 * @returns true if the signature is valid, false otherwise.
 * @returns An error if verification fails due to parsing errors.
 *
 * @notes
 * This is a utility function for testing and verification purposes.
 */
func (s *Signer) VerifySignature(publicKey *ecdsa.PublicKey, payloadJSON string, signatureHex string) (bool, error) {
	// Unmarshal the JSON payload into the EIP-712 TypedData structure.
	var typedData apitypes.TypedData
	if err := json.Unmarshal([]byte(payloadJSON), &typedData); err != nil {
		s.logger.Error("failed to unmarshal EIP-712 payload JSON for verification", "error", err)
		return false, errors.New("invalid EIP-712 payload JSON")
	}

	// Get the EIP-712 domain and message hashes.
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		s.logger.Error("failed to hash EIP712 domain for verification", "error", err)
		return false, errors.New("failed to hash EIP712 domain")
	}
	messageHash, err := typedData.HashStruct(typedData.PrimaryType, typedData.Message)
	if err != nil {
		s.logger.Error("failed to hash EIP712 message for verification", "error", err)
		return false, errors.New("failed to hash EIP712 message")
	}

	// Construct the final digest.
	prefixedData := []byte{0x19, 0x01}
	prefixedData = append(prefixedData, domainSeparator...)
	prefixedData = append(prefixedData, messageHash...)
	digest := crypto.Keccak256(prefixedData)

	// Decode the signature.
	signatureBytes, err := hexutil.Decode(signatureHex)
	if err != nil {
		s.logger.Error("failed to decode signature hex", "error", err)
		return false, errors.New("invalid signature format")
	}

	// Adjust V back if needed (should be 27 or 28).
	if len(signatureBytes) == 65 && signatureBytes[64] >= 27 {
		signatureBytes[64] -= 27
	}

	// Recover the public key from the signature.
	recoveredPubKey, err := crypto.SigToPub(digest, signatureBytes)
	if err != nil {
		s.logger.Error("failed to recover public key from signature", "error", err)
		return false, errors.New("failed to recover public key")
	}

	// Compare the recovered public key with the provided one.
	// We compare by address (Ethereum address derived from public key) as it's more reliable.
	recoveredAddr := crypto.PubkeyToAddress(*recoveredPubKey)
	expectedAddr := crypto.PubkeyToAddress(*publicKey)
	return recoveredAddr == expectedAddr, nil
}

