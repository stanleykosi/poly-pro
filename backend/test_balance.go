package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/poly-pro/backend/internal/polymarket"
)

func main() {
	// Test the data client with the wallet address from the database
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))

	// Test with the actual wallet address from the database
	walletAddress := "0x3159380850935c89d9D39fEA8e14D9c1e9A2833A"

	// Create data client
	dataClient := polymarket.NewDataAPIClient("", logger)

	// Test balance fetching
	fmt.Printf("Testing balance fetch for wallet: %s\n", walletAddress)

	balance, err := dataClient.GetUserBalance(context.Background(), walletAddress)
	if err != nil {
		fmt.Printf("Error fetching balance: %v\n", err)
		return
	}

	fmt.Printf("Success! Balance: %s USDC\n", balance.USDC)
}
