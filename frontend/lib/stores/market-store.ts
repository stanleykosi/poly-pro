/**
 * @description
 * This file defines the Zustand store for managing real-time market data.
 * The store acts as a centralized, client-side cache for data streamed from the WebSocket,
 * such as order books. Components can subscribe to this store to get live updates
 * and re-render reactively when data changes.
 *
 * Key features:
 * - Centralized State: Provides a single source of truth for all market data.
 * - Reactive Updates: Components automatically re-render when their subscribed state slice changes.
 * - Decoupling: Decouples the WebSocket service (which writes to the store) from the UI components
 *   (which read from the store).
 *
 * @dependencies
 * - zustand: The state management library.
 * - @/types: Contains the type definitions for market data structures.
 */
"use client"

import { create } from 'zustand'
import { MarketData, WebSocketBookMessage, ProcessedOrderBook, OrderBookLevelWithDepth, OrderBookLevel, PolymarketOrderBook, TokenOrderBook } from '@/types'

/**
 * @interface MarketState
 * @description Defines the shape of the Zustand store's state.
 * @property {Record<string, MarketData>} markets - A map where keys are market IDs and values are the
 *   corresponding market data objects.
 * @property {(marketId: string, message: WebSocketBookMessage) => void} setOrderBook - An action to update
 *   the order book for a specific market based on a new WebSocket message.
 * @property {(marketId: string) => void} setOrderBookLoading - Set loading state for order book.
 * @property {(marketId: string) => ProcessedOrderBook | null} getProcessedOrderBook - Get processed order book data.
 * @property {(marketId: string) => void} generateMockOrderBook - Generate mock order book data when real-time data is unavailable.
 */
interface MarketState {
  markets: Record<string, MarketData>
  processedOrderBooks: Record<string, ProcessedOrderBook> // Legacy support
  polymarketOrderBooks: Record<string, PolymarketOrderBook>
  setOrderBook: (marketId: string, message: WebSocketBookMessage) => void
  setOrderBookLoading: (marketId: string, isLoading: boolean) => void
  getProcessedOrderBook: (marketId: string) => ProcessedOrderBook | null // Legacy support
  getPolymarketOrderBook: (marketId: string) => PolymarketOrderBook | null
  generateMockOrderBook: (marketId: string, lastPrice?: number) => void
}

/**
 * @function processOrderBook
 * @description Processes raw order book data into enhanced format with depth calculations.
 */
const processOrderBook = (
  bids: OrderBookLevel[],
  asks: OrderBookLevel[],
  previousProcessed?: ProcessedOrderBook
): ProcessedOrderBook => {
  // Filter out invalid entries
  const validBids = bids.filter(bid =>
    bid.price !== '' && bid.size !== '' &&
    !isNaN(parseFloat(bid.price)) && !isNaN(parseFloat(bid.size)) &&
    parseFloat(bid.price) > 0 && parseFloat(bid.size) > 0
  )

  const validAsks = asks.filter(ask =>
    ask.price !== '' && ask.size !== '' &&
    !isNaN(parseFloat(ask.price)) && !isNaN(parseFloat(ask.size)) &&
    parseFloat(ask.price) > 0 && parseFloat(ask.size) > 0
  )

  // Calculate cumulative sizes
  const bidsWithDepth: OrderBookLevelWithDepth[] = []
  const asksWithDepth: OrderBookLevelWithDepth[] = []

  let bidCumulative = 0
  for (const bid of validBids) {
    const size = parseFloat(bid.size)
    bidCumulative += size
    bidsWithDepth.push({
      ...bid,
      cumulativeSize: bidCumulative,
      depthPercentage: 0, // Will be calculated after max depth is known
    })
  }

  let askCumulative = 0
  for (const ask of validAsks) {
    const size = parseFloat(ask.size)
    askCumulative += size
    asksWithDepth.push({
      ...ask,
      cumulativeSize: askCumulative,
      depthPercentage: 0, // Will be calculated after max depth is known
    })
  }

  // Calculate max depth and percentages
  const maxDepth = Math.max(bidCumulative, askCumulative)

  bidsWithDepth.forEach(bid => {
    bid.depthPercentage = maxDepth > 0 ? (bid.cumulativeSize / maxDepth) * 100 : 0
  })

  asksWithDepth.forEach(ask => {
    ask.depthPercentage = maxDepth > 0 ? (ask.cumulativeSize / maxDepth) * 100 : 0
  })

  // Calculate spread
  const bestBid = validBids.length > 0 ? parseFloat(validBids[0].price) : 0
  const bestAsk = validAsks.length > 0 ? parseFloat(validAsks[0].price) : 0
  const spread = bestBid > 0 && bestAsk > 0 ? bestAsk - bestBid : 0
  const spreadPercentage = bestBid > 0 ? (spread / bestBid) * 100 : 0

  // Determine market sentiment
  let marketSentiment: 'bullish' | 'bearish' | 'neutral' = 'neutral'
  if (validBids.length > 0 && validAsks.length > 0) {
    const bidDepth = bidCumulative
    const askDepth = askCumulative
    const depthRatio = bidDepth / (bidDepth + askDepth)
    if (depthRatio > 0.6) marketSentiment = 'bullish'
    else if (depthRatio < 0.4) marketSentiment = 'bearish'
  }

  // Mark new/changed levels for animations
  if (previousProcessed) {
    const existingBidPrices = new Set(previousProcessed.bids.map(b => b.price))
    const existingAskPrices = new Set(previousProcessed.asks.map(a => a.price))

    bidsWithDepth.forEach(bid => {
      if (!existingBidPrices.has(bid.price)) {
        bid.isNew = true
      } else {
        const prevBid = previousProcessed.bids.find(b => b.price === bid.price)
        if (prevBid && parseFloat(prevBid.size) !== parseFloat(bid.size)) {
          bid.isChanged = true
        }
      }
    })

    asksWithDepth.forEach(ask => {
      if (!existingAskPrices.has(ask.price)) {
        ask.isNew = true
      } else {
        const prevAsk = previousProcessed.asks.find(a => a.price === ask.price)
        if (prevAsk && parseFloat(prevAsk.size) !== parseFloat(ask.size)) {
          ask.isChanged = true
        }
      }
    })
  }

  return {
    bids: bidsWithDepth,
    asks: asksWithDepth,
    spread,
    spreadPercentage,
    maxDepth,
    isLoading: false,
    lastUpdate: new Date().toISOString(),
    marketSentiment,
  }
}

/**
 * @function calculateOverallSentiment
 * @description Calculates overall market sentiment based on YES/NO token volumes and spreads.
 */
const calculateOverallSentiment = (polymarketOrderBook: PolymarketOrderBook): 'bullish' | 'bearish' | 'neutral' => {
  const yesToken = polymarketOrderBook.yesToken
  const noToken = polymarketOrderBook.noToken

  if (!yesToken || !noToken) {
    return 'neutral'
  }

  // Calculate total volume for each token
  const yesVolume = yesToken.orderBook.bids.reduce((sum, bid) => sum + parseFloat(bid.size), 0) +
    yesToken.orderBook.asks.reduce((sum, ask) => sum + parseFloat(ask.size), 0)

  const noVolume = noToken.orderBook.bids.reduce((sum, bid) => sum + parseFloat(bid.size), 0) +
    noToken.orderBook.asks.reduce((sum, ask) => sum + parseFloat(ask.size), 0)

  // Volume ratio indicates market direction
  const volumeRatio = yesVolume / (yesVolume + noVolume)

  // Price spread indicates market efficiency/confidence
  const spread = polymarketOrderBook.marketSpread

  // High volume on YES + tight spread = bullish
  // High volume on NO + tight spread = bearish
  // Low volume or wide spread = neutral/uncertain

  if (spread < 0.1) { // Tight spread = high confidence
    if (volumeRatio > 0.6) return 'bullish'
    if (volumeRatio < 0.4) return 'bearish'
  }

  return 'neutral'
}

/**
 * @function useMarketStore
 * @description Creates and exports the Zustand store for market data.
 */
export const useMarketStore = create<MarketState>((set, get) => ({
  markets: {},
  processedOrderBooks: {}, // Legacy support
  polymarketOrderBooks: {},

  setOrderBook: (marketId, message) =>
    set((state) => {
      const tokenType = message.token_type || 'unknown'
      const tokenId = message.asset_id

      console.log('[MarketStore] Processing order book update:', {
        marketId,
        tokenType,
        tokenId,
        bidsCount: message.bids?.length || 0,
        asksCount: message.asks?.length || 0
      })

      // Get existing polymarket order book or create new one
      const existingPolymarketOrderBook = state.polymarketOrderBooks[marketId] || {
        marketId,
        yesToken: null,
        noToken: null,
        marketSpread: 0,
        overallSentiment: 'neutral' as const,
        isLoading: true,
        lastUpdate: new Date().toISOString()
      }

      // Process the order book for this token
      const processedOrderBook = processOrderBook(
        message.bids || [],
        message.asks || []
      )

      // Create token order book entry
      const tokenOrderBook: TokenOrderBook = {
        tokenId,
        tokenType: tokenType as 'yes' | 'no',
        orderBook: processedOrderBook
      }

      // Update the appropriate token in the polymarket order book
      let updatedPolymarketOrderBook = { ...existingPolymarketOrderBook }

      if (tokenType === 'yes') {
        updatedPolymarketOrderBook.yesToken = tokenOrderBook
      } else if (tokenType === 'no') {
        updatedPolymarketOrderBook.noToken = tokenOrderBook
      }

      // Calculate market spread (difference between YES and NO prices)
      const yesBestBid = updatedPolymarketOrderBook.yesToken?.orderBook.bids[0]?.price
      const noBestBid = updatedPolymarketOrderBook.noToken?.orderBook.bids[0]?.price

      if (yesBestBid && noBestBid) {
        const yesPrice = parseFloat(yesBestBid)
        const noPrice = parseFloat(noBestBid)
        updatedPolymarketOrderBook.marketSpread = Math.abs(yesPrice - noPrice)
      }

      // Calculate overall sentiment based on volume ratios
      updatedPolymarketOrderBook.overallSentiment = calculateOverallSentiment(updatedPolymarketOrderBook)
      updatedPolymarketOrderBook.isLoading = false
      updatedPolymarketOrderBook.lastUpdate = new Date().toISOString()

      // Legacy support: also update processedOrderBooks for backward compatibility
      // Combine YES and NO data for legacy components
      const combinedBids = [
        ...(updatedPolymarketOrderBook.yesToken?.orderBook.bids || []),
        ...(updatedPolymarketOrderBook.noToken?.orderBook.bids || [])
      ].sort((a, b) => parseFloat(b.price) - parseFloat(a.price))

      const combinedAsks = [
        ...(updatedPolymarketOrderBook.yesToken?.orderBook.asks || []),
        ...(updatedPolymarketOrderBook.noToken?.orderBook.asks || [])
      ].sort((a, b) => parseFloat(a.price) - parseFloat(b.price))

      const legacyProcessedOrderBook = processOrderBook(combinedBids, combinedAsks)

      console.log('[MarketStore] Updated polymarket order book:', {
        marketId,
        hasYesToken: !!updatedPolymarketOrderBook.yesToken,
        hasNoToken: !!updatedPolymarketOrderBook.noToken,
        marketSpread: updatedPolymarketOrderBook.marketSpread,
        overallSentiment: updatedPolymarketOrderBook.overallSentiment
      })

      return {
        ...state,
        polymarketOrderBooks: {
          ...state.polymarketOrderBooks,
          [marketId]: updatedPolymarketOrderBook,
        },
        // Legacy support
        processedOrderBooks: {
          ...state.processedOrderBooks,
          [marketId]: legacyProcessedOrderBook,
        },
      }
    }),

  setOrderBookLoading: (marketId, isLoading) =>
    set((state) => {
      const existingProcessed = state.processedOrderBooks[marketId]
      if (!existingProcessed) {
        return {
          processedOrderBooks: {
            ...state.processedOrderBooks,
            [marketId]: {
              bids: [],
              asks: [],
              spread: 0,
              spreadPercentage: 0,
              maxDepth: 0,
              isLoading,
              lastUpdate: new Date().toISOString(),
              marketSentiment: 'neutral' as const,
            },
          },
        }
      }

      return {
        processedOrderBooks: {
          ...state.processedOrderBooks,
          [marketId]: {
            ...existingProcessed,
            isLoading,
          },
        },
      }
    }),

  getProcessedOrderBook: (marketId) => {
    return get().processedOrderBooks[marketId] || null
  },

  generateMockOrderBook: (marketId, lastPrice = 0.5) => {
    set((state) => {
      // Generate mock order book data based on a reference price
      const generateOrders = (basePrice: number, count: number, isBid: boolean): OrderBookLevel[] => {
        const orders: OrderBookLevel[] = []
        for (let i = 0; i < count; i++) {
          const priceOffset = isBid ? -(i + 1) * 0.005 : (i + 1) * 0.005
          const price = Math.max(0.001, Math.min(0.999, basePrice + priceOffset))
          const size = Math.random() * 100 + 10 // Random size between 10-110
          orders.push({
            price: price.toFixed(4),
            size: size.toFixed(2)
          })
        }
        return orders
      }

      const mockBids = generateOrders(lastPrice, 8, true)
      const mockAsks = generateOrders(lastPrice, 8, false)

      const mockMessage: WebSocketBookMessage = {
        event_type: 'book',
        asset_id: 'mock-asset-id',
        token_type: 'unknown',
        market: marketId,
        bids: mockBids,
        asks: mockAsks,
        timestamp: Date.now().toString(),
        hash: 'mock-hash'
      }

      // Use existing logic to process and store the mock data
      const processedOrderBook = processOrderBook(mockBids, mockAsks)

      return {
        markets: {
          ...state.markets,
          [marketId]: {
            marketId,
            assetId: 'mock-asset-id',
            orderBook: {
              bids: mockBids,
              asks: mockAsks,
            },
            lastUpdate: Date.now(),
          },
        },
        processedOrderBooks: {
          ...state.processedOrderBooks,
          [marketId]: processedOrderBook,
        },
      }
    })
  },

  getPolymarketOrderBook: (marketId) => {
    const state = get()
    return state.polymarketOrderBooks[marketId] || null
  },
}))

