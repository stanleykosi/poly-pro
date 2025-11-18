package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func main() {
	// Fetch market data from Gamma API
	resp, err := http.Get("https://gamma-api.polymarket.com/markets?conditionId=0xf2d034812c1e871421eb6ca9afd30f2a03f0494fc910cc4cabc4515685b610cb")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading body: %v\n", err)
		return
	}

	var data []map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		fmt.Printf("Error parsing JSON: %v\n", err)
		return
	}

	if len(data) > 0 {
		market := data[0]
		fmt.Printf("Market: %s\n", market["question"])
		fmt.Printf("Active: %v\n", market["active"])
		fmt.Printf("Closed: %v\n", market["closed"])
		if market["tokens"] != nil {
			tokens := market["tokens"].([]interface{})
			for i, token := range tokens {
				tokenData := token.(map[string]interface{})
				fmt.Printf("Token %d: %s - Price: %s\n", i+1, tokenData["outcome"], tokenData["price"])
			}
		}
		fmt.Printf("Volume: %s\n", market["volume"])
		if market["volumeNum"] != nil {
			fmt.Printf("VolumeNum: %.2f\n", market["volumeNum"])
		}
	}
}
