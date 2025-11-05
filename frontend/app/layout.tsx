/**
 * @description
 * This is the root layout for the Poly-Pro Analytics application.
 * It wraps all pages and defines the primary HTML structure, including global providers.
 *
 * Key features:
 * - Metadata: Sets the default title and description for the application.
 * - Global Font: Applies the 'Inter' font across the entire application.
 * - Theme Provider: Wraps the application with the `ThemeProvider` to enable
 *   the dark theme by default, as specified in the design system.
 *
 * @dependencies
 * - next/font/google: For optimizing and loading the 'Inter' font.
 * - @/components/theme-provider: The client-side theme management component.
 * - ./globals.css: Imports the global stylesheet which includes Tailwind CSS.
 */
import type { Metadata } from 'next'
import { Inter } from 'next/font/google'
import './globals.css'
import { ThemeProvider } from '@/components/theme-provider'

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
    <html lang="en" suppressHydrationWarning>
      <body className={inter.className}>
        <ThemeProvider
          attribute="class"
          defaultTheme="dark"
          enableSystem={false}
          disableTransitionOnChange
        >
          {children}
        </ThemeProvider>
      </body>
    </html>
  )
}

