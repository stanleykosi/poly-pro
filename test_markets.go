package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/poly-pro/backend/internal/polymarket"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	// Create Gamma API client
	gammaClient := polymarket.NewGammaAPIClient("https://gamma-api.polymarket.com", logger)

	ctx := context.Background()

	// Test fetching first 10 markets with volume sorting
	fmt.Println("=== Testing Gamma API Market Fetching ===")

	// Fetch first 10 markets
	markets, err := gammaClient.ListActiveMarkets(ctx, 10, 0)
	if err != nil {
		log.Fatalf("Failed to fetch markets: %v", err)
	}

	fmt.Printf("Fetched %d markets\n\n", len(markets))

	// Print details of each market
	for i, market := range markets {
		fmt.Printf("Market %d:\n", i+1)
		fmt.Printf("  ID: %s\n", market.ConditionID)
		fmt.Printf("  Question: %s\n", market.Question)
		fmt.Printf("  Volume: %s\n", market.Volume)
		if market.VolumeNum != nil {
			fmt.Printf("  VolumeNum: %.2f\n", *market.VolumeNum)
		} else {
			fmt.Printf("  VolumeNum: nil\n")
		}
		if market.Volume24hr != nil {
			fmt.Printf("  Volume24hr: %.2f\n", *market.Volume24hr)
		} else {
			fmt.Printf("  Volume24hr: nil\n")
		}
		fmt.Printf("  Category: %s\n", market.Category)
		fmt.Printf("  Slug: %s\n", market.Slug)
		fmt.Printf("\n")
	}

	// Now fetch top 100 and show the range
	fmt.Println("=== Testing Top 100 Markets ===")
	topMarkets, err := gammaClient.ListActiveMarkets(ctx, 100, 0)
	if err != nil {
		log.Fatalf("Failed to fetch top markets: %v", err)
	}

	fmt.Printf("Fetched %d markets for top 100 test\n", len(topMarkets))

	// Find min and max volumes
	var minVolume, maxVolume *float64
	var minMarket, maxMarket *polymarket.GammaMarket

	for i, market := range topMarkets {
		if market.VolumeNum != nil {
			vol := *market.VolumeNum
			if minVolume == nil || vol < *minVolume {
				minVolume = &vol
				minMarket = &topMarkets[i]
			}
			if maxVolume == nil || vol > *maxVolume {
				maxVolume = &vol
				maxMarket = &topMarkets[i]
			}
		}
	}

	if minVolume != nil && maxVolume != nil {
		fmt.Printf("Volume range in top 100:\n")
		fmt.Printf("  Min: %.2f (%s)\n", *minVolume, minMarket.Question)
		fmt.Printf("  Max: %.2f (%s)\n", *maxVolume, maxMarket.Question)
		fmt.Printf("\nFirst 5 markets by volume:\n")
		for i := 0; i < 5 && i < len(topMarkets); i++ {
			if topMarkets[i].VolumeNum != nil {
				fmt.Printf("  %d. %.2f - %s\n", i+1, *topMarkets[i].VolumeNum, topMarkets[i].Question)
			}
		}
	}
}
