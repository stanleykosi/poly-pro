/**
 * @description
 * This client component provides the user interface for creating and submitting trade orders.
 * It handles different order types (Limit/Market) and sides (Buy YES/Buy NO/Sell) and communicates
 * with the backend to place the trade.
 *
 * Key features:
 * - Order Types: Supports both Limit and Market orders
 * - Outcomes: Supports buying YES shares, buying NO shares, and selling shares
 * - Real-time Price: For market orders, uses the best available price from the order book
 * - Calculations: Properly calculates cost, payout, and profit based on Polymarket's pricing model
 * - Form Validation: Includes comprehensive validation for all inputs
 * - Loading & Error States: Provides visual feedback during form submission
 *
 * @dependencies
 * - react: For useState, useEffect, useMemo, and component logic
 * - @/hooks/use-api: The custom hook for the authenticated API client
 * - @/lib/stores/market-store: For accessing real-time order book data
 * - @/components/ui/*: Shadcn UI components
 */

'use client'

import { useState, useEffect, FormEvent, useMemo } from 'react'
import { useApi } from '@/hooks/use-api'
import { useMarketStore } from '@/lib/stores/market-store'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

interface PlaceOrderFormProps {
  marketId: string
  selectedPrice?: string
  yesTokenId?: string
  noTokenId?: string
}

type OrderType = 'LIMIT' | 'MARKET'
type OrderSide = 'BUY_YES' | 'BUY_NO' | 'SELL'

export default function PlaceOrderForm({
  marketId,
  selectedPrice,
  yesTokenId,
  noTokenId,
}: PlaceOrderFormProps) {
  const [orderType, setOrderType] = useState<OrderType>('LIMIT')
  const [side, setSide] = useState<OrderSide>('BUY_YES')
  const [price, setPrice] = useState('')
  const [size, setSize] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  const api = useApi()
  const orderBook = useMarketStore((s) => s.markets[marketId]?.orderBook)
  const [userPositions, setUserPositions] = useState<any[]>([])
  const [positionsLoading, setPositionsLoading] = useState(false)

  // Check if we're in sandbox/test environment
  const isSandbox = useMemo(() => {
    return process.env.NEXT_PUBLIC_SANDBOX_MODE === 'true' ||
           process.env.NEXT_PUBLIC_ENV === 'development' ||
           window.location.hostname.includes('localhost')
  }, [])

  // Fetch user positions for this market
  const fetchUserPositions = async () => {
    try {
      setPositionsLoading(true)
      const response = await api.get(`/api/v1/positions/${marketId}`)
      if (response.data.status === 'success') {
        setUserPositions(response.data.data || [])
      } else {
        setUserPositions([])
      }
    } catch (err: any) {
      console.error('Failed to fetch user positions:', err)
      setUserPositions([])
    } finally {
      setPositionsLoading(false)
    }
  }

  // Fetch positions when sell is selected
  useEffect(() => {
    if (side === 'SELL') {
      fetchUserPositions()
    }
  }, [side, marketId])

  // Get best available prices from order book for market orders
  const bestBid = useMemo(() => {
    if (!orderBook?.bids || orderBook.bids.length === 0) return null
    const validBids = orderBook.bids.filter((bid) => {
      const p = parseFloat(bid.price)
      return !isNaN(p) && p > 0 && p < 1 && bid.price !== '' && bid.size !== ''
    })
    if (validBids.length === 0) return null
    return parseFloat(validBids[0].price)
  }, [orderBook?.bids])

  const bestAsk = useMemo(() => {
    if (!orderBook?.asks || orderBook.asks.length === 0) return null
    const validAsks = orderBook.asks.filter((ask) => {
      const p = parseFloat(ask.price)
      return !isNaN(p) && p > 0 && p < 1 && ask.price !== '' && ask.size !== ''
    })
    if (validAsks.length === 0) return null
    return parseFloat(validAsks[0].price)
  }, [orderBook?.asks])

  // Debug logging for order book data
  useEffect(() => {
    if (orderBook) {
      console.log('[PlaceOrderForm] Order book data:', {
        marketId,
        hasBids: orderBook.bids?.length > 0,
        hasAsks: orderBook.asks?.length > 0,
        bidsCount: orderBook.bids?.length || 0,
        asksCount: orderBook.asks?.length || 0,
        bestBid: bestBid,
        bestAsk: bestAsk,
      })
    } else {
      console.log('[PlaceOrderForm] No order book data for market:', marketId)
    }
  }, [orderBook, marketId, bestBid, bestAsk])

  // Update price field when a price is selected from the order book (only for limit orders)
  useEffect(() => {
    if (selectedPrice && orderType === 'LIMIT') {
      setPrice(selectedPrice)
    }
  }, [selectedPrice, orderType])

  // Auto-fill market price when switching to market order or changing side
  useEffect(() => {
    if (orderType === 'MARKET') {
      if (side === 'BUY_YES' || side === 'BUY_NO') {
        // For buying, use the best ask (lowest price sellers are willing to accept)
        if (bestAsk !== null && !isNaN(bestAsk)) {
          setPrice(bestAsk.toFixed(4))
        } else {
          setPrice('')
        }
      } else if (side === 'SELL') {
        // For selling, use the best bid (highest price buyers are willing to pay)
        if (bestBid !== null && !isNaN(bestBid)) {
          setPrice(bestBid.toFixed(4))
        } else {
          setPrice('')
        }
      }
    }
  }, [orderType, side, bestAsk, bestBid])

  // Determine which token ID to use based on side
  // TODO: For selling, this should check user positions to determine which token they want to sell
  const tokenId = useMemo(() => {
    if (side === 'BUY_YES') {
      return yesTokenId
    } else if (side === 'BUY_NO') {
      return noTokenId
    } else if (side === 'SELL') {
      // For selling, default to YES token for now
      // TODO: Check user positions to determine which token they have
      return yesTokenId
    }
    return undefined
  }, [side, yesTokenId, noTokenId])

  // Calculate cost, payout, and profit based on Polymarket's pricing model
  const { cost, payout, profit, profitLabel, hasPositions } = useMemo(() => {
    const priceNum = parseFloat(price) || 0
    const sizeNum = parseFloat(size) || 0

    if (priceNum <= 0 || sizeNum <= 0 || isNaN(priceNum) || isNaN(sizeNum)) {
      return { cost: 0, payout: 0, profit: 0, profitLabel: 'Profit', hasPositions: false }
    }

    if (side === 'BUY_YES' || side === 'BUY_NO') {
      // For buying: cost = size * price, payout = size if outcome wins ($1 per share)
      const calculatedCost = sizeNum * priceNum
      const calculatedPayout = sizeNum // If the outcome wins, you get $1 per share
      const calculatedProfit = calculatedPayout - calculatedCost

      let label = 'Profit'
      if (side === 'BUY_YES') {
        label = 'Profit if YES wins'
      } else {
        label = 'Profit if NO wins'
      }

      return {
        cost: calculatedCost,
        payout: calculatedPayout,
        profit: calculatedProfit,
        profitLabel: label,
        hasPositions: false,
      }
    } else {
      // For selling: calculate based on user's actual positions
      if (userPositions.length === 0) {
        // No positions - show simplified calculation
        const receivedAmount = sizeNum * priceNum
        const potentialPayout = sizeNum
        return {
          cost: potentialPayout - receivedAmount,
          payout: receivedAmount,
          profit: receivedAmount - potentialPayout,
          profitLabel: 'Net (no positions found)',
          hasPositions: false,
        }
      }

      // Calculate profit/loss based on user's positions
      let totalCostBasis = 0
      let totalSharesToSell = sizeNum
      let actualProfit = 0

      // Sort positions by average price (sell most expensive first)
      const sortedPositions = [...userPositions].sort((a, b) => b.avg_price - a.avg_price)

      for (const position of sortedPositions) {
        if (totalSharesToSell <= 0) break

        const sharesFromThisPosition = Math.min(totalSharesToSell, parseFloat(position.total_size))
        const costBasisForTheseShares = sharesFromThisPosition * parseFloat(position.avg_price)

        totalCostBasis += costBasisForTheseShares
        totalSharesToSell -= sharesFromThisPosition

        // If this is a YES position, profit = sell_price - buy_price
        // If this is a NO position, profit = sell_price - (1 - buy_price)
        // because NO tokens cost (1 - YES_price) but payout $1 if NO wins
        if (position.side === 'BUY') {
          // This is the cost basis we paid, now selling at current price
          actualProfit += sharesFromThisPosition * (priceNum - parseFloat(position.avg_price))
        }
      }

      const receivedAmount = sizeNum * priceNum

      return {
        cost: totalCostBasis,
        payout: receivedAmount,
        profit: actualProfit,
        profitLabel: `Realized P&L (${userPositions.length} position${userPositions.length !== 1 ? 's' : ''})`,
        hasPositions: true,
      }
    }
  }, [price, size, side, userPositions])

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setIsLoading(true)
    setError(null)
    setSuccess(null)

    const priceNum = parseFloat(price)
    const sizeNum = parseFloat(size)

    // Validation
    if (orderType === 'LIMIT') {
      if (isNaN(priceNum) || priceNum <= 0 || priceNum >= 1) {
        setError('Price must be between 0 and 1 for limit orders.')
        setIsLoading(false)
        return
      }
    } else {
      // For market orders, price should be set automatically
      if (isNaN(priceNum) || priceNum <= 0 || priceNum >= 1) {
        setError('Unable to determine market price. Please wait for order book data.')
        setIsLoading(false)
        return
      }
    }

    if (isNaN(sizeNum) || sizeNum <= 0) {
      setError('Size must be greater than 0.')
      setIsLoading(false)
      return
    }

    if (!tokenId) {
      setError('Market token ID is not available.')
      setIsLoading(false)
      return
    }

    // Convert side to backend format: BUY or SELL
    const backendSide = side === 'SELL' ? 'SELL' : 'BUY'

    try {
      const response = await api.post('/api/v1/orders', {
        marketId: marketId,
        tokenId,
        price: priceNum,
        size: sizeNum,
        side: backendSide,
        orderType: orderType, // Include order type for future use
      })

      if (response.data.status === 'success') {
        setSuccess('Order placed successfully!')
        // Clear form on success
        setPrice('')
        setSize('')
        // Reset to limit order after successful submission
        setOrderType('LIMIT')
      } else {
        throw new Error(response.data.message || 'An unknown error occurred.')
      }
    } catch (err: any) {
      console.error('Failed to place order:', err)
      const errorMessage =
        err.response?.data?.message || err.message || 'Failed to place order.'
      setError(errorMessage)

      if (err.response?.data?.code === 'WALLET_NOT_FOUND') {
        setError('No wallet found. Please create a wallet to place orders.')
      }
    } finally {
      setIsLoading(false)
    }
  }

  // Format price as percentage (like Polymarket: 0.65 = 65¢)
  const priceDisplay = useMemo(() => {
    const priceNum = parseFloat(price)
    if (isNaN(priceNum) || priceNum === 0) return ''
    return (priceNum * 100).toFixed(2)
  }, [price])

  // Get side label for display
  const sideLabel = useMemo(() => {
    if (side === 'BUY_YES') return 'Buy YES'
    if (side === 'BUY_NO') return 'Buy NO'
    return 'Sell'
  }, [side])

  // Determine button variant based on side
  const buttonVariant = useMemo(() => {
    if (side === 'BUY_YES') return 'default' // Green
    if (side === 'BUY_NO') return 'destructive' // Red
    return 'secondary' // Orange for sell
  }, [side])

  // Market price display
  const marketPriceDisplay = useMemo(() => {
    if (orderType === 'MARKET') {
      if (side === 'BUY_YES' || side === 'BUY_NO') {
        if (bestAsk !== null) {
          return (bestAsk * 100).toFixed(2)
        }
        return null
      } else {
        if (bestBid !== null) {
          return (bestBid * 100).toFixed(2)
        }
        return null
      }
    }
    return null
  }, [orderType, side, bestAsk, bestBid])

  return (
    <div className="w-full">
      {/* Sandbox Environment Warning */}
      {isSandbox && (
        <div className="mb-4 rounded-lg border-2 border-yellow-200 bg-yellow-50 p-3">
          <div className="flex items-center">
            <div className="flex-shrink-0">
              <svg className="h-5 w-5 text-yellow-400" viewBox="0 0 20 20" fill="currentColor">
                <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
              </svg>
            </div>
            <div className="ml-3">
              <h3 className="text-sm font-medium text-yellow-800">
                Sandbox Environment
              </h3>
              <div className="mt-2 text-sm text-yellow-700">
                <p>This is a test environment. Orders will not execute real trades.</p>
              </div>
            </div>
          </div>
        </div>
      )}

      <form onSubmit={handleSubmit} className="w-full">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Order Type Selection */}
        <div className="space-y-2">
          <Label htmlFor="orderType" className="text-sm font-medium">
            Order Type
          </Label>
          <Select
            value={orderType}
            onValueChange={(value) => setOrderType(value as OrderType)}
          >
            <SelectTrigger id="orderType" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="LIMIT">Limit Order</SelectItem>
              <SelectItem value="MARKET">Market Order</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* Side Selection (Buy YES, Buy NO, Sell) */}
        <div className="space-y-2 md:col-span-2">
          <Label className="text-sm font-medium">Outcome</Label>
          <Tabs
            value={side}
            onValueChange={(value) => setSide(value as OrderSide)}
            className="w-full"
          >
            <TabsList className="grid w-full grid-cols-3">
              <TabsTrigger
                value="BUY_YES"
                className="data-[state=active]:bg-green-600 data-[state=active]:text-white hover:bg-green-700"
              >
                Buy YES
              </TabsTrigger>
              <TabsTrigger
                value="BUY_NO"
                className="data-[state=active]:bg-red-600 data-[state=active]:text-white hover:bg-red-700"
              >
                Buy NO
              </TabsTrigger>
              <TabsTrigger
                value="SELL"
                className="data-[state=active]:bg-orange-600 data-[state=active]:text-white hover:bg-orange-700"
              >
                Sell
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>

        {/* Price Input */}
        <div className="space-y-2">
          <Label htmlFor="price" className="text-sm font-medium">
            Price per share
          </Label>
          <div className="relative">
            <Input
              id="price"
              type="number"
              step="0.0001"
              min="0.0001"
              max="0.9999"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
              placeholder="0.0000"
              className="pr-12"
              disabled={orderType === 'MARKET'}
              required
            />
            <span className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
              {priceDisplay ? `${priceDisplay}¢` : '¢'}
            </span>
          </div>
          {orderType === 'MARKET' && (
            <p className="text-xs text-muted-foreground">
              {marketPriceDisplay
                ? `Market: ${marketPriceDisplay}¢`
                : 'Waiting for market data...'}
            </p>
          )}
        </div>

        {/* Size Input */}
        <div className="space-y-2">
          <Label htmlFor="size" className="text-sm font-medium">
            Shares
          </Label>
          <Input
            id="size"
            type="number"
            step="0.1"
            min="0.1"
            value={size}
            onChange={(e) => setSize(e.target.value)}
            placeholder="0.0"
            required
          />
        </div>
      </div>

      {/* Position Info for Sell Orders */}
      {side === 'SELL' && (
        <div className={`mt-4 rounded-lg border p-3 ${
          hasPositions
            ? 'border-blue-200 bg-blue-50'
            : positionsLoading
              ? 'border-gray-200 bg-gray-50'
              : 'border-orange-200 bg-orange-50'
        }`}>
          <div className="flex items-start">
            <div className="flex-shrink-0">
              {positionsLoading ? (
                <svg className="h-5 w-5 text-gray-400 animate-spin" viewBox="0 0 24 24" fill="none">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                </svg>
              ) : hasPositions ? (
                <svg className="h-5 w-5 text-blue-400" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                </svg>
              ) : (
                <svg className="h-5 w-5 text-orange-400" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M8.257 3.099c.765-1.36 2.722-1.36 3.486 0l5.58 9.92c.75 1.334-.213 2.98-1.742 2.98H4.42c-1.53 0-2.493-1.646-1.743-2.98l5.58-9.92zM11 13a1 1 0 11-2 0 1 1 0 012 0zm-1-8a1 1 0 00-1 1v3a1 1 0 002 0V6a1 1 0 00-1-1z" clipRule="evenodd" />
                </svg>
              )}
            </div>
            <div className="ml-3">
              <h3 className={`text-sm font-medium ${
                hasPositions ? 'text-blue-800' : positionsLoading ? 'text-gray-800' : 'text-orange-800'
              }`}>
                {positionsLoading ? 'Loading Positions...' : hasPositions ? 'Positions Found' : 'No Positions Found'}
              </h3>
              <div className={`mt-2 text-sm ${
                hasPositions ? 'text-blue-700' : positionsLoading ? 'text-gray-700' : 'text-orange-700'
              }`}>
                {positionsLoading ? (
                  <p>Checking your current positions in this market...</p>
                ) : hasPositions ? (
                  <div>
                    <p className="mb-2">Found {userPositions.length} position{userPositions.length !== 1 ? 's' : ''} in this market:</p>
                    <ul className="space-y-1">
                      {userPositions.map((pos, index) => (
                        <li key={index} className="text-xs">
                          {parseFloat(pos.total_size).toFixed(2)} shares at avg ${parseFloat(pos.avg_price).toFixed(4)}
                        </li>
                      ))}
                    </ul>
                  </div>
                ) : (
                  <p>No filled orders found for this market. You need to buy shares first before you can sell them.</p>
                )}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Cost and Payout Summary - Full width below inputs */}
      {(price && size && parseFloat(price) > 0 && parseFloat(size) > 0) && (
        <div className="mt-4 rounded-lg border border-border bg-muted/50 p-4">
          <div className="grid grid-cols-3 gap-4">
            <div className="text-center">
              <p className="text-xs text-muted-foreground mb-1">Cost</p>
              <p className="text-lg font-semibold">${cost.toFixed(2)}</p>
            </div>
            <div className="text-center border-x border-border">
              <p className="text-xs text-muted-foreground mb-1">
                {side === 'BUY_YES'
                  ? 'Payout if YES wins'
                  : side === 'BUY_NO'
                    ? 'Payout if NO wins'
                    : 'You receive'}
              </p>
              <p className="text-lg font-semibold">${payout.toFixed(2)}</p>
            </div>
            <div className="text-center">
              <p className="text-xs text-muted-foreground mb-1">
                {profitLabel}
              </p>
              <p
                className={`text-lg font-semibold ${
                  profit >= 0 ? 'text-green-600' : 'text-red-600'
                }`}
              >
                {profit >= 0 ? '+' : ''}${profit.toFixed(2)}
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Submit Button - Full width */}
      <div className="mt-4">
        <Button
          type="submit"
          className="w-full h-11 text-base font-semibold"
          variant={buttonVariant}
          disabled={
            isLoading ||
            !price ||
            !size ||
            (orderType === 'MARKET' && !bestAsk && !bestBid && (side === 'BUY_YES' || side === 'BUY_NO'))
          }
        >
          {isLoading
            ? 'Placing Order...'
            : orderType === 'MARKET'
              ? `${sideLabel} ${size || '0'} shares at market price`
              : `${sideLabel} ${size || '0'} shares at ${priceDisplay || '0'}¢`}
        </Button>
      </div>

      {/* Error Message */}
      {error && (
        <div className="mt-4 rounded-md bg-destructive/10 border border-destructive/20 p-3">
          <p className="text-sm text-destructive text-center">{error}</p>
        </div>
      )}

      {/* Success Message */}
      {success && (
        <div className="mt-4 rounded-md bg-constructive/10 border border-constructive/20 p-3">
          <p className="text-sm text-constructive text-center">{success}</p>
        </div>
      )}
      </form>
    </div>
  )
}
