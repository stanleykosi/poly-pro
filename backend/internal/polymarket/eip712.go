/**
 * @description
 * This file defines the Go structs that represent the EIP-712 typed data structure
 * for Polymarket's Central Limit Order Book (CLOB) orders. These structures are
 * essential for creating the JSON payload that will be sent to the remote-signer service.
 *
 * Key features:
 * - EIP-712 Domain: Defines the `TypedDataDomain` for the Polymarket CLOB, including
 *   the name, version, chain ID, and verifying contract address.
 * - EIP-712 Types: Specifies the `Types` map, which describes the structure of the
 *   `Order` message, matching the on-chain contract's expectations.
 * - Order Struct: The `Order` struct represents the actual order payload with all its
 *   fields, which will be serialized into the `message` part of the TypedData.
 *
 * @dependencies
 * - github.com/ethereum/go-ethereum/signer/core/apitypes: Provides the base `TypedData` struct.
 *
 * @notes
 * - The `verifyingContract` address and `chainId` must match the target deployment
 *   of the Polymarket CLOB. These are hardcoded for Polygon Mainnet for the MVP.
 * - The structure of the `Order` type MUST exactly match the one expected by the
 *   Polymarket exchange contract to ensure signature validity. This structure was
 *   derived from the Polymarket API documentation and client libraries.
 */

package polymarket

import (
	"math/big"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// PolymarketEIP712Domain defines the EIP-712 domain separator for Polymarket CLOB orders.
var PolymarketEIP712Domain = apitypes.TypedDataDomain{
	Name:              "Polymarket CLOB",
	Version:           "1",
	ChainId:           137, // Polygon Mainnet
	VerifyingContract: "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E",
}

// PolymarketEIP712Types defines the EIP-712 message types for an Order.
var PolymarketEIP712Types = apitypes.Types{
	"EIP712Domain": {
		{Name: "name", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "chainId", Type: "uint256"},
		{Name: "verifyingContract", Type: "address"},
	},
	"Order": {
		{Name: "salt", Type: "uint256"},
		{Name: "maker", Type: "address"},
		{Name: "signer", Type: "address"},
		{Name: "taker", Type: "address"},
		{Name: "tokenId", Type: "uint256"},
		{Name: "makerAmount", Type: "uint256"},
		{Name: "takerAmount", Type: "uint256"},
		{Name: "expiration", Type: "uint256"},
		{Name: "nonce", Type: "uint256"},
		{Name: "feeRateBps", Type: "uint256"},
		{Name: "side", Type: "uint8"},
		{Name: "signatureType", Type: "uint8"},
	},
}

// Order represents the EIP-712 message for a Polymarket CLOB order.
// The fields are tagged with `json:"..."` to ensure correct serialization.
type Order struct {
	Salt          string `json:"salt"`
	Maker         string `json:"maker"`
	Signer        string `json:"signer"`
	Taker         string `json:"taker"`
	TokenId       string `json:"tokenId"`
	MakerAmount   string `json:"makerAmount"`
	TakerAmount   string `json:"takerAmount"`
	Expiration    string `json:"expiration"`
	Nonce         string `json:"nonce"`
	FeeRateBps    string `json:"feeRateBps"`
	Side          int    `json:"side"`
	SignatureType int    `json:"signatureType"`
}

// SignedOrder represents the final structure that is sent to the Polymarket API,
// including the EIP-712 signature.
type SignedOrder struct {
	Order
	Signature string `json:"signature"`
}

// ToMessage converts the Order struct into a apitypes.TypedDataMessage,
// which is a map[string]interface{}, required for the EIP-712 signing process.
func (o Order) ToMessage() apitypes.TypedDataMessage {
	return apitypes.TypedDataMessage{
		"salt":          o.Salt,
		"maker":         o.Maker,
		"signer":        o.Signer,
		"taker":         o.Taker,
		"tokenId":       o.TokenId,
		"makerAmount":   o.MakerAmount,
		"takerAmount":   o.TakerAmount,
		"expiration":    o.Expiration,
		"nonce":         o.Nonce,
		"feeRateBps":    o.FeeRateBps,
		"side":          new(big.Int).SetInt64(int64(o.Side)),
		"signatureType": new(big.Int).SetInt64(int64(o.SignatureType)),
	}
}

