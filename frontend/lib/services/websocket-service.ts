/**
 * @description
 * This file implements a singleton WebSocket service to manage the real-time data connection
 * with the backend. Using a singleton pattern ensures that only one WebSocket connection is
 * established for the entire application, preventing resource waste. The service handles
 * the connection lifecycle, message parsing, and updating the central Zustand store.
 *
 * Key features:
 * - Singleton Pattern: Ensures a single instance of the service throughout the app's lifecycle.
 * - Connection Management: Handles connecting, disconnecting, and automatic reconnection on failure.
 * - State Integration: Directly updates the `useMarketStore` (Zustand store) upon receiving new data.
 * - Subscription API: Provides simple `subscribe` and `unsubscribe` methods that components
 *   can use without needing to know the underlying WebSocket implementation details.
 *
 * @dependencies
 * - @/lib/stores/market-store: The Zustand store for market data.
 * - @/types: TypeScript definitions for WebSocket messages.
 */
"use client"

import { useMarketStore } from '@/lib/stores/market-store'
import { WebSocketBookMessage } from '@/types'

/**
 * @class WebSocketService
 * @description Manages the application's WebSocket connection as a singleton.
 */
class WebSocketService {
  private static instance: WebSocketService
  private ws: WebSocket | null = null
  private connectionUrl: string | null = null
  private reconnectInterval = 5000 // Reconnect every 5 seconds
  private reconnectTimeout: ReturnType<typeof setTimeout> | null = null
  private subscriptions: Set<string> = new Set()

  // Private constructor to enforce the singleton pattern.
  private constructor() {}

  /**
   * @description Gets the singleton instance of the WebSocketService.
   * @returns {WebSocketService} The singleton instance.
   */
  public static getInstance(): WebSocketService {
    if (!WebSocketService.instance) {
      WebSocketService.instance = new WebSocketService()
    }
    return WebSocketService.instance
  }

  /**
   * @description Establishes a connection to the WebSocket server. If already connected, it does nothing.
   * @param {string} url - The WebSocket server URL.
   */
  public connect(url: string): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      return
    }

    // Avoid reconnecting if there's already a reconnect attempt scheduled
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout)
      this.reconnectTimeout = null
    }

    this.connectionUrl = url
    console.log('[WebSocket] Connecting to:', url)
    this.ws = new WebSocket(url)

    this.ws.onopen = this.handleOpen
    this.ws.onmessage = this.handleMessage
    this.ws.onclose = this.handleClose
    this.ws.onerror = this.handleError
  }

  /**
   * @description Closes the WebSocket connection and prevents reconnection.
   */
  public disconnect(): void {
    console.log('[WebSocket] Disconnecting...')
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout)
      this.reconnectTimeout = null
    }
    if (this.ws) {
      this.ws.onclose = null // Prevent handleClose from triggering reconnection
      this.ws.close()
      this.ws = null
    }
  }

  /**
   * @description Subscribes to real-time updates for a specific market.
   * @param {string} marketId - The ID of the market to subscribe to.
   */
  public subscribe(marketId: string): void {
    this.subscriptions.add(marketId)
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.sendSubscriptionMessage([marketId], 'subscribe')
    }
  }

  /**
   * @description Unsubscribes from real-time updates for a specific market.
   * @param {string} marketId - The ID of the market to unsubscribe from.
   */
  public unsubscribe(marketId: string): void {
    this.subscriptions.delete(marketId)
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.sendSubscriptionMessage([marketId], 'unsubscribe')
    }
  }

  // --- Private Event Handlers ---

  private handleOpen = () => {
    console.log('[WebSocket] Connection established.')
    // If there are any pending subscriptions, send them now.
    if (this.subscriptions.size > 0) {
      this.sendSubscriptionMessage(Array.from(this.subscriptions), 'subscribe')
    }
  }

  private handleMessage = (event: MessageEvent) => {
    try {
      const message = JSON.parse(event.data) as WebSocketBookMessage

      if (message.event_type === 'book' && message.market) {
        // Update the Zustand store with the new order book data.
        useMarketStore.getState().setOrderBook(message.market, message)
      } else {
        console.warn('[WebSocket] Received unhandled message:', message)
      }
    } catch (error) {
      console.error('[WebSocket] Failed to parse message:', error)
    }
  }

  private handleClose = () => {
    console.log('[WebSocket] Connection closed. Attempting to reconnect...')
    this.ws = null
    // Schedule reconnection only if not intentionally disconnected.
    this.reconnectTimeout = setTimeout(() => {
      if (this.connectionUrl) {
        this.connect(this.connectionUrl)
      }
    }, this.reconnectInterval)
  }

  private handleError = (event: Event) => {
    console.error('[WebSocket] Error:', event)
    // The 'onclose' event will usually be triggered after an error,
    // which will then handle the reconnection logic.
  }

  // --- Private Helper Methods ---

  private sendSubscriptionMessage(marketIds: string[], type: 'subscribe' | 'unsubscribe') {
    const message = {
      type,
      market_ids: marketIds,
    }
    this.ws?.send(JSON.stringify(message))
    console.log(`[WebSocket] Sent ${type} for markets:`, marketIds)
  }
}

// Export a singleton instance of the service.
const websocketService = WebSocketService.getInstance()
export default websocketService

