/**
 * @description
 * This file configures the middleware for the Next.js application, with a primary
 * focus on authentication using Clerk. The middleware intercepts incoming requests
 * to determine whether a route is public or requires authentication.
 *
 * Key features:
 * - Clerk Authentication: Integrates `clerkMiddleware` to protect routes by default.
 * - Public Routes: Explicitly defines an array of routes that should be accessible
 *   to unauthenticated users. This includes the home page, authentication pages,
 *   and API routes intended for webhooks.
 * - Route Protection: Any route not listed in `publicRoutes` will automatically
 *   require a user to be logged in. If they are not, they will be redirected
 *   to the sign-in page.
 *
 * @dependencies
 * - @clerk/nextjs: Provides the `clerkMiddleware` function.
 */
import { clerkMiddleware, createRouteMatcher } from '@clerk/nextjs/server'

// Define the routes that do not require authentication.
const isPublicRoute = createRouteMatcher([
  '/', // The landing page
  '/sign-in(.*)', // The sign-in page and all its sub-routes
  '/sign-up(.*)', // The sign-up page and all its sub-routes
  '/api/webhooks/clerk', // The Clerk webhook endpoint
])

export default clerkMiddleware((auth, req) => {
  // Protect all routes that are not public
  if (!isPublicRoute(req)) {
    auth().protect()
  }
})

export const config = {
  // The matcher configuration ensures that the middleware runs on all routes
  // except for internal Next.js assets and static files.
  matcher: [
    '/((?!.*\\..*|_next).*)', // Run on all routes that don't have a file extension or _next
    '/', // Also run on the root route
    '/(api|trpc)(.*)', // Run on all API routes
  ],
}

