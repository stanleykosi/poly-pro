/**
 * @description
 * Service for managing user wallets via the backend API.
 * Handles wallet creation, retrieval, and management operations.
 * 
 * Note: This service requires an authenticated API client. Use the `useApi` hook
 * in components to get an authenticated client, then pass it to these methods.
 */

import { AxiosResponse } from 'axios'

// API client interface matching what useApi returns
interface ApiClient {
  get: <T = any>(url: string, config?: any) => Promise<AxiosResponse<T>>
  post: <T = any>(url: string, data?: any, config?: any) => Promise<AxiosResponse<T>>
  put: <T = any>(url: string, data?: any, config?: any) => Promise<AxiosResponse<T>>
  delete: <T = any>(url: string, config?: any) => Promise<AxiosResponse<T>>
}

export interface Wallet {
  id: string
  polymarket_funder_address: string
  is_active: boolean
  created_at: string
}

export interface CreateWalletRequest {
  polymarket_funder_address?: string
  private_key?: string
}

export interface CreateWalletResponse {
  status: string
  message: string
  data: Wallet
}

export interface GetWalletsResponse {
  status: string
  data: Wallet[]
  meta: {
    count: number
  }
}

class WalletService {
  /**
   * Create a new wallet for the authenticated user
   */
  async createWallet(api: ApiClient, request?: CreateWalletRequest): Promise<Wallet> {
    const response = await api.post<CreateWalletResponse>('/api/v1/wallets', request || {})
    if (response.data.status === 'success') {
      return response.data.data
    }
    throw new Error(response.data.message || 'Failed to create wallet')
  }

  /**
   * Get all wallets for the authenticated user
   */
  async getWallets(api: ApiClient): Promise<Wallet[]> {
    // Only log in development to reduce console noise
    if (process.env.NODE_ENV === 'development') {
      console.log('[WalletService] Fetching wallets from /api/v1/wallets')
    }
    try {
      const response = await api.get<GetWalletsResponse>('/api/v1/wallets')
      if (process.env.NODE_ENV === 'development') {
        console.log('[WalletService] Wallet response received:', {
          status: response.status,
          dataStatus: response.data.status,
          walletCount: response.data.data?.length || 0,
          wallets: response.data.data,
        })
      }
      if (response.data.status === 'success') {
        return response.data.data
      }
      throw new Error('Failed to retrieve wallets')
    } catch (error: any) {
      // Only log errors in development, or if it's not a 401 (which is expected for unauthenticated users)
      if (process.env.NODE_ENV === 'development' || error.response?.status !== 401) {
        console.error('[WalletService] Error fetching wallets:', {
          message: error.message,
          response: error.response?.data,
          status: error.response?.status,
          url: error.config?.url,
          baseURL: error.config?.baseURL,
        })
      }
      throw error
    }
  }

  /**
   * Check if user has an active wallet
   */
  async hasActiveWallet(api: ApiClient): Promise<boolean> {
    try {
      const wallets = await this.getWallets(api)
      return wallets.some((wallet) => wallet.is_active)
    } catch (error) {
      return false
    }
  }
}

export const walletService = new WalletService()

