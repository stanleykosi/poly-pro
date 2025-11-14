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
import { MarketData, WebSocketBookMessage } from '@/types'

/**
 * @interface MarketState
 * @description Defines the shape of the Zustand store's state.
 * @property {Record<string, MarketData>} markets - A map where keys are market IDs and values are the
 *   corresponding market data objects.
 * @property {(marketId: string, message: WebSocketBookMessage) => void} setOrderBook - An action to update
 *   the order book for a specific market based on a new WebSocket message.
 */
interface MarketState {
  markets: Record<string, MarketData>
  setOrderBook: (marketId: string, message: WebSocketBookMessage) => void
}

/**
 * @function useMarketStore
 * @description Creates and exports the Zustand store for market data.
 *
 * @property {Record<string, MarketData>} markets - The initial state is an empty object. It will be populated
 *   with market data as WebSocket messages are received.
 * @property {Function} setOrderBook - The action that updates the store. It takes a market ID and a
 *   WebSocket message, then immutably updates the state by adding or replacing the data for that market.
 */
export const useMarketStore = create<MarketState>((set) => ({
  markets: {},
  setOrderBook: (marketId, message) =>
    set((state) => {
      // Check if the data actually changed to avoid unnecessary re-renders
      const existingMarket = state.markets[marketId]
      const newTimestamp = parseInt(message.timestamp, 10)
      
      // If market exists and timestamp hasn't changed, skip update
      if (
        existingMarket &&
        existingMarket.lastUpdate === newTimestamp &&
        existingMarket.assetId === message.asset_id
      ) {
        return state
      }

      return {
        markets: {
          ...state.markets,
          [marketId]: {
            marketId: message.market,
            assetId: message.asset_id,
            orderBook: {
              bids: message.bids,
              asks: message.asks,
            },
            // Parse the timestamp string to a number for easier use later.
            lastUpdate: newTimestamp,
          },
        },
      }
    }),
}))

