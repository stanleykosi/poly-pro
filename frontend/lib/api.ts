/**
 * @description
 * This file configures and exports a centralized Axios instance for making API calls
 * to the Poly-Pro Analytics backend.
 *
 * Key features:
 * - Centralized Configuration: Provides a single point of configuration for the API client,
 *   including the base URL.
 * - Environment-based URL: The API's base URL is sourced from environment variables,
 *   allowing for easy switching between development, staging, and production environments.
 * - Type Safety: Exports the configured instance with proper TypeScript types for use
 *   throughout the application.
 *
 * @dependencies
 * - axios: The HTTP client for making requests.
 *
 * @notes
 * - This instance does not include an authentication interceptor by default. The JWT token
 *   is added dynamically by the `useApi` hook to ensure the latest token is used for
 *   every request, leveraging Clerk's session management capabilities.
 */
"use client"

import axios from 'axios'

// Get the backend API URL from environment variables.
// The NEXT_PUBLIC_ prefix is required for the variable to be exposed to the browser.
let baseURL = process.env.NEXT_PUBLIC_API_URL

// Ensure the API URL doesn't already end with /api/v1
// Remove trailing slashes and /api/v1 if present
if (baseURL) {
  baseURL = baseURL.replace(/\/api\/v1\/?$/, '').replace(/\/$/, '')
}

// Create a new Axios instance with the base URL configured.
// If baseURL is undefined, Axios will use relative URLs (acceptable for development).
const api = axios.create({
  baseURL,
})

export default api
