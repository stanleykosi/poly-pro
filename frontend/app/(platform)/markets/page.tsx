/**
 * @description
 * This server component serves as the main "Markets" page within the authenticated
 * section of the application. It displays a list of all available markets from Polymarket.
 *
 * Key features:
 * - Server Component: Rendered on the server, ensuring fast initial load times.
 * - Market Listing: Displays all active markets in a grid layout.
 * - Client Component Integration: Uses a client component for interactive market cards.
 *
 * @notes
 * - This page is automatically protected because it resides within the `(platform)`
 *   route group, which is covered by the authentication middleware.
 */

import MarketsList from './_components/markets-list'

export default function MarketsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-3xl font-bold tracking-tight text-foreground">
          Markets
        </h1>
        <p className="mt-2 text-secondary">
          Browse and trade on a wide variety of prediction markets.
        </p>
      </div>
      <MarketsList />
    </div>
  )
}
