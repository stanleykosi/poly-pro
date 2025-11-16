/**
 * @description
 * Service for managing user wallets via the backend API.
 * Handles wallet creation, retrieval, and management operations.
 */

import { api } from '../api'

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
  async createWallet(request?: CreateWalletRequest): Promise<Wallet> {
    const response = await api.post<CreateWalletResponse>('/api/v1/wallets', request || {})
    if (response.data.status === 'success') {
      return response.data.data
    }
    throw new Error(response.data.message || 'Failed to create wallet')
  }

  /**
   * Get all wallets for the authenticated user
   */
  async getWallets(): Promise<Wallet[]> {
    const response = await api.get<GetWalletsResponse>('/api/v1/wallets')
    if (response.data.status === 'success') {
      return response.data.data
    }
    throw new Error('Failed to retrieve wallets')
  }

  /**
   * Check if user has an active wallet
   */
  async hasActiveWallet(): Promise<boolean> {
    try {
      const wallets = await this.getWallets()
      return wallets.some((wallet) => wallet.is_active)
    } catch (error) {
      return false
    }
  }
}

export const walletService = new WalletService()

