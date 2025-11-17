/**
 * @description
 * Modal component for creating a new wallet. Allows users to either:
 * 1. Generate a new wallet (default)
 * 2. Link an existing Polymarket proxy wallet
 */

'use client'

import { useState, FormEvent } from 'react'
import { useApi } from '@/hooks/use-api'
import { walletService } from '@/lib/services/wallet-service'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Alert, AlertDescription } from '@/components/ui/alert'

interface CreateWalletModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}

export default function CreateWalletModal({
  open,
  onOpenChange,
  onSuccess,
}: CreateWalletModalProps) {
  const [mode, setMode] = useState<'new' | 'link'>('new')
  const [funderAddress, setFunderAddress] = useState('')
  const [privateKey, setPrivateKey] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  const api = useApi()

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setIsLoading(true)
    setError(null)
    setSuccess(null)

    try {
      const requestBody =
        mode === 'link'
          ? {
              polymarket_funder_address: funderAddress,
              private_key: privateKey,
            }
          : {}

      await walletService.createWallet(api, requestBody)

      setSuccess('Wallet created successfully!')
      
      // Dispatch custom event to notify other components
      window.dispatchEvent(new CustomEvent('wallet-created'))
      
      setTimeout(() => {
        onSuccess()
        onOpenChange(false)
        // Reset form
        setFunderAddress('')
        setPrivateKey('')
        setMode('new')
        setSuccess(null)
      }, 1500)
    } catch (err: any) {
      console.error('Failed to create wallet:', err)
      const errorMessage =
        err.response?.data?.message || err.message || 'Failed to create wallet'
      setError(errorMessage)
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>Create Wallet</DialogTitle>
          <DialogDescription>
            Create a new wallet or link your existing Polymarket proxy wallet to start trading.
          </DialogDescription>
        </DialogHeader>

        <Tabs value={mode} onValueChange={(value) => setMode(value as 'new' | 'link')}>
          <TabsList className="grid w-full grid-cols-2">
            <TabsTrigger value="new">New Wallet</TabsTrigger>
            <TabsTrigger value="link">Link Existing</TabsTrigger>
          </TabsList>

          <form onSubmit={handleSubmit} className="mt-4 space-y-4">
            <TabsContent value="new" className="space-y-4">
              <Alert>
                <AlertDescription>
                  A new Ethereum wallet will be generated for you. This wallet will be used for
                  all your trading operations on Polymarket.
                </AlertDescription>
              </Alert>
              <p className="text-sm text-muted-foreground">
                Click &quot;Create Wallet&quot; to generate a new wallet automatically.
              </p>
            </TabsContent>

            <TabsContent value="link" className="space-y-4">
              <Alert>
                <AlertDescription>
                  Link your existing Polymarket proxy wallet by providing your funder address and
                  private key. You can export your private key from{' '}
                  <a
                    href="https://reveal.magic.link/polymarket"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary underline"
                  >
                    reveal.magic.link/polymarket
                  </a>
                </AlertDescription>
              </Alert>

              <div className="space-y-2">
                <Label htmlFor="funderAddress">Polymarket Funder Address</Label>
                <Input
                  id="funderAddress"
                  type="text"
                  placeholder="0x..."
                  value={funderAddress}
                  onChange={(e) => setFunderAddress(e.target.value)}
                  required={mode === 'link'}
                  pattern="^0x[a-fA-F0-9]{40}$"
                />
                <p className="text-xs text-muted-foreground">
                  Your Polymarket proxy wallet address (shown below your profile picture)
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="privateKey">Private Key</Label>
                <Input
                  id="privateKey"
                  type="password"
                  placeholder="0x..."
                  value={privateKey}
                  onChange={(e) => setPrivateKey(e.target.value)}
                  required={mode === 'link'}
                />
                <p className="text-xs text-muted-foreground">
                  Your wallet&apos;s private key (keep this secure!)
                </p>
              </div>
            </TabsContent>

            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            {success && (
              <Alert className="border-constructive bg-constructive/10">
                <AlertDescription className="text-constructive">{success}</AlertDescription>
              </Alert>
            )}

            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={isLoading}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={isLoading}>
                {isLoading ? 'Creating...' : mode === 'new' ? 'Create Wallet' : 'Link Wallet'}
              </Button>
            </div>
          </form>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}

