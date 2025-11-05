/**
 * @description
 * This component renders the main sidebar navigation for the authenticated application.
 * It provides users with persistent links to the key sections of the platform, such as
 * Markets and Portfolio.
 *
 * Key features:
 * - Client-side Component: Marked as `"use client"` to use React hooks like `usePathname` for
 *   active link highlighting.
 * - Navigation Links: Displays navigation items with icons and labels.
 * - Active State Highlighting: The current route is highlighted to provide visual feedback to the user.
 * - User Profile Management: Includes the `<UserButton />` from Clerk, allowing users to manage
 *   their account and sign out.
 *
 * @dependencies
 * - next/link: For client-side navigation.
 * - next/navigation: Provides the `usePathname` hook to determine the current URL.
 * - @clerk/nextjs: For the `<UserButton />` component.
 * - lucide-react: For icons.
 * - @/lib/utils: The `cn` utility for conditional class names.
 */
"use client"

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { UserButton } from '@clerk/nextjs'
import { LayoutDashboard, Wallet } from 'lucide-react'
import { cn } from '@/lib/utils'

// Define the structure for a navigation link
interface NavLink {
  href: string
  label: string
  icon: React.ElementType
}

const navLinks: NavLink[] = [
  { href: '/markets', label: 'Markets', icon: LayoutDashboard },
  { href: '/portfolio', label: 'Portfolio', icon: Wallet },
]

export default function Sidebar() {
  const pathname = usePathname()

  return (
    <aside className="flex w-64 flex-col border-r border-border bg-card p-4">
      <div className="mb-8 flex items-center space-x-2">
        {/* Placeholder for a logo */}
        <div className="h-8 w-8 rounded-lg bg-primary"></div>
        <h1 className="text-xl font-bold text-foreground">Poly-Pro</h1>
      </div>
      <nav className="flex flex-1 flex-col">
        <ul className="space-y-2">
          {navLinks.map((link) => {
            const isActive = pathname.startsWith(link.href)
            return (
              <li key={link.href}>
                <Link
                  href={link.href}
                  className={cn(
                    'flex items-center space-x-3 rounded-md px-3 py-2 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-primary/10 text-primary'
                      : 'text-secondary hover:bg-accent hover:text-foreground'
                  )}
                >
                  <link.icon className="h-5 w-5" />
                  <span>{link.label}</span>
                </Link>
              </li>
            )
          })}
        </ul>
      </nav>
      <div className="mt-auto">
        <div className="flex items-center space-x-3 p-2">
          <UserButton afterSignOutUrl="/" />
          <span className="text-sm font-medium text-foreground">My Account</span>
        </div>
      </div>
    </aside>
  )
}

