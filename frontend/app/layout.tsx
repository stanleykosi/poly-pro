/**
 * @description
 * This is the root layout for the Poly-Pro Analytics application.
 * It wraps all pages and defines the primary HTML structure.
 *
 * Key features:
 * - Metadata: Sets the default title and description for the application.
 * - Global Structure: Provides the `<html>` and `<body>` tags that will contain all page content.
 * - Provider Hub: This is where global context providers (like Clerk for auth) will be added in subsequent steps.
 *
 * @dependencies
 * - next/font/google: For optimizing and loading the 'Inter' font.
 * - ./globals.css: Imports the global stylesheet which includes Tailwind CSS.
 */
import type { Metadata } from 'next'
import { Inter } from 'next/font/google'
import './globals.css'

const inter = Inter({ subsets: ['latin'] })

export const metadata: Metadata = {
  title: 'Poly-Pro Analytics',
  description:
    'A professional-grade trading terminal and AI-powered analytical assistant for the Polymarket platform.',
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode
}>) {
  return (
    <html lang="en">
      <body className={inter.className}>{children}</body>
    </html>
  )
}

