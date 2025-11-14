/**
 * @description
 * The main client component for the trading terminal interface. It orchestrates all
 * the interactive elements of the trading view, including the chart, order book,
 * and order entry form.
 *
 * Key features:
 * - Client-Side Interactivity: Marked as a client component to handle state and user interactions.
 * - Real-time Subscriptions: Uses the `useMarketSubscription` hook to connect to the
 *   WebSocket service and subscribe to live data for the current market.
 * - Component Composition: Assembles the layout with the chart, order book, and order form components.
 * - State Management: Manages shared state between child components, like passing the selected price
 *   from the order book to the order form.
 *
 * @dependencies
 * - react: For `useState` and component logic.
 * - @/types: For the `Market` type.
 * - @/hooks/use-market-subscription: To subscribe to real-time data.
 * - @/components/ui/card: For structuring the layout.
 * - @/app/(platform)/markets/[slug]/_components/*: The child components for the terminal.
 */
'use client'

import { useState } from 'react'
import { Market } from '@/types'
import { useMarketSubscription } from '@/hooks/use-market-subscription'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import LightweightChart from '@/app/(platform)/markets/[slug]/_components/lightweight-chart'
import OrderBook from '@/app/(platform)/markets/[slug]/_components/order-book'
import PlaceOrderForm from '@/app/(platform)/markets/[slug]/_components/place-order-form'

interface TradingTerminalProps {
  initialMarketData: Market
}

export default function TradingTerminal({
  initialMarketData,
}: TradingTerminalProps) {
  // Subscribe to real-time data for the current market.
  useMarketSubscription(initialMarketData.id)

  // State to link the order book to the order form.
  const [selectedPrice, setSelectedPrice] = useState<string | undefined>()

  // Parse the clobTokenIds to get the specific token IDs for YES and NO outcomes.
  // Polymarket's `clobTokenIds` is a JSON string array: `["NO_TOKEN_ID", "YES_TOKEN_ID"]`.
  let yesTokenId: string | undefined
  let noTokenId: string | undefined
  try {
    if (initialMarketData.clobTokenIds) {
      const [noId, yesId] = JSON.parse(initialMarketData.clobTokenIds)
      noTokenId = noId
      yesTokenId = yesId
    }
  } catch (error) {
    console.error('Failed to parse clobTokenIds:', error)
  }

  return (
    <div className="flex h-full flex-col gap-6">
      <header className="space-y-2">
        <h1 className="text-3xl font-bold tracking-tight text-foreground">
          {initialMarketData.title}
        </h1>
        {initialMarketData.description && (
          <p className="text-muted-foreground">{initialMarketData.description}</p>
        )}
        {initialMarketData.category && (
          <div className="flex items-center gap-2">
            <span className="text-xs px-2.5 py-1 bg-muted rounded-md text-muted-foreground capitalize">
              {initialMarketData.category}
            </span>
            {initialMarketData.liquidity && (
              <span className="text-xs px-2.5 py-1 bg-muted rounded-md text-muted-foreground">
                💰 ${parseFloat(initialMarketData.liquidity).toLocaleString(undefined, {
                  minimumFractionDigits: 0,
                  maximumFractionDigits: 0,
                })}
              </span>
            )}
          </div>
        )}
      </header>
      <div className="grid h-full flex-1 grid-cols-1 gap-6 lg:grid-cols-4">
        {/* Main content area for the chart */}
        <div className="lg:col-span-3 min-h-[600px]">
          <Card className="h-full flex flex-col">
            <CardHeader className="pb-3">
              <CardTitle className="text-lg">Price Chart</CardTitle>
              <CardDescription>Real-time market data</CardDescription>
            </CardHeader>
            <CardContent className="flex-1 min-h-0">
              <LightweightChart marketId={initialMarketData.id} />
            </CardContent>
          </Card>
        </div>

        {/* Sidebar area for order book and order entry */}
        <div className="flex flex-col gap-6">
          <Card className="flex flex-col min-h-0">
            <CardHeader className="pb-3">
              <CardTitle className="text-lg">Order Book</CardTitle>
              <CardDescription className="text-xs">Live bids and asks</CardDescription>
            </CardHeader>
            <CardContent className="flex-1 min-h-0 p-0">
              <OrderBook
                marketId={initialMarketData.id}
                onPriceSelect={setSelectedPrice}
              />
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-lg">Place Order</CardTitle>
            </CardHeader>
            <CardContent>
              <PlaceOrderForm
                marketId={initialMarketData.id}
                selectedPrice={selectedPrice}
                yesTokenId={yesTokenId}
                noTokenId={noTokenId}
              />
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}

