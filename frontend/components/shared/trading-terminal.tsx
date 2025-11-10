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
 * - Component Composition: Assembles the layout with placeholder components for the chart,
 *   order book, and order form.
 * - State Management: Will coordinate state between its child components (e.g., clicking
 *   a price in the order book populates the order form).
 *
 * @dependencies
 * - react: For component logic.
 * - @/types: For the `Market` type.
 * - @/hooks/use-market-subscription: To subscribe to real-time data.
 * - @/components/ui/card: For structuring the layout.
 * - @/app/(platform)/markets/[slug]/_components/lightweight-chart: The charting component.
 */
'use client'

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

interface TradingTerminalProps {
  initialMarketData: Market
}

export default function TradingTerminal({
  initialMarketData,
}: TradingTerminalProps) {
  // Subscribe to real-time data for the current market. The hook handles the
  // connection and subscription lifecycle automatically.
  useMarketSubscription(initialMarketData.id)

  return (
    <div className="flex h-full flex-col gap-4">
      <header>
        <h1 className="text-2xl font-bold tracking-tight text-foreground">
          {initialMarketData.title}
        </h1>
        <p className="mt-1 text-secondary">{initialMarketData.description}</p>
      </header>
      <div className="grid h-full flex-1 grid-cols-1 gap-4 lg:grid-cols-4">
        {/* Main content area for the chart and other primary views */}
        <div className="lg:col-span-3">
          <Card className="h-full">
            <CardHeader>
              <CardTitle>Market Chart</CardTitle>
            </CardHeader>
            <CardContent>
              {/* Integrate the Lightweight Chart component */}
              <LightweightChart marketId={initialMarketData.id} />
            </CardContent>
          </Card>
        </div>

        {/* Sidebar area for order book and order entry */}
        <div className="flex flex-col gap-4">
          <Card>
            <CardHeader>
              <CardTitle>Order Book</CardTitle>
              <CardDescription>Live bids and asks</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex h-[200px] items-center justify-center text-secondary">
                {/* Order Book component will be integrated here in Step 17 */}
                <p>Order Book Placeholder</p>
              </div>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Place Order</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex h-[150px] items-center justify-center text-secondary">
                {/* Place Order Form component will be integrated here in Step 17 */}
                <p>Place Order Form Placeholder</p>
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}

