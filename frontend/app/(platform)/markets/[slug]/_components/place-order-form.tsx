/**
 * @description
 * This client component provides the user interface for creating and submitting trade orders.
 * It handles different order types (Limit/Market) and sides (Buy/Sell) and communicates
 * with the backend to place the trade.
 *
 * Key features:
 * - Client-Side State: Manages form inputs like side, price, and size using React state.
 * - API Integration: Uses the `useApi` hook to get an authenticated Axios client for submitting orders.
 * - State Synchronization: Listens to the `selectedPrice` prop to update the price field when a user
 *   clicks on the order book.
 * - Form Validation: Includes basic validation for input fields.
 * - Loading & Error States: Provides visual feedback to the user during form submission.
 *
 * @dependencies
 * - react: For `useState`, `useEffect`, and component logic.
 * - @/hooks/use-api: The custom hook for the authenticated API client.
 * - @/components/ui/*: Shadcn UI components for building the form (Button, Input, etc.).
 *
 * @notes
 * - The component determines which `tokenId` to use based on the selected `side` and the
 *   `clobTokenIds` passed in from the parent. Polymarket uses one token ID for "YES" (BUY)
 *   and another for "NO" (SELL).
 */
'use client'

import { useState, useEffect, FormEvent, useMemo } from 'react'
import { useApi } from '@/hooks/use-api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

interface PlaceOrderFormProps {
  marketId: string
  selectedPrice?: string
  yesTokenId?: string
  noTokenId?: string
}

export default function PlaceOrderForm({
  marketId,
  selectedPrice,
  yesTokenId,
  noTokenId,
}: PlaceOrderFormProps) {
  const [side, setSide] = useState<'BUY' | 'SELL'>('BUY')
  const [price, setPrice] = useState('')
  const [size, setSize] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  const api = useApi()

  // Update the price field when a price is selected from the order book.
  useEffect(() => {
    if (selectedPrice) {
      setPrice(selectedPrice)
    }
  }, [selectedPrice])

  // Calculate cost and payout based on price and size
  const { cost, payout } = useMemo(() => {
    const priceNum = parseFloat(price) || 0
    const sizeNum = parseFloat(size) || 0
    
    if (side === 'BUY') {
      // For BUY: cost = size * price, payout = size if YES wins
      return {
        cost: priceNum * sizeNum,
        payout: sizeNum,
      }
    } else {
      // For SELL: cost = size * (1 - price), payout = size if NO wins
      return {
        cost: (1 - priceNum) * sizeNum,
        payout: sizeNum,
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

    if (isNaN(priceNum) || priceNum <= 0 || priceNum >= 1) {
      setError('Price must be between 0 and 1.')
      setIsLoading(false)
      return
    }

    if (isNaN(sizeNum) || sizeNum <= 0) {
      setError('Size must be greater than 0.')
      setIsLoading(false)
      return
    }

    const tokenId = side === 'BUY' ? yesTokenId : noTokenId
    if (!tokenId) {
      setError('Market token ID is not available.')
      setIsLoading(false)
      return
    }

    try {
      const response = await api.post('/api/v1/orders', {
        marketId: marketId,
        tokenId,
        price: priceNum,
        size: sizeNum,
        side,
      })

      if (response.data.status === 'success') {
        setSuccess('Order placed successfully!')
        // Clear form on success
        setPrice('')
        setSize('')
      } else {
        throw new Error(response.data.message || 'An unknown error occurred.')
      }
    } catch (err: any) {
      console.error('Failed to place order:', err)
      const errorMessage =
        err.response?.data?.message || err.message || 'Failed to place order.'
      setError(errorMessage)
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

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <Tabs
        value={side}
        onValueChange={(value) => setSide(value as 'BUY' | 'SELL')}
        className="w-full"
      >
        <TabsList className="grid w-full grid-cols-2">
          <TabsTrigger
            value="BUY"
            className="data-[state=active]:bg-constructive data-[state=active]:text-constructive-foreground"
          >
            Buy YES
          </TabsTrigger>
          <TabsTrigger
            value="SELL"
            className="data-[state=active]:bg-destructive data-[state=active]:text-destructive-foreground"
          >
            Sell YES
          </TabsTrigger>
        </TabsList>
      </Tabs>

      <div className="space-y-2">
        <Label htmlFor="price" className="text-sm font-medium">
          Price per share
        </Label>
        <div className="relative">
          <Input
            id="price"
            type="number"
            step="0.01"
            min="0.01"
            max="0.99"
            value={price}
            onChange={(e) => setPrice(e.target.value)}
            placeholder="0.00"
            className="pr-12"
            required
          />
          <span className="absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
            {priceDisplay ? `${priceDisplay}¢` : '¢'}
          </span>
        </div>
        <p className="text-xs text-muted-foreground">
          {priceDisplay
            ? `$${parseFloat(priceDisplay).toFixed(2)} per share`
            : 'Enter price between 1¢ and 99¢'}
        </p>
      </div>

      <div className="space-y-2">
        <Label htmlFor="size" className="text-sm font-medium">
          Shares
        </Label>
        <Input
          id="size"
          type="number"
          step="0.1"
          min="0"
          value={size}
          onChange={(e) => setSize(e.target.value)}
          placeholder="0.0"
          required
        />
      </div>

      {/* Cost and Payout Summary */}
      {(price && size && parseFloat(price) > 0 && parseFloat(size) > 0) && (
        <div className="rounded-lg border border-border bg-muted/50 p-3 space-y-2">
          <div className="flex justify-between text-sm">
            <span className="text-muted-foreground">Cost</span>
            <span className="font-medium">${cost.toFixed(2)}</span>
          </div>
          <div className="flex justify-between text-sm">
            <span className="text-muted-foreground">
              {side === 'BUY' ? 'Payout if YES' : 'Payout if NO'}
            </span>
            <span className="font-medium">${payout.toFixed(2)}</span>
          </div>
          <div className="flex justify-between text-sm pt-2 border-t border-border">
            <span className="text-muted-foreground">Profit</span>
            <span
              className={`font-medium ${
                payout - cost >= 0 ? 'text-constructive' : 'text-destructive'
              }`}
            >
              ${(payout - cost).toFixed(2)}
            </span>
          </div>
        </div>
      )}

      <Button
        type="submit"
        className="w-full h-11 text-base font-semibold"
        variant={side === 'BUY' ? 'constructive' : 'destructive'}
        disabled={isLoading || !price || !size}
      >
        {isLoading
          ? 'Placing Order...'
          : side === 'BUY'
            ? `Buy ${size || '0'} shares at ${priceDisplay || '0'}¢`
            : `Sell ${size || '0'} shares at ${priceDisplay || '0'}¢`}
      </Button>

      {error && (
        <div className="rounded-md bg-destructive/10 border border-destructive/20 p-3">
          <p className="text-sm text-destructive text-center">{error}</p>
        </div>
      )}
      {success && (
        <div className="rounded-md bg-constructive/10 border border-constructive/20 p-3">
          <p className="text-sm text-constructive text-center">{success}</p>
        </div>
      )}
    </form>
  )
}

