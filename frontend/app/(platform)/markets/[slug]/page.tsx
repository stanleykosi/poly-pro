/**
 * @description
 * This server component represents the main trading page for a specific market.
 * It is responsible for fetching the initial, static market data from the backend
 * and passing it down to the client-side <TradingTerminal /> component.
 *
 * Key features:
 * - Server-Side Data Fetching: Fetches market details on the server to ensure fast
 *   page loads and good SEO.
 * - Dynamic Routing: Uses the `slug` parameter from the URL to fetch data for the
 *   correct market.
 * - Client Component Hydration: Renders the interactive <TradingTerminal /> component,
 *   passing the server-fetched data as initial props.
 * - Error Handling: Includes basic error handling for cases where the market data
 *   cannot be fetched.
 *
 * @dependencies
 * - next/navigation: For the `notFound` function to render a 404 page.
 * - @/components/shared/trading-terminal: The main client component for the trading UI.
 * - @/types: Contains the `Market` type definition.
 */

import TradingTerminal from '@/components/shared/trading-terminal'
import { Market } from '@/types'
import { notFound } from 'next/navigation'

// Define the expected shape of the API response for a single market.
interface MarketApiResponse {
  status: 'success'
  data: Market
}

/**
 * @function getMarketData
 * @description Fetches initial market data from the backend API.
 * @param {string} slug - The market ID (slug) to fetch.
 * @returns {Promise<Market | null>} A promise that resolves to the market data or null if not found.
 */
async function getMarketData(slug: string): Promise<Market | null> {
  const apiUrl = process.env.NEXT_PUBLIC_API_URL
  if (!apiUrl) {
    console.error('API URL is not defined in environment variables.')
    return null
  }

  try {
    const res = await fetch(`${apiUrl}/api/v1/markets/${slug}`, {
      // Use a short cache lifetime for market data, as details could change.
      // Revalidating every 60 seconds is a reasonable starting point.
      next: { revalidate: 60 },
    })

    if (!res.ok) {
      if (res.status === 404) {
        return null // Explicitly return null for 404 Not Found
      }
      // Throw an error for other non-successful responses to be caught by the catch block.
      throw new Error(`Failed to fetch market data: ${res.statusText}`)
    }

    const data: MarketApiResponse = await res.json()
    return data.data
  } catch (error) {
    console.error('Error fetching market data:', error)
    return null
  }
}

/**
 * @page MarketPage
 * @description The Next.js page component for the individual market view.
 * @param {object} params - The route parameters, containing the market `slug`.
 */
export default async function MarketPage({
  params,
}: {
  params: { slug: string }
}) {
  const marketData = await getMarketData(params.slug)

  // If the market data could not be fetched (e.g., 404 from the API),
  // render the Next.js not-found page.
  if (!marketData) {
    notFound()
  }

  // Render the main trading terminal client component, passing the
  // server-fetched data as a prop.
  return <TradingTerminal initialMarketData={marketData} />
}

