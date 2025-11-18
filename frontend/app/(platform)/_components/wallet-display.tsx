/**
 * @description
 * This component displays the user's wallet information in the sidebar.
 * It shows the wallet address and allows users to view wallet details.
 * 
 * Key features:
 * - Fetches wallet data on mount
 * - Displays wallet address in a truncated format
 * - Shows loading and error states
 * - Provides a link to create a wallet if none exists
 */

'use client'

import { useEffect, useState, useCallback, useRef } from 'react'
import { useApi } from '@/hooks/use-api'
import { walletService, Wallet } from '@/lib/services/wallet-service'
import { Wallet as WalletIcon, Copy, Check } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

export default function WalletDisplay() {
  const [wallet, setWallet] = useState<Wallet | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [balance, setBalance] = useState<string | null>(null)
  const [balanceLoading, setBalanceLoading] = useState(false)
  const [balanceError, setBalanceError] = useState(false)
  const api = useApi()
  const fetchingRef = useRef(false)
  const mountedRef = useRef(true)
  const apiRef = useRef(api)

  // Keep apiRef up to date
  useEffect(() => {
    apiRef.current = api
  }, [api])

  const fetchWallet = useCallback(async () => {
    // Prevent multiple simultaneous requests
    if (fetchingRef.current) {
      return
    }

    try {
      fetchingRef.current = true
      setLoading(true)
      setError(null)
      // Use apiRef to get the latest API instance
      const wallets = await walletService.getWallets(apiRef.current)
      
      // Only update state if component is still mounted
      if (!mountedRef.current) return
      
      // Handle empty array - no wallet exists
      if (!wallets || wallets.length === 0) {
        setWallet(null)
        setError(null) // Not an error, just no wallet
      } else {
        const activeWallet = wallets.find((w) => w.is_active) || wallets[0]
        setWallet(activeWallet || null)
        setError(null)

        // Fetch balance for the active wallet
        if (activeWallet) {
          console.log('[WalletDisplay] Found active wallet, fetching balance:', activeWallet.polymarket_funder_address)
          fetchBalance()
        } else {
          console.log('[WalletDisplay] No active wallet found')
        }
      }
    } catch (err: any) {
      console.error('Failed to fetch wallet:', err)
      
      // Only update state if component is still mounted
      if (!mountedRef.current) return
      
      // Handle 401 (unauthorized) - user not authenticated or token expired
      if (err.response?.status === 401) {
        setWallet(null)
        setError(null) // Don't show error for auth issues, just show "No wallet found"
      } else if (err.response?.status === 404 || err.response?.status === 200) {
        setWallet(null)
        setError(null)
      } else {
        setError('Failed to load wallet')
      }
    } finally {
      if (mountedRef.current) {
        setLoading(false)
      }
      fetchingRef.current = false
    }
  }, []) // No dependencies - uses apiRef

  useEffect(() => {
    mountedRef.current = true
    fetchWallet()

    // Listen for wallet creation events
    const handleWalletCreated = () => {
      if (mountedRef.current && !fetchingRef.current) {
        fetchWallet()
      }
    }

    window.addEventListener('wallet-created', handleWalletCreated)
    return () => {
      mountedRef.current = false
      window.removeEventListener('wallet-created', handleWalletCreated)
    }
  }, [fetchWallet]) // Only depends on fetchWallet, which is stable

  const fetchBalance = async () => {
    if (!wallet) {
      console.log('[WalletDisplay] No wallet available, skipping balance fetch')
      return
    }

    console.log('[WalletDisplay] Fetching balance for wallet:', wallet.polymarket_funder_address)

    // Create a timeout to automatically fail after 15 seconds
    const timeoutId = setTimeout(() => {
      if (mountedRef.current && balanceLoading) {
        console.error('[WalletDisplay] Balance fetch timed out after 15 seconds')
        setBalanceError(true)
        setBalance(null)
        setBalanceLoading(false)
      }
    }, 15000)

    try {
      setBalanceLoading(true)
      setBalanceError(false) // Reset error state
      const balanceData = await walletService.getWalletBalance(apiRef.current)
      clearTimeout(timeoutId) // Clear timeout on success

      console.log('[WalletDisplay] Balance data received:', balanceData)

      if (mountedRef.current) {
        setBalance(balanceData.usdc_balance)
        console.log('[WalletDisplay] Balance set to:', balanceData.usdc_balance)
      }
    } catch (err: any) {
      clearTimeout(timeoutId) // Clear timeout on error
      console.error('[WalletDisplay] Failed to fetch balance:', err)
      if (mountedRef.current) {
        setBalanceError(true)
        setBalance(null) // Keep as null to show error state
      }
    } finally {
      if (mountedRef.current) {
        setBalanceLoading(false)
      }
    }
  }

  const handleCopy = async () => {
    if (!wallet) return
    try {
      await navigator.clipboard.writeText(wallet.polymarket_funder_address)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch (err) {
      console.error('Failed to copy:', err)
    }
  }

  const truncateAddress = (address: string) => {
    if (address.length <= 10) return address
    return `${address.slice(0, 6)}...${address.slice(-4)}`
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-border bg-muted/50 p-3">
        <div className="flex items-center space-x-2">
          <WalletIcon className="h-4 w-4 text-muted-foreground animate-pulse" />
          <span className="text-xs text-muted-foreground">Loading wallet...</span>
        </div>
      </div>
    )
  }

  if (error || !wallet) {
    return (
      <div className="rounded-lg border border-border bg-muted/50 p-3">
        <div className="flex items-center space-x-2">
          <WalletIcon className="h-4 w-4 text-muted-foreground" />
          <span className="text-xs text-muted-foreground">No wallet found</span>
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-border bg-muted/50 p-3 space-y-2">
      <div className="flex items-center justify-between">
        <div className="flex items-center space-x-2">
          <WalletIcon className="h-4 w-4 text-primary" />
          <span className="text-xs font-medium text-foreground">Wallet</span>
        </div>
        {wallet.is_active && (
          <span className="text-xs px-1.5 py-0.5 rounded bg-constructive/10 text-constructive">
            Active
          </span>
        )}
      </div>
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <div className="flex items-center space-x-2 group cursor-pointer">
              <code className="text-xs font-mono text-muted-foreground group-hover:text-foreground transition-colors">
                {truncateAddress(wallet.polymarket_funder_address)}
              </code>
              <Button
                variant="ghost"
                size="sm"
                className="h-5 w-5 p-0"
                onClick={handleCopy}
              >
                {copied ? (
                  <Check className="h-3 w-3 text-constructive" />
                ) : (
                  <Copy className="h-3 w-3 text-muted-foreground" />
                )}
              </Button>
            </div>
          </TooltipTrigger>
          <TooltipContent>
            <p className="font-mono text-xs">{wallet.polymarket_funder_address}</p>
            <p className="text-xs mt-1">{copied ? 'Copied!' : 'Click to copy'}</p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>

      {/* Balance Display */}
      <div className="flex items-center justify-between mt-2 pt-2 border-t border-border">
        <span className="text-xs text-muted-foreground">Balance:</span>
        <div className="flex items-center space-x-2">
          {balanceLoading ? (
            <div className="flex items-center space-x-1">
              <div className="w-3 h-3 border-2 border-primary border-t-transparent rounded-full animate-spin"></div>
              <span className="text-xs text-muted-foreground">Loading...</span>
            </div>
          ) : balance !== null ? (
            <div className="flex items-center space-x-1">
              <span className="text-xs font-mono font-semibold text-foreground">
                ${parseFloat(balance).toLocaleString(undefined, {
                  minimumFractionDigits: 2,
                  maximumFractionDigits: 2
                })} USDC
              </span>
              <button
                onClick={fetchBalance}
                className="text-xs text-muted-foreground hover:text-foreground opacity-50 hover:opacity-100"
                disabled={balanceLoading}
                title="Refresh balance"
              >
                ↻
              </button>
            </div>
          ) : balanceError && wallet ? (
            <div className="flex items-center space-x-1">
              <span className="text-xs text-red-600 dark:text-red-400">Failed to load</span>
              <button
                onClick={fetchBalance}
                className="text-xs text-blue-600 dark:text-blue-400 hover:underline"
                disabled={balanceLoading}
              >
                Retry
              </button>
            </div>
          ) : wallet ? (
            <div className="flex items-center space-x-1">
              <div className="w-2 h-2 bg-yellow-500 rounded-full animate-pulse"></div>
              <span className="text-xs text-yellow-600 dark:text-yellow-400">Waiting...</span>
              <button
                onClick={fetchBalance}
                className="text-xs text-blue-600 dark:text-blue-400 hover:underline ml-1"
                disabled={balanceLoading}
              >
                Load
              </button>
            </div>
          ) : (
            <span className="text-xs text-muted-foreground">No wallet</span>
          )}
        </div>
      </div>
    </div>
  )
}

