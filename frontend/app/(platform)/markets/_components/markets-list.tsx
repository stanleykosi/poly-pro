/**
 * @description
 * Client component that fetches and displays all available markets from the backend API.
 * Each market is displayed as a clickable card that navigates to the market detail page.
 *
 * Key features:
 * - Client-Side Data Fetching: Fetches markets on the client side for real-time updates.
 * - Responsive Grid Layout: Displays markets in a responsive grid.
 * - Market Cards: Each market is displayed as a card with title, description, and metadata.
 * - Navigation: Clicking a market card navigates to the market detail page.
 *
 * @dependencies
 * - react: For useState, useEffect, and component logic.
 * - next/link: For client-side navigation.
 * - @/types: Contains the Market type definition.
 * - @/components/ui/card: For the card component.
 */

'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Market } from '@/types'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

interface MarketsApiResponse {
  status: 'success'
  data: Market[]
  meta: {
    count: number
    limit: number
    offset: number
  }
}

export default function MarketsList() {
  const [markets, setMarkets] = useState<Market[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const fetchMarkets = async () => {
      try {
        setIsLoading(true)
        setError(null)

        const apiUrl = process.env.NEXT_PUBLIC_API_URL
        if (!apiUrl) {
          throw new Error('API URL is not defined in environment variables.')
        }

        // Ensure the API URL doesn't already end with /api/v1
        const baseUrl = apiUrl.replace(/\/api\/v1\/?$/, '').replace(/\/$/, '')
        const endpoint = `${baseUrl}/api/v1/markets?limit=100`

        const response = await fetch(endpoint, {
          method: 'GET',
          headers: {
            'Content-Type': 'application/json',
          },
          cache: 'no-store', // Always fetch fresh data
        })

        if (!response.ok) {
          throw new Error(`Failed to fetch markets: ${response.statusText}`)
        }

        const data: MarketsApiResponse = await response.json()
        if (data.status === 'success' && data.data) {
          setMarkets(data.data)
        } else {
          throw new Error('Invalid response format from API')
        }
      } catch (err) {
        console.error('Error fetching markets:', err)
        setError(
          err instanceof Error ? err.message : 'Failed to load markets'
        )
      } finally {
        setIsLoading(false)
      }
    }

    fetchMarkets()
  }, [])

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Loading Markets...</CardTitle>
          <CardDescription>Fetching available markets from the API.</CardDescription>
        </CardHeader>
      </Card>
    )
  }

  if (error) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Error Loading Markets</CardTitle>
          <CardDescription>{error}</CardDescription>
        </CardHeader>
        <CardContent>
          <button
            onClick={() => window.location.reload()}
            className="text-primary hover:underline"
          >
            Try again
          </button>
        </CardContent>
      </Card>
    )
  }

  if (markets.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>No Markets Available</CardTitle>
          <CardDescription>
            There are currently no active markets available.
          </CardDescription>
        </CardHeader>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-secondary">
          Showing {markets.length} active market{markets.length !== 1 ? 's' : ''}
        </p>
      </div>
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        {markets.map((market) => {
          // Use slug if available, otherwise fall back to id
          const marketSlug = market.slug || market.id
          const marketUrl = `/markets/${marketSlug}`

          return (
            <Link key={market.id} href={marketUrl}>
              <Card className="h-full transition-all hover:shadow-lg hover:border-primary/50 cursor-pointer">
                <CardHeader>
                  <CardTitle className="line-clamp-2">{market.title}</CardTitle>
                  {market.category && (
                    <CardDescription className="capitalize">
                      {market.category}
                    </CardDescription>
                  )}
                </CardHeader>
                <CardContent>
                  <p className="text-sm text-secondary line-clamp-2 mb-4">
                    {market.description}
                  </p>
                  <div className="flex flex-wrap gap-2 text-xs text-secondary">
                    {market.liquidity && (
                      <span className="px-2 py-1 bg-muted rounded">
                        Liquidity: {parseFloat(market.liquidity).toLocaleString()}
                      </span>
                    )}
                    {market.end_date && (
                      <span className="px-2 py-1 bg-muted rounded">
                        Ends: {new Date(market.end_date).toLocaleDateString()}
                      </span>
                    )}
                  </div>
                </CardContent>
              </Card>
            </Link>
          )
        })}
      </div>
    </div>
  )
}

