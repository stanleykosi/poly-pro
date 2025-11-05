/**
 * @description
 * This server component serves as the main "Markets" page within the authenticated
 * section of the application. For the initial implementation, it acts as a placeholder
 * to demonstrate the protected layout and routing.
 *
 * Key features:
 * - Server Component: Rendered on the server, ensuring fast initial load times.
 * - Placeholder Content: Displays a simple heading to identify the page. This
 *   will be replaced with the actual market browsing interface in future steps.
 *
 * @notes
 * - This page is automatically protected because it resides within the `(platform)`
 *   route group, which is covered by the authentication middleware.
 */
export default function MarketsPage() {
  return (
    <div>
      <h1 className="text-3xl font-bold tracking-tight text-foreground">
        Markets
      </h1>
      <p className="mt-2 text-secondary">
        Browse and trade on a wide variety of prediction markets.
      </p>
      {/* The main Trading Terminal component will be added here in a future step. */}
    </div>
  )
}

