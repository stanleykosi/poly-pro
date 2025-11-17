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
  const tokenId = useMemo(() => {
    if (side === 'BUY_YES' || (side === 'SELL' && yesTokenId)) {
      return yesTokenId
    } else if (side === 'BUY_NO') {
      return noTokenId
    }
    return undefined
  }, [side, yesTokenId, noTokenId])

  // Calculate cost, payout, and profit based on Polymarket's pricing model
  const { cost, payout, profit } = useMemo(() => {
    const priceNum = parseFloat(price) || 0
    const sizeNum = parseFloat(size) || 0

    if (priceNum <= 0 || sizeNum <= 0 || isNaN(priceNum) || isNaN(sizeNum)) {
      return { cost: 0, payout: 0, profit: 0 }
    }

    if (side === 'BUY_YES' || side === 'BUY_NO') {
      // For buying: cost = size * price, payout = size if outcome wins ($1 per share)
      const calculatedCost = sizeNum * priceNum
      const calculatedPayout = sizeNum // If the outcome wins, you get $1 per share
      return {
        cost: calculatedCost,
        payout: calculatedPayout,
        profit: calculatedPayout - calculatedCost,
      }
    } else {
      // For selling: you receive size * price
      const receivedAmount = sizeNum * priceNum
      const potentialPayout = sizeNum // If you held and outcome wins
      return {
        cost: potentialPayout - receivedAmount, // Opportunity cost
        payout: receivedAmount,
        profit: receivedAmount - potentialPayout,
      }
    }
  }, [price, size, side])

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
    if (side === 'BUY_YES' || side === 'BUY_NO') return 'constructive'
    return 'destructive'
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
                className="data-[state=active]:bg-constructive data-[state=active]:text-constructive-foreground"
              >
                Buy YES
              </TabsTrigger>
              <TabsTrigger
                value="BUY_NO"
                className="data-[state=active]:bg-constructive data-[state=active]:text-constructive-foreground"
              >
                Buy NO
              </TabsTrigger>
              <TabsTrigger
                value="SELL"
                className="data-[state=active]:bg-destructive data-[state=active]:text-destructive-foreground"
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
                {side === 'SELL' ? 'Net' : 'Profit'}
              </p>
              <p
                className={`text-lg font-semibold ${
                  profit >= 0 ? 'text-constructive' : 'text-destructive'
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
  )
}
