"use client"

/**
 * @description
 * This component provides theme management for the application using the `next-themes` library.
 * It is a client component that wraps the `ThemeProvider` from `next-themes` to ensure
 * it works correctly within the Next.js App Router.
 *
 * Key features:
 * - Client-side Provider: Explicitly marked as a client component to handle theme state.
 * - Prop Delegation: Passes all props through to the underlying `ThemeProvider`,
 *   allowing for flexible configuration (e.g., setting the default theme, disabling
 *   the system theme preference).
 *
 * @dependencies
 * - next-themes: The underlying library for theme management.
 * - react: For component creation.
 */

import * as React from 'react'
import { ThemeProvider as NextThemesProvider } from 'next-themes'
import { type ThemeProviderProps } from 'next-themes/dist/types'

export function ThemeProvider({ children, ...props }: ThemeProviderProps) {
  return <NextThemesProvider {...props}>{children}</NextThemesProvider>
}

