"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { MoonIcon, SunIcon, WalletIcon } from "lucide-react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import { useTheme } from "next-themes"
import { useState, type ReactNode } from "react"

import { SyncToast } from "@/components/sync-toast"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { IconTooltip } from "@/components/ui/tooltip"
import { gmailStatusQuery, syncStatusQuery, triggerSyncApi } from "@/lib/api"

const NAV_ITEMS = [
  { href: "/transactions", label: "Transactions" },
  { href: "/accounts", label: "Accounts" },
  { href: "/categories", label: "Categories & Tags" },
  { href: "/settings", label: "Settings" },
]

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname()
  const { resolvedTheme, setTheme } = useTheme()
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
      <header className="sticky top-0 z-30 border-b bg-card/95 backdrop-blur">
        <div className="mx-auto grid max-w-6xl grid-cols-[1fr_auto] items-center gap-x-4 px-4 sm:flex sm:h-16 sm:justify-between sm:px-6">
          <div className="contents sm:flex sm:min-w-0 sm:items-center sm:gap-6">
            <Link
              href="/"
              className="flex w-fit items-center gap-2 py-3 font-semibold tracking-tight text-primary"
              aria-current={pathname === "/" ? "page" : undefined}
            >
              <span className="grid size-8 place-items-center rounded-lg bg-primary text-primary-foreground">
                <WalletIcon className="size-4" />
              </span>
              Spender
            </Link>
            <nav
              aria-label="Primary"
              className="col-span-2 row-start-2 flex min-w-0 items-center gap-1 overflow-x-auto pb-2 sm:pb-0"
            >
              {NAV_ITEMS.map((item) => {
                const active =
                  item.href === "/"
                    ? pathname === "/"
                    : pathname.startsWith(item.href)
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    aria-current={active ? "page" : undefined}
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

          <div className="flex shrink-0 items-center gap-2">
            <IconTooltip label="Toggle light and dark mode">
              <Button
                type="button"
                variant="outline"
                size="icon"
                className="size-10 rounded-full"
                onClick={() =>
                  setTheme(resolvedTheme === "dark" ? "light" : "dark")
                }
              >
                <MoonIcon className="dark:hidden" aria-hidden="true" />
                <SunIcon className="hidden dark:block" aria-hidden="true" />
                <span className="sr-only dark:hidden">Switch to dark mode</span>
                <span className="sr-only hidden dark:block">
                  Switch to light mode
                </span>
              </Button>
            </IconTooltip>

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
        </div>
      </header>

      <div className="mx-auto max-w-6xl space-y-4 px-4 pt-4 sm:px-6">
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
                <Link href="/settings" className="text-link underline">
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

      <main className="mx-auto max-w-6xl px-4 py-8 sm:px-6">{children}</main>
      <SyncToast />
    </div>
  )
}
