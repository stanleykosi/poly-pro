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

import { useEffect, useState, useMemo } from 'react'
import Link from 'next/link'
import { Market } from '@/types'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'

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

  // Group markets by category - MUST be called before any early returns to follow Rules of Hooks
  const { groupedMarkets, categories } = useMemo(() => {
    const grouped: Record<string, Market[]> = {}
    const cats: string[] = []

    markets.forEach((market) => {
      const category = market.category || 'Uncategorized'
      if (!grouped[category]) {
        grouped[category] = []
        cats.push(category)
      }
      grouped[category].push(market)
    })

    // Sort categories alphabetically, but put "Uncategorized" last
    cats.sort((a, b) => {
      if (a === 'Uncategorized') return 1
      if (b === 'Uncategorized') return -1
      return a.localeCompare(b)
    })

    return { groupedMarkets: grouped, categories: cats }
  }, [markets])

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

  const renderMarketCard = (market: Market) => {
    const marketSlug = market.slug || market.id
    const marketUrl = `/markets/${marketSlug}`

    return (
      <Link key={market.id} href={marketUrl}>
        <Card className="h-full transition-all hover:shadow-lg hover:border-primary/50 cursor-pointer group">
          <CardHeader className="pb-3">
            <CardTitle className="line-clamp-2 text-base group-hover:text-primary transition-colors">
              {market.title}
            </CardTitle>
            {market.category && (
              <CardDescription className="capitalize text-xs">
                {market.category}
              </CardDescription>
            )}
          </CardHeader>
          <CardContent className="pt-0">
            <p className="text-sm text-muted-foreground line-clamp-2 mb-4">
              {market.description}
            </p>
            <div className="flex flex-wrap gap-2 text-xs">
              {market.liquidity && (
                <span className="px-2.5 py-1 bg-muted rounded-md text-muted-foreground">
                  💰 ${parseFloat(market.liquidity).toLocaleString(undefined, {
                    minimumFractionDigits: 0,
                    maximumFractionDigits: 0,
                  })}
                </span>
              )}
              {market.end_date && (
                <span className="px-2.5 py-1 bg-muted rounded-md text-muted-foreground">
                  📅 {new Date(market.end_date).toLocaleDateString()}
                </span>
              )}
            </div>
          </CardContent>
        </Card>
      </Link>
    )
  }

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
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">Markets</h2>
          <p className="text-sm text-muted-foreground mt-1">
            {markets.length} active market{markets.length !== 1 ? 's' : ''}
          </p>
        </div>
      </div>

      {categories.length > 1 ? (
        <Tabs defaultValue={categories[0]} className="w-full">
          <TabsList className="grid w-full max-w-md grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 h-auto">
            {categories.slice(0, 5).map((category) => (
              <TabsTrigger
                key={category}
                value={category}
                className="text-xs capitalize"
              >
                {category}
              </TabsTrigger>
            ))}
            {categories.length > 5 && (
              <TabsTrigger value="all" className="text-xs">
                All
              </TabsTrigger>
            )}
          </TabsList>

          {categories.slice(0, 5).map((category) => (
            <TabsContent key={category} value={category} className="mt-4">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
                {groupedMarkets[category].map(renderMarketCard)}
              </div>
            </TabsContent>
          ))}
          <TabsContent value="all" className="mt-4">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
              {markets.map(renderMarketCard)}
            </div>
          </TabsContent>
        </Tabs>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
          {markets.map(renderMarketCard)}
        </div>
      )}
    </div>
  )
}

