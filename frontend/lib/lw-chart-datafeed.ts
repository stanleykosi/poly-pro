/**
 * @description
 * This file implements the datafeed adapter for TradingView Lightweight Charts.
 * It acts as a bridge between the charting library and our application's data sources
 * (backend API for historical data and WebSocket service for real-time updates).
 *
 * Key features:
 * - Historical Data Fetching: Makes API calls to our backend's `/api/v1/markets/:id/history`
 *   endpoint to fetch historical OHLCV data.
 * - Real-time Data Subscription: Connects to our global Zustand store (`useMarketStore`),
 *   which is updated by the singleton WebSocket service. When the store's data for a market
 *   changes, this datafeed pushes the new bar to the chart.
 * - Data Format Conversion: Converts our backend's data format to Lightweight Charts' expected format.
 *
 * @dependencies
 * - @/lib/stores/market-store: The Zustand store for real-time data.
 * - lightweight-charts: The TradingView Lightweight Charts library.
 */

import { useMarketStore } from '@/lib/stores/market-store'
import { IChartApi, ISeriesApi, Time } from 'lightweight-charts'

// Define a type for bar data
export interface BarData {
  time: Time
  open: number
  high: number
  low: number
  close: number
  volume?: number
}

// A map to store active subscriptions for real-time updates
const subscriptions = new Map<
  string,
  {
    marketId: string
    series: ISeriesApi<'Candlestick' | 'Line' | 'Area' | 'Histogram'>
    lastBar: BarData | null
  }
>()

// Listen to the Zustand store for any changes
useMarketStore.subscribe((state, prevState) => {
  subscriptions.forEach((sub) => {
    const prevMarketData = prevState.markets[sub.marketId]
    const newMarketData = state.markets[sub.marketId]

    if (newMarketData && newMarketData !== prevMarketData) {
      console.log('[Chart Datafeed] Store update received for market:', sub.marketId, {
        has_bids: newMarketData.orderBook.bids?.length > 0,
        has_asks: newMarketData.orderBook.asks?.length > 0,
        has_lastBar: !!sub.lastBar,
      })

      // Update the last bar with new price from order book
      if (sub.lastBar) {
        // Get the latest price from the order book (best bid or ask)
        let newClose = sub.lastBar.close

        // Try to get price from bids first, then asks
        if (
          newMarketData.orderBook.bids &&
          newMarketData.orderBook.bids.length > 0
        ) {
          newClose = parseFloat(newMarketData.orderBook.bids[0].price)
        } else if (
          newMarketData.orderBook.asks &&
          newMarketData.orderBook.asks.length > 0
        ) {
          newClose = parseFloat(newMarketData.orderBook.asks[0].price)
        }

        // Skip if we couldn't get a valid price
        if (newClose === 0 || isNaN(newClose)) {
          console.warn('[Chart Datafeed] Invalid price extracted:', newClose, {
            bids: newMarketData.orderBook.bids,
            asks: newMarketData.orderBook.asks,
          })
          return
        }

        // Check if this is the first update (bar is empty/initialized with zeros)
        const isInitialBar = sub.lastBar.open === 0 && sub.lastBar.close === 0 && sub.lastBar.high === 0 && sub.lastBar.low === 0

        let updatedBar: BarData

        if (isInitialBar) {
          // First real-time update - create a proper bar from the current price
          const currentTime = Math.floor(Date.now() / 1000) as Time
          updatedBar = {
            time: currentTime,
            open: newClose,
            high: newClose,
            low: newClose,
            close: newClose,
            volume: 0,
          }
          console.log('[Chart Datafeed] Creating initial bar:', updatedBar)
          // Add the bar to the series (first bar)
          if ('setData' in sub.series) {
            sub.series.setData([updatedBar])
          }
        } else {
          // Update existing bar with new close price
          updatedBar = {
            ...sub.lastBar,
            close: newClose,
            // Update high/low if the new price exceeds them
            high: Math.max(sub.lastBar.high, newClose),
            low: Math.min(sub.lastBar.low, newClose),
          }
          console.log('[Chart Datafeed] Updating bar:', updatedBar)
          // Update the chart series
          if ('update' in sub.series) {
            sub.series.update(updatedBar)
          }
        }

        sub.lastBar = updatedBar
      } else {
        console.warn('[Chart Datafeed] No lastBar available for market:', sub.marketId)
      }
    }
  })
})

/**
 * @function fetchHistoricalData
 * @description Fetches historical OHLCV data from the backend API.
 * @param {string} marketId - The market ID to fetch data for.
 * @param {number} from - Unix timestamp in seconds for the start time.
 * @param {number} to - Unix timestamp in seconds for the end time.
 * @param {string} resolution - The resolution/interval (e.g., '1', '5', '15', '60', 'D').
 * @returns {Promise<BarData[]>} Array of bar data points.
 */
export async function fetchHistoricalData(
  marketId: string,
  from: number,
  to: number,
  resolution: string
): Promise<BarData[]> {
  let apiUrl = process.env.NEXT_PUBLIC_API_URL || ''
  if (apiUrl) {
    apiUrl = apiUrl.replace(/\/api\/v1\/?$/, '').replace(/\/$/, '')
  }
  
  if (!apiUrl) {
    const error = new Error('NEXT_PUBLIC_API_URL is not set')
    console.error('fetchHistoricalData error:', error)
    throw error
  }
  
  const endpoint = `${apiUrl}/api/v1/markets/${marketId}/history?from=${from}&to=${to}&resolution=${resolution}`

  try {
    const response = await fetch(endpoint, {
      method: 'GET',
      headers: {
        'Content-Type': 'application/json',
      },
    })
    
    if (!response.ok) {
      const errorText = await response.text()
      const error = new Error(
        `Failed to fetch history: ${response.status} ${response.statusText}${errorText ? ` - ${errorText}` : ''}`
      )
      console.error('fetchHistoricalData error:', error, {
        endpoint,
        status: response.status,
        statusText: response.statusText,
      })
      throw error
    }

    const data = await response.json()

    // Handle "no_data" response
    if (data.s === 'no_data') {
      console.debug('No historical data available for market', { marketId, resolution })
      return []
    }

    if (data.s !== 'ok' || !data.t || data.t.length === 0) {
      console.debug('Empty or invalid historical data response', { marketId, resolution, data })
      return []
    }

    // Convert backend format to Lightweight Charts format
    // Backend returns timestamps in seconds, Lightweight Charts Time type expects seconds for historical data
    const bars: BarData[] = data.t.map((time: number, index: number) => ({
      time: time as Time, // Keep as seconds - Lightweight Charts expects Unix timestamp in seconds
      open: data.o[index],
      high: data.h[index],
      low: data.l[index],
      close: data.c[index],
      volume: data.v[index],
    }))

    console.debug('Fetched historical data', { marketId, resolution, barCount: bars.length })
    return bars
  } catch (error) {
    console.error('fetchHistoricalData error:', error, {
      endpoint,
      marketId,
      from,
      to,
      resolution,
    })
    throw error
  }
}

/**
 * @function subscribeToRealtimeUpdates
 * @description Subscribes a chart series to real-time updates for a market.
 * @param {string} marketId - The market ID to subscribe to.
 * @param {ISeriesApi} series - The chart series to update.
 * @param {BarData | null} lastBar - The last bar data point (for updating).
 * @returns {string} Subscription ID for later cleanup.
 */
export function subscribeToRealtimeUpdates(
  marketId: string,
  series: ISeriesApi<'Candlestick' | 'Line' | 'Area' | 'Histogram'>,
  lastBar: BarData | null
): string {
  // Check if there's already an active subscription for this market
  // In React Strict Mode, the series object might be a new instance, so we check by marketId only
  for (const [id, sub] of subscriptions.entries()) {
    if (sub.marketId === marketId) {
      // Update the existing subscription with new series and lastBar
      sub.series = series
      sub.lastBar = lastBar
      console.log('[Chart Datafeed] Updated existing subscription:', {
        subscriptionId: id,
        marketId,
        has_lastBar: !!lastBar,
      })
      return id
    }
  }
  
  // Create new subscription
  const subscriptionId = `${marketId}-${Date.now()}`
  subscriptions.set(subscriptionId, {
    marketId,
    series,
    lastBar,
  })
  console.log('[Chart Datafeed] Subscribed to real-time updates:', {
    subscriptionId,
    marketId,
    has_lastBar: !!lastBar,
    total_subscriptions: subscriptions.size,
  })
  return subscriptionId
}

/**
 * @function unsubscribeFromRealtimeUpdates
 * @description Unsubscribes a chart series from real-time updates.
 * @param {string} marketId - The market ID to unsubscribe from.
 */
export function unsubscribeFromRealtimeUpdates(marketId: string): void {
  subscriptions.forEach((sub, id) => {
    if (sub.marketId === marketId) {
      subscriptions.delete(id)
    }
  })
}

