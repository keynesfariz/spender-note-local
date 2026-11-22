"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import Link from "next/link"
import { useEffect, useRef, useState } from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Separator } from "@/components/ui/separator"
import {
  cancelSyncApi,
  retryFailedExtractionsApi,
  syncStatusQuery,
} from "@/lib/api"

function formatPhase(phase: string): string {
  switch (phase) {
    case "preflight":
      return "Preparing sync..."
    case "fetching":
      return "Fetching Gmail messages..."
    case "extracting":
      return "Extracting with local LLM..."
    case "done":
      return "Finished"
    default:
      return phase
  }
}

function formatRunStatus(status: string): string {
  switch (status) {
    case "running":
      return "Syncing in background"
    case "completed":
      return "Sync complete"
    case "partial_error":
      return "Sync completed with errors"
    case "failed":
      return "Sync failed"
    case "cancelled":
      return "Sync cancelled"
    case "interrupted":
      return "Previous sync interrupted"
    default:
      return status
  }
}

export function SyncToast() {
  const queryClient = useQueryClient()
  const [dismissedRunId, setDismissedRunId] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const prevRunningRef = useRef(false)
  const prevRunIdRef = useRef<string | null>(null)
  const initializedRef = useRef(false)

  const statusQuery = useQuery({
    ...syncStatusQuery(),
    retry: false,
    refetchIntervalInBackground: false,
    refetchInterval: (q) => (q.state.data?.isRunning ? 1500 : false),
  })

  const isRunning = statusQuery.data?.isRunning ?? false
  const latestRunId = statusQuery.data?.latestRun?.id ?? null

  useEffect(() => {
    const transitionedToDone = prevRunningRef.current && !isRunning
    const fastCompletedNewRun =
      initializedRef.current &&
      !isRunning &&
      latestRunId !== null &&
      latestRunId !== prevRunIdRef.current

    if (transitionedToDone || fastCompletedNewRun) {
      void queryClient.invalidateQueries({ queryKey: ["spending"] })
      void queryClient.invalidateQueries({ queryKey: ["transactions"] })
      void queryClient.invalidateQueries({ queryKey: ["accounts"] })
      void queryClient.invalidateQueries({ queryKey: ["senders"] })
      void queryClient.invalidateQueries({ queryKey: ["gmail-status"] })
    }
    initializedRef.current = true
    prevRunningRef.current = isRunning
    prevRunIdRef.current = latestRunId
  }, [isRunning, latestRunId, queryClient])

  const cancelMutation = useMutation({
    mutationFn: () => cancelSyncApi(),
    onSuccess: () => {
      setActionError(null)
      void queryClient.invalidateQueries({ queryKey: ["sync-status"] })
    },
    onError: (err) => {
      setActionError(
        err instanceof Error ? err.message : "Could not cancel sync."
      )
    },
  })

  const retryMutation = useMutation({
    mutationFn: () => retryFailedExtractionsApi(),
    onSuccess: () => {
      setActionError(null)
      setDismissedRunId(null)
      void queryClient.invalidateQueries({ queryKey: ["sync-status"] })
    },
    onError: (err) => {
      setActionError(
        err instanceof Error ? err.message : "Could not retry extractions."
      )
    },
  })

  const run = statusQuery.data?.latestRun
  if (!run || (!isRunning && dismissedRunId === run.id)) {
    return null
  }

  return (
    <div className="fixed right-5 bottom-5 z-50 w-96 max-w-[calc(100vw-2.5rem)]">
      <Card>
        <CardHeader>
          <CardTitle>{formatRunStatus(run.status)}</CardTitle>
          <CardDescription>
            {run.status === "running"
              ? formatPhase(run.phase)
              : `Model: ${run.llm_model}`}
          </CardDescription>
          <CardAction>
            {run.status === "running" ? (
              <Button
                variant="outline"
                size="sm"
                disabled={cancelMutation.isPending}
                onClick={() => cancelMutation.mutate()}
              >
                {cancelMutation.isPending ? "Cancelling..." : "Cancel sync"}
              </Button>
            ) : (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setDismissedRunId(run.id)}
              >
                Dismiss
              </Button>
            )}
          </CardAction>
        </CardHeader>
        <CardContent className="space-y-3">
          <dl className="grid grid-cols-3 gap-2 text-xs">
            <div>
              <dt className="text-muted-foreground">Fetched</dt>
              <dd className="font-mono font-medium">{run.fetched_count}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Cleaned</dt>
              <dd className="font-mono font-medium">{run.cleaned_count}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Imported</dt>
              <dd className="font-mono font-medium">{run.imported_count}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Skipped</dt>
              <dd className="font-mono font-medium">{run.skipped_count}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Failed</dt>
              <dd className="font-mono font-medium">{run.failed_count}</dd>
            </div>
          </dl>

          {(actionError ?? run.error_message) && (
            <Alert variant="destructive">
              <AlertDescription>
                {actionError ?? run.error_message}
              </AlertDescription>
            </Alert>
          )}

          {run.status !== "running" &&
            (run.provisional_accounts_created ?? 0) > 0 && (
              <Alert>
                <AlertDescription>
                  {run.provisional_accounts_created} provisional{" "}
                  {run.provisional_accounts_created === 1
                    ? "account"
                    : "accounts"}{" "}
                  created.{" "}
                  <Link href="/accounts" className="underline">
                    Review accounts
                  </Link>
                </AlertDescription>
              </Alert>
            )}

          {run.status !== "running" && run.has_more_emails === 1 && (
            <Alert>
              <AlertDescription>
                Batch limit reached — more emails are available. Run Sync again
                to fetch the next batch.
              </AlertDescription>
            </Alert>
          )}

          {run.status !== "running" &&
            ((statusQuery.data?.failedEmailCount ?? 0) > 0 ||
              (statusQuery.data?.pendingEmailCount ?? 0) > 0) && (
              <>
                <Separator />
                <div className="flex items-center justify-between gap-2 text-xs">
                  <span className="text-muted-foreground">
                    {statusQuery.data?.pendingEmailCount ?? 0} pending,{" "}
                    {statusQuery.data?.failedEmailCount ?? 0} failed
                  </span>
                  <div className="flex items-center gap-2">
                    <Button
                      size="sm"
                      disabled={retryMutation.isPending}
                      onClick={() => retryMutation.mutate()}
                    >
                      Retry failed
                    </Button>
                    <Link
                      href="/settings"
                      className={buttonVariants({
                        variant: "link",
                        size: "sm",
                      })}
                    >
                      Settings
                    </Link>
                  </div>
                </div>
              </>
            )}
        </CardContent>
      </Card>
    </div>
  )
}
