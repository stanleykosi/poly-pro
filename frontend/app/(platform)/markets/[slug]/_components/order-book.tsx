/**
 * @description
 * This client component displays the real-time Central Limit Order Book (CLOB) for a given market.
 * It subscribes to the global Zustand store to receive live updates pushed from the WebSocket service.
 *
 * Key features:
 * - Real-time Updates: Reacts to changes in the `useMarketStore` to display the latest bids and asks.
 * - Depth Visualization: Renders a visual indicator for the depth of orders at each price level.
 * - Interactivity: Allows users to click on a price level, which populates the order form via a callback.
 * - Performance: Optimized to handle frequent updates efficiently.
 *
 * @dependencies
 * - react: For component logic.
 * - @/lib/stores/market-store: The Zustand store for real-time market data.
 * - @/lib/utils: The `cn` utility for conditional class names.
 *
 * @notes
 * - The component calculates the maximum cumulative size to normalize the depth visualization bars.
 * - It separates bids and asks into two distinct, scrollable lists.
 */
'use client'

import { useMemo } from 'react'
import { useMarketStore } from '@/lib/stores/market-store'
import { cn } from '@/lib/utils'
import { OrderBookLevel } from '@/types'

interface OrderBookProps {
  marketId: string
  onPriceSelect: (price: string) => void
}

// Stable empty order book to avoid creating new objects on every render
const EMPTY_ORDER_BOOK = { bids: [], asks: [] } as const

export default function OrderBook({ marketId, onPriceSelect }: OrderBookProps) {
  // Use a selector that returns the orderBook directly, or undefined
  const orderBook = useMarketStore((s) => s.markets[marketId]?.orderBook)
  
  // Use useMemo to ensure stable references
  const { bids, asks } = useMemo(
    () => orderBook ?? EMPTY_ORDER_BOOK,
    [orderBook]
  )

  // Calculate cumulative sizes for depth visualization - memoized to prevent recalculation
  const { bidsWithCumulative, asksWithCumulative, maxCumulativeSize } = useMemo(() => {
    const bidsWithCum = bids.reduce((acc, bid) => {
      const lastSize = acc.length > 0 ? acc[acc.length - 1].cumulativeSize : 0
      const currentSize = parseFloat(bid.size)
      acc.push({ ...bid, cumulativeSize: lastSize + currentSize })
      return acc
    }, [] as (OrderBookLevel & { cumulativeSize: number })[])

    const asksWithCum = asks.reduce((acc, ask) => {
      const lastSize = acc.length > 0 ? acc[acc.length - 1].cumulativeSize : 0
      const currentSize = parseFloat(ask.size)
      acc.push({ ...ask, cumulativeSize: lastSize + currentSize })
      return acc
    }, [] as (OrderBookLevel & { cumulativeSize: number })[])

    const maxCumSize = Math.max(
      bidsWithCum[bidsWithCum.length - 1]?.cumulativeSize ?? 0,
      asksWithCum[asksWithCum.length - 1]?.cumulativeSize ?? 0
    )

    return {
      bidsWithCumulative: bidsWithCum,
      asksWithCumulative: asksWithCum,
      maxCumulativeSize: maxCumSize,
    }
  }, [bids, asks])

  const OrderBookRow = ({
    price,
    size,
    cumulativeSize,
    isBid,
    formatPrice,
  }: {
    price: string
    size: string
    cumulativeSize: number
    isBid: boolean
    formatPrice: (price: string) => string
  }) => {
    const depthPercentage =
      maxCumulativeSize > 0 ? (cumulativeSize / maxCumulativeSize) * 100 : 0

    return (
      <div
        className="relative flex cursor-pointer items-center justify-between px-3 py-1.5 text-xs transition-colors hover:bg-accent/50"
        onClick={() => onPriceSelect(price)}
      >
        <div
          className={cn(
            'absolute right-0 top-0 h-full opacity-20 transition-opacity',
            isBid ? 'bg-constructive' : 'bg-destructive'
          )}
          style={{ width: `${depthPercentage}%` }}
        />
        <span
          className={cn(
            'z-10 w-24 font-mono font-medium',
            isBid ? 'text-constructive' : 'text-destructive'
          )}
        >
          {formatPrice(price)}¢
        </span>
        <span className="z-10 flex-1 text-right font-mono text-foreground">
          {parseFloat(size).toLocaleString(undefined, {
            minimumFractionDigits: 0,
            maximumFractionDigits: 2,
          })}
        </span>
        <span className="z-10 w-20 text-right font-mono text-muted-foreground">
          {cumulativeSize.toLocaleString(undefined, {
            minimumFractionDigits: 0,
            maximumFractionDigits: 2,
          })}
        </span>
      </div>
    )
  }

  // Calculate spread
  const spread = useMemo(() => {
    if (bids.length > 0 && asks.length > 0) {
      const bidPrice = parseFloat(bids[0].price)
      const askPrice = parseFloat(asks[0].price)
      const spreadValue = askPrice - bidPrice
      const spreadPercent = ((spreadValue / bidPrice) * 100).toFixed(2)
      return { value: spreadValue.toFixed(4), percent: spreadPercent }
    }
    return { value: '0.0000', percent: '0.00' }
  }, [bids, asks])

  // Format price as cents (like Polymarket)
  const formatPrice = (priceStr: string) => {
    const price = parseFloat(priceStr)
    return (price * 100).toFixed(2)
  }

  return (
    <div className="flex h-[400px] flex-col">
      <div className="flex justify-between border-b border-border bg-muted/30 px-3 py-2 text-xs font-medium text-secondary">
        <span className="w-24">Price</span>
        <span className="flex-1 text-right">Size</span>
        <span className="w-20 text-right">Cum.</span>
      </div>
      <div className="flex-1 overflow-y-auto">
        {/* Asks (Sell Orders) - displayed at top, reversed */}
        <div className="flex flex-col-reverse">
          {asksWithCumulative.length > 0 ? (
            asksWithCumulative.slice(0, 10).map((ask) => (
              <OrderBookRow
                key={`ask-${ask.price}`}
                price={ask.price}
                size={ask.size}
                cumulativeSize={ask.cumulativeSize}
                isBid={false}
                formatPrice={formatPrice}
              />
            ))
          ) : (
            <div className="py-8 text-center text-xs text-muted-foreground">
              No asks available
            </div>
          )}
        </div>
        
        {/* Spread */}
        <div className="border-y-2 border-border bg-muted/50 py-2.5 text-center">
          <div className="text-sm font-semibold text-foreground">
            {spread.value}
          </div>
          <div className="text-xs text-muted-foreground">
            Spread ({spread.percent}%)
          </div>
        </div>
        
        {/* Bids (Buy Orders) */}
        <div>
          {bidsWithCumulative.length > 0 ? (
            bidsWithCumulative.slice(0, 10).map((bid) => (
              <OrderBookRow
                key={`bid-${bid.price}`}
                price={bid.price}
                size={bid.size}
                cumulativeSize={bid.cumulativeSize}
                isBid={true}
                formatPrice={formatPrice}
              />
            ))
          ) : (
            <div className="py-8 text-center text-xs text-muted-foreground">
              No bids available
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

