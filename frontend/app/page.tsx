/**
 * @description
 * This component serves as the main entry page for the application.
 * For the initial setup, it's a placeholder to confirm the frontend is running
 * and to demonstrate that the new dark theme and Shadcn UI components are working correctly.
 *
 * @notes
 * - This will later be developed into the main landing page or will redirect
 *   to the trading terminal for logged-in users.
 */

"use client"

import { Button } from '@/components/ui/button'

export default function Home() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center p-24">
      <div className="flex flex-col items-center space-y-4">
        <h1 className="text-5xl font-bold tracking-tighter">
          Poly-Pro Analytics
        </h1>
        <p className="text-secondary">
          The professional-grade trading terminal for Polymarket.
        </p>
        <div className="flex space-x-2 pt-4">
          <Button>Get Started</Button>
          <Button variant="secondary">Learn More</Button>
        </div>
      </div>
    </main>
  )
}

