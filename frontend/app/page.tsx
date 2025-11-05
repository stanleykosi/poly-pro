/**
 * @description
 * This component serves as the main landing page for the application.
 * It provides a brief introduction to the platform and includes navigation
 * for users to sign in or sign up.
 *
 * Key features:
 * - Call to Action: Features "Sign In" and "Sign Up" buttons to guide users
 *   to the authentication flows.
 * - Simple Layout: A clean and focused presentation of the application's value proposition.
 *
 * @dependencies
 * - next/link: For client-side navigation to the auth pages.
 * - @/components/ui/button: The application's standard button component.
 */

"use client"

import { Button } from '@/components/ui/button'
import Link from 'next/link'

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
          <Button asChild>
            <Link href="/sign-in">Sign In</Link>
          </Button>
          <Button variant="secondary" asChild>
            <Link href="/sign-up">Sign Up</Link>
          </Button>
        </div>
      </div>
    </main>
  )
}

