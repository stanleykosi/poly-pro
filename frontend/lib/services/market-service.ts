/**
 * Service for fetching official market prices from Polymarket's API
 */

import { ApiClient } from '@/hooks/use-api'

export interface OfficialMarketPrice {
  token_id: string
  best_bid: number
  best_ask: number
  spread: number
  market_price: number // Official Polymarket price (midpoint or last traded)
  last_traded_price?: number // Last traded price if available
  price_source: 'midpoint' | 'last_traded' // How the price was calculated
  last_updated: string
}

export class MarketService {
  constructor(private api: ApiClient) {}

  /**
   * Get official market prices directly from Polymarket's Gamma API
   * Uses the Token.price field which contains Polymarket's official prices
   */
  async getOfficialMarketPrices(marketId: string): Promise<OfficialMarketPrice[]> {
    console.log('[MarketService] Fetching official market prices for market:', marketId)

    const response = await this.api.get<{
      status: string
      data: OfficialMarketPrice[]
      meta: {
        market_id: string
        returned_prices: number
        source: string
      }
    }>(`/api/v1/markets/prices?market_id=${marketId}`)

    if (response.data.status !== 'success') {
      throw new Error('Failed to fetch official market prices')
    }

    console.log('[MarketService] Received official prices from Gamma API:', response.data.data)
    return response.data.data
  }

  /**
   * Get official price for a single token (deprecated - use getOfficialMarketPrices with marketId)
   */
  async getOfficialMarketPrice(tokenId: string): Promise<OfficialMarketPrice | null> {
    // This method is now deprecated since we fetch by market ID
    // For backward compatibility, we'll need to know the market ID
    // For now, return null
    console.warn('getOfficialMarketPrice is deprecated. Use getOfficialMarketPrices with marketId instead.')
    return null
  }
}

// Export a singleton instance
export const marketService = new MarketService({} as ApiClient)

// Helper function to inject API client
export const createMarketService = (api: ApiClient) => {
  return new MarketService(api)
}
