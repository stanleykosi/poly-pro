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
    console.log('[WebSocket] Subscribing to market:', marketId, 'Current subscriptions:', Array.from(this.subscriptions))
    this.subscriptions.add(marketId)
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.sendSubscriptionMessage([marketId], 'subscribe')
    } else {
      console.log('[WebSocket] WebSocket not open yet, subscription will be sent on connect. ReadyState:', this.ws?.readyState)
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
    // Use requestAnimationFrame to ensure WebSocket is fully ready before sending
    // This prevents the "Still in CONNECTING state" error
    requestAnimationFrame(() => {
      // Double-check readyState before sending
      if (this.ws?.readyState === WebSocket.OPEN && this.subscriptions.size > 0) {
        this.sendSubscriptionMessage(Array.from(this.subscriptions), 'subscribe')
      }
    })
  }

  private handleMessage = (event: MessageEvent) => {
    // Log that we received a message, even before parsing
    console.log('[WebSocket] Raw message received', {
      data_type: typeof event.data,
      data_length: event.data?.length || 0,
      data_preview: typeof event.data === 'string' ? event.data.substring(0, 200) : 'not a string',
    })

    try {
      // Handle case where multiple messages might be in one frame (separated by newlines)
      const dataStr = typeof event.data === 'string' ? event.data : String(event.data)
      const lines = dataStr.split('\n').filter(line => line.trim().length > 0)
      
      // Process each line as a separate message
      for (const line of lines) {
        try {
          const message = JSON.parse(line) as WebSocketBookMessage

          console.log('[WebSocket] Received message:', {
            event_type: message.event_type,
            market: message.market,
            asset_id: message.asset_id,
            has_bids: message.bids?.length > 0,
            has_asks: message.asks?.length > 0,
            bids_count: message.bids?.length || 0,
            asks_count: message.asks?.length || 0,
          })

          if (message.event_type === 'book' && message.market) {
            // Update the Zustand store with the new order book data.
            useMarketStore.getState().setOrderBook(message.market, message)
            console.log('[WebSocket] Updated store for market:', message.market)
          } else {
            console.warn('[WebSocket] Received unhandled message:', message)
          }
        } catch (lineError) {
          console.error('[WebSocket] Failed to parse message line:', lineError, 'Line:', line.substring(0, 100))
        }
      }
    } catch (error) {
      console.error('[WebSocket] Failed to process message:', error, 'Raw data:', event.data)
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
    // Ensure WebSocket is in OPEN state before sending
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      console.warn(`[WebSocket] Cannot send ${type} message: WebSocket is not open`, {
        readyState: this.ws?.readyState,
        states: {
          CONNECTING: WebSocket.CONNECTING,
          OPEN: WebSocket.OPEN,
          CLOSING: WebSocket.CLOSING,
          CLOSED: WebSocket.CLOSED,
        },
      })
      return
    }

    const message = {
      type,
      market_ids: marketIds,
    }
    try {
      this.ws.send(JSON.stringify(message))
      console.log(`[WebSocket] Sent ${type} for markets:`, marketIds)
    } catch (error) {
      console.error(`[WebSocket] Failed to send ${type} message:`, error)
    }
  }
}

// Export a singleton instance of the service.
const websocketService = WebSocketService.getInstance()
export default websocketService

