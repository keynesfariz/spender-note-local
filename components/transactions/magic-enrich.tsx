"use client"

import { CircleAlertIcon, Loader2Icon, SparklesIcon } from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"

import { IconTooltip } from "@/components/icon-tooltip"
import { Button } from "@/components/ui/button"
import { enrichTransactionsApi } from "@/lib/api/transactions"
import { type TransactionEnrichmentPreview } from "@/lib/api/types.generated"

export function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError"
}

export function useMagicEnrich() {
  const [isEnriching, setIsEnriching] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const seqRef = useRef(0)

  const abortEnrichment = useCallback(() => {
    seqRef.current += 1
    abortRef.current?.abort()
    abortRef.current = null
    setIsEnriching(false)
  }, [])

  useEffect(() => {
    return () => {
      seqRef.current += 1
      abortRef.current?.abort()
      abortRef.current = null
    }
  }, [])

  const runEnrichment = useCallback(
    async ({
      transactionIds,
      onStart,
      onSuccess,
      onError,
    }: {
      transactionIds: string[]
      onStart?: () => void
      onSuccess: (preview: TransactionEnrichmentPreview) => void
      onError: (message: string) => void
    }) => {
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller
      const reqSeq = ++seqRef.current

      onStart?.()
      setIsEnriching(true)

      try {
        const preview = await enrichTransactionsApi(
          { transactionIds },
          { signal: controller.signal }
        )
        if (reqSeq !== seqRef.current || controller.signal.aborted) {
          return
        }
        onSuccess(preview)
      } catch (err) {
        if (
          reqSeq !== seqRef.current ||
          controller.signal.aborted ||
          isAbortError(err)
        ) {
          return
        }
        onError(
          err instanceof Error && err.message
            ? err.message
            : "Failed to enrich transaction with Magic LLM."
        )
      } finally {
        if (reqSeq === seqRef.current) {
          abortRef.current = null
          setIsEnriching(false)
        }
      }
    },
    []
  )

  return { isEnriching, abortEnrichment, runEnrichment }
}

export function MagicEnrichControl({
  isEnriching,
  disabled = false,
  onClick,
  description,
  statusMessage,
}: {
  isEnriching: boolean
  disabled?: boolean
  onClick: () => void
  description: string
  statusMessage?: string | null
}) {
  const tooltipLabel = statusMessage || description
  return (
    <div className="flex shrink-0 items-center gap-1">
      <MagicEnrichButton
        isEnriching={isEnriching}
        disabled={disabled}
        onClick={onClick}
      />
      <IconTooltip label={tooltipLabel}>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className={
            statusMessage
              ? "text-emerald-600 dark:text-emerald-400"
              : "text-muted-foreground"
          }
          aria-label={tooltipLabel}
        >
          <CircleAlertIcon className="size-4" aria-hidden="true" />
        </Button>
      </IconTooltip>
    </div>
  )
}

export function MagicEnrichButton({
  isEnriching,
  disabled = false,
  onClick,
}: {
  isEnriching: boolean
  disabled?: boolean
  onClick: () => void
}) {
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={disabled || isEnriching}
      onClick={onClick}
    >
      {isEnriching ? (
        <>
          <span className="mr-1.5 animate-spin">
            <Loader2Icon className="size-3.5" />
          </span>
          Enriching...
        </>
      ) : (
        <>
          <SparklesIcon className="mr-1.5 size-3.5" />
          Magic LLM
        </>
      )}
    </Button>
  )
}
