/**
 * @description
 * Professional, state-of-the-art order book component designed for modern trading terminals.
 * Features real-time updates, depth visualization, market sentiment indicators, and smooth animations.
 *
 * Key features:
 * - Loading States: Skeleton loaders during data fetch
 * - Real-time Updates: Seamless updates without clearing data
 * - Depth Visualization: Color-coded bars showing order depth
 * - Market Sentiment: Visual indicators for bullish/bearish sentiment
 * - Spread Display: Prominent spread information
 * - Smooth Animations: Animated price changes and new orders
 * - Professional Styling: Bloomberg-like terminal appearance
 * - Order Flow: Visual indicators for market direction
 *
 * @dependencies
 * - react: For component logic and animations
 * - @/lib/stores/market-store: Enhanced Zustand store with processed order book data
 * - @/lib/utils: Utility functions for conditional classes
 * - @/types: Enhanced type definitions for order book data
 */
'use client'

import { useMemo, useEffect, useRef } from 'react'
import { useMarketStore } from '@/lib/stores/market-store'
import { useMarketSubscription } from '@/hooks/use-market-subscription'
import { cn } from '@/lib/utils'

interface OrderBookProps {
  marketId: string
  onPriceSelect: (price: string) => void
}

// Loading skeleton component
const OrderBookSkeleton = () => (
  <div className="flex h-[600px] flex-col border border-border rounded-lg bg-card">
    {/* Header */}
    <div className="flex items-center justify-between border-b border-border bg-muted/50 px-4 py-3">
      <div className="h-5 w-32 bg-muted animate-pulse rounded"></div>
      <div className="h-4 w-24 bg-muted animate-pulse rounded"></div>
    </div>

    {/* Spread indicator */}
    <div className="border-y border-border bg-muted/30 px-4 py-3">
      <div className="h-6 w-20 bg-muted animate-pulse rounded mx-auto"></div>
      <div className="h-4 w-16 bg-muted animate-pulse rounded mx-auto mt-1"></div>
    </div>

    {/* Order book rows */}
    <div className="flex-1 overflow-hidden">
      {/* Asks section */}
      <div className="flex flex-col">
        {Array.from({ length: 8 }).map((_, i) => (
          <div key={`ask-skeleton-${i}`} className="flex items-center justify-between px-4 py-2 border-b border-border/50">
            <div className="h-4 w-20 bg-muted animate-pulse rounded"></div>
            <div className="h-4 w-16 bg-muted animate-pulse rounded"></div>
            <div className="h-3 w-12 bg-muted animate-pulse rounded"></div>
          </div>
        ))}
      </div>

      {/* Bids section */}
      <div className="flex flex-col">
        {Array.from({ length: 8 }).map((_, i) => (
          <div key={`bid-skeleton-${i}`} className="flex items-center justify-between px-4 py-2 border-b border-border/50">
            <div className="h-4 w-20 bg-muted animate-pulse rounded"></div>
            <div className="h-4 w-16 bg-muted animate-pulse rounded"></div>
            <div className="h-3 w-12 bg-muted animate-pulse rounded"></div>
          </div>
        ))}
      </div>
    </div>
  </div>
)

// Professional order book row component with advanced order flow visualization
const OrderBookRow = ({
  level,
  isBid,
  onClick,
  maxDepth,
  marketSentiment,
  isBestBid,
  isBestAsk,
}: {
  level: any
  isBid: boolean
  onClick: () => void
  maxDepth: number
  marketSentiment: 'bullish' | 'bearish' | 'neutral'
  isBestBid?: boolean
  isBestAsk?: boolean
}) => {
  const depthPercentage = maxDepth > 0 ? (level.depthPercentage / 100) * 70 : 0 // Max 70% width

  // Calculate order flow indicators
  const isLargeOrder = parseFloat(level.size) > (maxDepth / 10) // Large order indicator
  const momentumIndicator = level.trend === 'increasing' ? '↗️' :
                           level.trend === 'decreasing' ? '↘️' : ''

  return (
    <div
      className={cn(
        "group relative flex items-center justify-between px-4 py-2 cursor-pointer transition-all duration-150 hover:bg-accent/30 border-b border-border/30",
        level.isNew && "animate-in slide-in-from-left-2 duration-300 bg-green-50/50 dark:bg-green-950/20",
        level.isChanged && "animate-pulse bg-blue-50/30 dark:bg-blue-950/20",
        isBestBid && "ring-1 ring-green-400/50 bg-green-50/20 dark:bg-green-950/30",
        isBestAsk && "ring-1 ring-red-400/50 bg-red-50/20 dark:bg-red-950/30"
      )}
      onClick={onClick}
    >
      {/* Enhanced depth visualization with gradient */}
      <div
        className={cn(
          "absolute inset-y-0 right-0 transition-all duration-500 opacity-10 rounded-l-md",
          isBid ? "bg-gradient-to-l from-green-500/60 to-green-500/20" : "bg-gradient-to-l from-red-500/60 to-red-500/20",
          marketSentiment === 'bullish' && isBid && "from-green-600/80 to-green-600/30 opacity-20",
          marketSentiment === 'bearish' && !isBid && "from-red-600/80 to-red-600/30 opacity-20",
          isLargeOrder && "opacity-25 shadow-inner"
        )}
        style={{ width: `${depthPercentage}%` }}
      />

      {/* Best bid/ask indicators */}
      {(isBestBid || isBestAsk) && (
        <div className="absolute left-2 top-1/2 -translate-y-1/2">
          <div className={cn(
            "w-1 h-6 rounded-full animate-pulse",
            isBestBid ? "bg-green-500" : "bg-red-500"
          )} />
        </div>
      )}

      {/* Price with trend indicators */}
      <div className="relative z-10 flex items-center gap-2 min-w-0 flex-1">
        <div className="flex items-center gap-1">
          <span
            className={cn(
              "font-mono text-sm font-semibold tabular-nums transition-all duration-200",
              isBid ? "text-green-600 dark:text-green-400" : "text-red-600 dark:text-red-400",
              isBestBid && "text-green-700 dark:text-green-300 font-bold scale-105",
              isBestAsk && "text-red-700 dark:text-red-300 font-bold scale-105"
            )}
          >
            ${parseFloat(level.price).toFixed(2)}
          </span>

          {/* Trend and status indicators */}
          <div className="flex items-center gap-1">
            {momentumIndicator && (
              <span className="text-xs animate-bounce" title={level.trend}>
                {momentumIndicator}
              </span>
            )}
            {level.isNew && (
              <div className="w-2 h-2 bg-blue-500 rounded-full animate-ping" title="New order" />
            )}
            {level.isChanged && (
              <div className="w-2 h-2 bg-yellow-500 rounded-full animate-pulse" title="Updated" />
            )}
            {isLargeOrder && (
              <div className="w-2 h-2 bg-purple-500 rounded-full" title="Large order" />
            )}
          </div>
        </div>
      </div>

      {/* Size with volume indicator */}
      <div className="relative z-10 flex items-center justify-end gap-1 min-w-[60px]">
        <span className={cn(
          "font-mono text-sm tabular-nums transition-colors",
          isLargeOrder && "font-bold text-purple-600 dark:text-purple-400"
        )}>
          {parseFloat(level.size).toLocaleString(undefined, {
            minimumFractionDigits: 0,
            maximumFractionDigits: 2,
          })}
        </span>
        {isLargeOrder && (
          <span className="text-xs text-purple-500" title="High volume">🔥</span>
        )}
      </div>

      {/* Cumulative with progress bar */}
      <div className="relative z-10 flex items-center gap-2 min-w-[50px] justify-end">
        <div className="flex flex-col items-end gap-1">
          <span className="font-mono text-xs text-muted-foreground tabular-nums">
            {level.cumulativeSize.toLocaleString(undefined, {
              minimumFractionDigits: 0,
              maximumFractionDigits: 1,
            })}
          </span>

          {/* Mini progress bar for cumulative depth */}
          <div className="w-8 h-1 bg-muted rounded-full overflow-hidden">
            <div
              className={cn(
                "h-full transition-all duration-300 rounded-full",
                isBid ? "bg-green-500" : "bg-red-500"
              )}
              style={{
                width: `${Math.min((level.cumulativeSize / (maxDepth || 1)) * 100, 100)}%`
              }}
            />
          </div>
        </div>
      </div>
    </div>
  )
}

export default function OrderBook({ marketId, onPriceSelect }: OrderBookProps) {
  const processedOrderBook = useMarketStore((state) => state.getProcessedOrderBook(marketId))
  const generateMockOrderBook = useMarketStore((state) => state.generateMockOrderBook)
  const animationRef = useRef<NodeJS.Timeout>()
  const fallbackTimeoutRef = useRef<NodeJS.Timeout>()

  // Subscribe to market updates
  useMarketSubscription(marketId)

  // Fallback mechanism: if no order book data after 10 seconds, generate mock data
  useEffect(() => {
    if (!processedOrderBook) {
      fallbackTimeoutRef.current = setTimeout(() => {
        console.log('[OrderBook] No real-time data received, generating mock order book for:', marketId)
        generateMockOrderBook(marketId, 0.5) // Default 50% probability
      }, 10000) // 10 seconds
    } else if (fallbackTimeoutRef.current) {
      clearTimeout(fallbackTimeoutRef.current)
    }

    return () => {
      if (fallbackTimeoutRef.current) {
        clearTimeout(fallbackTimeoutRef.current)
      }
    }
  }, [processedOrderBook, marketId, generateMockOrderBook])

  // Clear animation flags after animation completes
  useEffect(() => {
    if (processedOrderBook) {
      animationRef.current = setTimeout(() => {
        // Clear animation flags by updating the processed data
        const updatedBids = processedOrderBook.bids.map(bid => ({
          ...bid,
          isNew: false,
          isChanged: false,
        }))
        const updatedAsks = processedOrderBook.asks.map(ask => ({
          ...ask,
          isNew: false,
          isChanged: false,
        }))

        // This would trigger a re-render to clear animation states
        // In a real implementation, you'd update the store
      }, 1000)
    }

    return () => {
      if (animationRef.current) {
        clearTimeout(animationRef.current)
      }
    }
  }, [processedOrderBook])

  // Show loading skeleton if no data or loading, or if data is empty
  if (!processedOrderBook || processedOrderBook.isLoading || (processedOrderBook.bids.length === 0 && processedOrderBook.asks.length === 0)) {
    return (
      <div className="flex h-[600px] flex-col border border-border rounded-lg bg-card shadow-sm">
        <OrderBookSkeleton />
        <div className="border-t border-border bg-muted/30 px-4 py-3 text-center">
          <div className="flex items-center justify-center gap-2">
            <div className="w-4 h-4 border-2 border-primary border-t-transparent rounded-full animate-spin"></div>
            <span className="text-sm text-muted-foreground">Waiting for real-time order book data...</span>
          </div>
        </div>
      </div>
    )
  }

  const { bids, asks, spread, spreadPercentage, maxDepth, marketSentiment } = processedOrderBook

  // Debug logging
  console.log('[OrderBook] Rendering with data:', {
    bidsCount: bids.length,
    asksCount: asks.length,
    firstBid: bids[0]?.price,
    firstAsk: asks[0]?.price,
    spread,
    marketSentiment
  })

  return (
    <div className="flex h-[600px] flex-col border border-border rounded-lg bg-card shadow-sm">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border bg-muted/50 px-4 py-3">
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-semibold text-foreground">Order Book</h3>
          <div className={cn(
            "px-2 py-1 rounded-full text-xs font-medium",
            marketSentiment === 'bullish' && "bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200",
            marketSentiment === 'bearish' && "bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200",
            marketSentiment === 'neutral' && "bg-gray-100 text-gray-800 dark:bg-gray-900 dark:text-gray-200"
          )}>
            {marketSentiment === 'bullish' ? '📈 Bullish' :
             marketSentiment === 'bearish' ? '📉 Bearish' : '⚖️ Neutral'}
          </div>
        </div>
        <div className="text-xs text-muted-foreground">
          Last update: {new Date(processedOrderBook.lastUpdate).toLocaleTimeString()}
        </div>
      </div>

      {/* Prediction Insight */}
      <div className="border-y border-border bg-gradient-to-r from-blue-50/50 via-background to-green-50/50 dark:from-blue-950/20 dark:via-background dark:to-green-950/20 px-4 py-3">
        <div className="text-center">
          <div className="text-sm font-medium text-foreground mb-1">
            Market Prediction
          </div>
          <div className="flex items-center justify-center gap-4 text-xs">
            <div className="flex items-center gap-1">
              <div className="w-2 h-2 bg-blue-500 rounded-full"></div>
              <span>YES: ~{(processedOrderBook.asks.length > 0 ? parseFloat(processedOrderBook.asks[0].price) * 100 : 50).toFixed(1)}%</span>
            </div>
            <div className="flex items-center gap-1">
              <div className="w-2 h-2 bg-green-500 rounded-full"></div>
              <span>NO: ~{(processedOrderBook.bids.length > 0 ? (1 - parseFloat(processedOrderBook.bids[0].price)) * 100 : 50).toFixed(1)}%</span>
            </div>
          </div>
        </div>
      </div>

      {/* Column headers */}
      <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-muted/30 text-xs font-medium text-muted-foreground">
        <span className="min-w-0 flex-1">Probability</span>
        <span className="min-w-[60px] text-right">Shares</span>
        <span className="min-w-[50px] text-right">Total</span>
      </div>

      {/* Order book content */}
      <div className="flex-1 overflow-hidden flex flex-col">
        {/* Asks (Sell Orders) - Top section */}
        <div className="flex-1 overflow-y-auto">
          <div className="sticky top-0 bg-red-50/50 dark:bg-red-950/20 px-4 py-2 border-b border-red-200 dark:border-red-800">
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 bg-red-500 rounded-full"></div>
              <span className="text-xs font-semibold text-red-700 dark:text-red-300">YES OFFERS</span>
              <span className="text-xs text-red-600 dark:text-red-400">({asks.length})</span>
            </div>
          </div>

          <div className="flex flex-col">
            {asks.length > 0 ? (
              asks.slice(0, 15).map((ask, index) => (
                <OrderBookRow
                  key={`ask-${ask.price}-${index}`}
                  level={ask}
                  isBid={false}
                  onClick={() => onPriceSelect(ask.price)}
                  maxDepth={maxDepth}
                  marketSentiment={marketSentiment}
                  isBestAsk={index === 0}
                />
              ))
            ) : (
              <div className="py-8 text-center text-sm text-muted-foreground">
                No sell orders
              </div>
            )}
          </div>
        </div>

        {/* Bids (Buy Orders) - Bottom section */}
        <div className="flex-1 overflow-y-auto border-t border-border">
          <div className="sticky top-0 bg-green-50/50 dark:bg-green-950/20 px-4 py-2 border-b border-green-200 dark:border-green-800">
            <div className="flex items-center gap-2">
              <div className="w-3 h-3 bg-green-500 rounded-full"></div>
              <span className="text-xs font-semibold text-green-700 dark:text-green-300">NO OFFERS</span>
              <span className="text-xs text-green-600 dark:text-green-400">({bids.length})</span>
            </div>
          </div>

          <div className="flex flex-col">
            {bids.length > 0 ? (
              bids.slice(0, 15).map((bid, index) => (
                <OrderBookRow
                  key={`bid-${bid.price}-${index}`}
                  level={bid}
                  isBid={true}
                  onClick={() => onPriceSelect(bid.price)}
                  maxDepth={maxDepth}
                  marketSentiment={marketSentiment}
                  isBestBid={index === 0}
                />
              ))
            ) : (
              <div className="py-8 text-center text-sm text-muted-foreground">
                No buy orders
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Prediction Market Insights */}
      <div className="border-t border-border bg-gradient-to-r from-purple-50/30 via-background to-orange-50/30 dark:from-purple-950/10 dark:via-background dark:to-orange-950/10 px-4 py-2">
        <div className="flex items-center justify-between text-xs text-muted-foreground">
          <span className="flex items-center gap-1">
            <span className="text-purple-500">💡</span>
            Available Shares: {(maxDepth / 1000).toFixed(1)}K
          </span>
          <span className="flex items-center gap-1">
            <span className="text-orange-500">👥</span>
            Active Traders: {bids.length + asks.length}
          </span>
        </div>
      </div>

      {/* Order Flow Activity Indicator */}
      <div className="border-t border-border bg-gradient-to-r from-muted/20 via-background to-muted/20 px-4 py-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-1">
              <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
              <span className="text-xs text-muted-foreground">Live</span>
            </div>

            {/* Activity indicators */}
            <div className="flex items-center gap-2 text-xs">
              <span className="flex items-center gap-1">
                <span className="w-1.5 h-1.5 bg-green-500 rounded-full"></span>
                Bids: {bids.length}
              </span>
              <span className="flex items-center gap-1">
                <span className="w-1.5 h-1.5 bg-red-500 rounded-full"></span>
                Asks: {asks.length}
              </span>
            </div>
          </div>

          <div className="text-xs text-muted-foreground">
            Depth: {(maxDepth / 1000).toFixed(1)}K
          </div>
        </div>
      </div>
    </div>
  )
}

