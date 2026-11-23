"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { useState, type ReactNode } from "react"

import { SyncToast } from "@/components/sync-toast"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { gmailStatusQuery, syncStatusQuery, triggerSyncApi } from "@/lib/api"

const NAV_ITEMS = [
  { href: "/transactions", label: "Transactions" },
  { href: "/accounts", label: "Accounts" },
  { href: "/categories", label: "Categories & Tags" },
  { href: "/settings", label: "Settings" },
]

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname()
  const queryClient = useQueryClient()
  const [syncError, setSyncError] = useState<string | null>(null)

  const gmailQuery = useQuery(gmailStatusQuery())
  const syncQuery = useQuery(syncStatusQuery())

  const syncMutation = useMutation({
    mutationFn: () => triggerSyncApi(),
    onMutate: () => setSyncError(null),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["sync-status"] })
    },
    onError: (err) => {
      setSyncError(err instanceof Error ? err.message : "Could not start sync.")
    },
  })

  return (
    <div className="min-h-screen">
      <header className="sticky top-0 z-30 border-b bg-background">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-4 px-6">
          <div className="flex items-center gap-6">
            <Link href="/" className="font-semibold tracking-tight">
              Spender
            </Link>
            <nav aria-label="Primary" className="flex items-center gap-1">
              {NAV_ITEMS.map((item) => {
                const active =
                  item.href === "/"
                    ? pathname === "/"
                    : pathname.startsWith(item.href)
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={buttonVariants({
                      variant: active ? "secondary" : "ghost",
                      size: "sm",
                    })}
                  >
                    {item.label}
                  </Link>
                )
              })}
            </nav>
          </div>

          <Button
            size="sm"
            disabled={syncQuery.data?.isRunning || syncMutation.isPending}
            onClick={() => syncMutation.mutate()}
          >
            {syncQuery.data?.isRunning
              ? "Syncing..."
              : syncMutation.isPending
                ? "Starting..."
                : "Sync now"}
          </Button>
        </div>
      </header>

      <div className="mx-auto max-w-6xl space-y-4 px-6 pt-4">
        {gmailQuery.data?.status === "expired" && (
          <Alert>
            <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
              <span>
                Gmail authorization has expired. Reconnect your account to
                continue syncing.
              </span>
              <a
                href="/api/gmail/auth"
                className={buttonVariants({ size: "sm" })}
              >
                Reconnect Gmail
              </a>
            </AlertDescription>
          </Alert>
        )}

        {syncError && (
          <Alert variant="destructive">
            <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex items-center gap-3">
                <span>{syncError}</span>
                <Link href="/settings" className="underline">
                  Open Settings
                </Link>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setSyncError(null)}
              >
                Dismiss
              </Button>
            </AlertDescription>
          </Alert>
        )}
      </div>

      <main className="mx-auto max-w-6xl px-6 py-8">{children}</main>
      <SyncToast />
    </div>
  )
}
