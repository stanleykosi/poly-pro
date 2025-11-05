/**
 * @description
 * This custom hook provides an authenticated API client for use within React components.
 * It abstracts the logic of retrieving the user's JWT from Clerk and attaching it to
 * every API request.
 *
 * Key features:
 * - Automatic Authentication: Retrieves the latest JWT from Clerk's `useAuth` hook
 *   before making any request, ensuring all API calls are authenticated.
 * - Memoization: Uses `useCallback` to memoize the returned API methods, preventing
 *   unnecessary re-renders in components that consume the hook.
 * - Simplified API Calls: Exposes familiar methods (`get`, `post`, `put`, `delete`)
 *   that mirror the Axios API, making it easy to use in components.
 * - Error Handling: The underlying Axios instance will handle standard HTTP errors,
 *   which can be caught with try/catch blocks in the component layer.
 *
 * @dependencies
 * - @clerk/nextjs: For the `useAuth` hook to get the session token.
 * - react: For `useCallback`.
 * - @/lib/api: The base, unauthenticated Axios instance.
 *
 * @example
 * const { get, post } = useApi();
 *
 * const fetchData = async () => {
 *   try {
 *     const response = await get('/markets');
 *     setData(response.data);
 *   } catch (error) {
 *     console.error("Failed to fetch markets", error);
 *   }
 * };
 *
 * const createItem = async (newItem) => {
 *   try {
 *     const response = await post('/items', newItem);
 *     // ... handle success
 *   } catch (error) {
 *     // ... handle error
 *   }
 * };
 */
"use client"

import { useAuth } from '@clerk/nextjs'
import { useCallback } from 'react'
import api from '@/lib/api'
import { AxiosRequestConfig, AxiosResponse } from 'axios'

// Define the structure of the API client returned by the hook.
interface ApiClient {
  get: <T = any>(
    url: string,
    config?: AxiosRequestConfig
  ) => Promise<AxiosResponse<T>>
  post: <T = any>(
    url: string,
    data?: any,
    config?: AxiosRequestConfig
  ) => Promise<AxiosResponse<T>>
  put: <T = any>(
    url: string,
    data?: any,
    config?: AxiosRequestConfig
  ) => Promise<AxiosResponse<T>>
  delete: <T = any>(
    url: string,
    config?: AxiosRequestConfig
  ) => Promise<AxiosResponse<T>>
}

export function useApi(): ApiClient {
  const { getToken } = useAuth()

  // Generic request function that adds the auth token.
  const request = useCallback(
    async <T>(config: AxiosRequestConfig): Promise<AxiosResponse<T>> => {
      const token = await getToken()
      if (!token) {
        // This case should ideally not be hit if the hook is used in a protected route,
        // but it's good practice for robustness.
        throw new Error('User is not authenticated.')
      }

      return api({
        ...config,
        headers: {
          ...(config.headers || {}),
          Authorization: `Bearer ${token}`,
        },
      })
    },
    [getToken]
  )

  // Memoized API methods that use the generic request function.
  const get = useCallback(
    <T = any>(url: string, config?: AxiosRequestConfig) => {
      return request<T>({ ...config, method: 'GET', url })
    },
    [request]
  )

  const post = useCallback(
    <T = any>(url: string, data?: any, config?: AxiosRequestConfig) => {
      return request<T>({ ...config, method: 'POST', url, data })
    },
    [request]
  )

  const put = useCallback(
    <T = any>(url: string, data?: any, config?: AxiosRequestConfig) => {
      return request<T>({ ...config, method: 'PUT', url, data })
    },
    [request]
  )

  const del = useCallback(
    <T = any>(url: string, config?: AxiosRequestConfig) => {
      return request<T>({ ...config, method: 'DELETE', url })
    },
    [request]
  )

  return { get, post, put, delete: del }
}
