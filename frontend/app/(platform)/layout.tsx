/**
 * @description
 * This server component defines the main layout for all authenticated routes
 * within the Poly-Pro Analytics application, grouped under the `(platform)` route group.
 * It establishes the primary visual structure, consisting of a persistent sidebar
 * for navigation and a main content area where the page content is rendered.
 *
 * Key features:
 * - Structural Layout: Uses a flexbox container to create a two-column layout
 *   with the sidebar on the left and the main content on the right.
 * - Authentication Enforcement: Being inside the `(platform)` group, which is protected
 *   by the `middleware.ts`, this layout will only be rendered for authenticated users.
 * - Component Composition: Renders the <Sidebar /> component and the `children` prop,
 *   which represents the content of the specific page being viewed.
 *
 * @dependencies
 * - @/app/(platform)/_components/sidebar: The sidebar navigation component.
 */
import Sidebar from '@/app/(platform)/_components/sidebar'

export default function PlatformLayout({
  children,
}: {
  children: React.ReactNode
}) {
  return (
    <div className="flex h-screen bg-background">
      {/* The main sidebar for navigation */}
      <Sidebar />
      {/* The main content area for the application pages */}
      <main className="flex-1 overflow-y-auto p-6">{children}</main>
    </div>
  )
}

