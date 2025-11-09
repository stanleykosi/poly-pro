/**
 * @description
 * This file contains all TypeScript type definitions related to Polymarket market data,
 * including order books and WebSocket message structures. Defining these types ensures
 * data consistency and type safety when interacting with the backend and managing state.
 *
 * Key features:
 * - OrderBookLevel: Defines the structure of a single price level in the order book.
 * - OrderBook: Represents the bids and asks for a market.
 * - MarketData: The structure for storing all relevant real-time data for a single market in our state.
 * - WebSocketBookMessage: Defines the exact shape of the 'book' message received from the backend WebSocket,
 *   which is crucial for correctly parsing incoming data.
 */

/**
 * @interface OrderBookLevel
 * @description Represents a single price level (either a bid or an ask) in the order book.
 * @property {string} price - The price level, as a string to maintain precision.
 * @property {string} size - The total volume of orders at this price level, as a string.
 */
export interface OrderBookLevel {
  price: string
  size: string
}

/**
 * @interface OrderBook
 * @description Represents the complete order book for a market, containing arrays of bids and asks.
 * @property {OrderBookLevel[]} bids - An array of the current buy orders, sorted from highest to lowest price.
 * @property {OrderBookLevel[]} asks - An array of the current sell orders, sorted from lowest to highest price.
 */
export interface OrderBook {
  bids: OrderBookLevel[]
  asks: OrderBookLevel[]
}

/**
 * @interface MarketData
 * @description The structure used to store all real-time data for a single market within the Zustand store.
 * @property {string} marketId - The unique identifier for the market (Condition ID).
 * @property {string} assetId - The unique identifier for the asset (Token ID).
 * @property {OrderBook} orderBook - The current state of the order book.
 * @property {number} lastUpdate - The timestamp of the last update received.
 */
export interface MarketData {
  marketId: string
  assetId: string
  orderBook: OrderBook
  lastUpdate: number
}

/**
 * @interface Market
 * @description Represents the static, non-real-time data for a single market.
 * This data is typically fetched once when the market page is loaded.
 */
export interface Market {
  id: string
  title: string
  description: string
  resolution_source: string
}

/**
 * @interface WebSocketBookMessage
 * @description Defines the shape of the `book` event message received from the backend WebSocket service.
 * This corresponds to the `MockOrderBookData` struct on the backend.
 */
export interface WebSocketBookMessage {
  event_type: 'book'
  asset_id: string
  market: string // This is the marketId (Condition ID)
  bids: OrderBookLevel[]
  asks: OrderBookLevel[]
  timestamp: string // Unix timestamp in milliseconds as a string
  hash: string
}

