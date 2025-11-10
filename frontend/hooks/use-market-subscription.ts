/**
 * @description
 * This file contains a custom React hook, `useMarketSubscription`, designed to simplify
 * the process of subscribing and unsubscribing to real-time market data streams within
 * React components. It encapsulates the lifecycle logic (connect, subscribe, unsubscribe)
 * required to interact with the singleton WebSocket service.
 *
 * Key features:
 * - Declarative API: Components can subscribe to a market simply by calling the hook.
 * - Automatic Lifecycle Management: Uses `useEffect` to automatically subscribe when a component
 *   mounts (or when the `marketId` changes) and unsubscribe when it unmounts.
 * - Abstraction: Hides the direct interaction with the `websocketService`, making components
 *   cleaner and more focused on their rendering logic.
 *
 * @dependencies
 * - react: For the `useEffect` hook.
 * - @/lib/services/websocket-service: The singleton service that manages the connection.
 *
 * @example
 * const MyComponent = ({ marketId }) => {
 *   // This single line handles connecting and subscribing.
 *   useMarketSubscription(marketId);
 *
 *   // The component can now access the data from the global store.
 *   const data = useMarketStore(state => state.markets[marketId]);
 *
 *   return <div>{...}</div>
 * }
 */
"use client"

import { useEffect } from 'react'
import websocketService from '@/lib/services/websocket-service'

// The WebSocket URL is sourced from environment variables, with a fallback for local development.
// If NEXT_PUBLIC_WS_URL is not set, derive it from NEXT_PUBLIC_API_URL
function getWebSocketURL(): string {
  if (process.env.NEXT_PUBLIC_WS_URL) {
    return process.env.NEXT_PUBLIC_WS_URL
  }
  
  // Derive WebSocket URL from API URL
  const apiUrl = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'
  // Replace http:// with ws:// and https:// with wss://
  const wsUrl = apiUrl.replace(/^http:/, 'ws:').replace(/^https:/, 'wss:')
  // Ensure it ends with /api/v1/ws
  return wsUrl.replace(/\/api\/v1\/?$/, '').replace(/\/$/, '') + '/api/v1/ws'
}

const WS_URL = getWebSocketURL()

/**
 * @function useMarketSubscription
 * @description A custom React hook to manage a subscription to a specific market's real-time data
 * via the WebSocket service. It handles connecting, subscribing, and unsubscribing
 * automatically based on the component's lifecycle.
 *
 * @param {string | null | undefined} marketId - The ID of the market to subscribe to.
 *        If null or undefined, the hook will take no action.
 */
export function useMarketSubscription(marketId: string | null | undefined) {
  useEffect(() => {
    // Only proceed if a valid marketId is provided. This allows components to
    // conditionally subscribe based on their props or state.
    if (!marketId) {
      return
    }

    // 1. Establish the WebSocket connection. The service is a singleton, so this will
    // either initiate a new connection or use the existing one without duplication.
    websocketService.connect(WS_URL)

    // 2. Subscribe to the specified market.
    websocketService.subscribe(marketId)

    // 3. Return a cleanup function. This function will be called by React when the
    // component unmounts or when the `marketId` dependency changes.
    return () => {
      websocketService.unsubscribe(marketId)
      // We do not call `disconnect()` here, as other components might still be
      // subscribed to other markets using the same shared connection.
    }
  }, [marketId]) // The effect re-runs if the marketId changes, handling subscription to a new market.
}

