"use client"

import { XIcon } from "lucide-react"
import { useEffect, useRef, useState } from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import type {
  ApplyTriageVerdictInput,
  TriageVerdictResult,
} from "@/lib/api/types.generated"

const UNDO_TIMEOUT_MS = 12_000

export function TriageUndoToast({
  verdict,
  action,
  isUndoing,
  isVerdictPending,
  undoError,
  onUndo,
  onDismiss,
}: {
  verdict: TriageVerdictResult
  action: ApplyTriageVerdictInput["action"]
  isUndoing: boolean
  isVerdictPending: boolean
  undoError: string | null
  onUndo: () => void
  onDismiss: () => void
}) {
  const [isHovered, setIsHovered] = useState(false)
  const [isFocused, setIsFocused] = useState(false)
  const remainingMsRef = useRef(UNDO_TIMEOUT_MS)
  const startedAtRef = useRef<number | null>(null)
  const onDismissRef = useRef(onDismiss)

  useEffect(() => {
    onDismissRef.current = onDismiss
  }, [onDismiss])

  const isPaused = isHovered || isFocused || isUndoing || isVerdictPending

  useEffect(() => {
    if (isPaused) {
      if (startedAtRef.current !== null) {
        const elapsed = Date.now() - startedAtRef.current
        remainingMsRef.current = Math.max(0, remainingMsRef.current - elapsed)
        startedAtRef.current = null
      }
      return
    }

    if (remainingMsRef.current <= 0) {
      const immediateId = window.setTimeout(() => onDismissRef.current(), 0)
      return () => window.clearTimeout(immediateId)
    }

    startedAtRef.current = Date.now()
    const timerId = window.setTimeout(() => {
      startedAtRef.current = null
      remainingMsRef.current = 0
      onDismissRef.current()
    }, remainingMsRef.current)

    return () => {
      if (startedAtRef.current !== null) {
        const elapsed = Date.now() - startedAtRef.current
        remainingMsRef.current = Math.max(0, remainingMsRef.current - elapsed)
        startedAtRef.current = null
      }
      window.clearTimeout(timerId)
    }
  }, [isPaused])

  const count = verdict.affectedCount
  const summary =
    action === "promote"
      ? `Promoted ${count} ${count === 1 ? "transaction" : "transactions"} · Pattern saved`
      : `Ignored ${count} ${count === 1 ? "email" : "emails"} · Rule saved`

  return (
    <div
      role="status"
      aria-live="polite"
      className="fixed right-5 bottom-[calc(1.25rem+var(--sync-toast-offset,0px))] left-5 z-50 min-[820px]:right-auto min-[820px]:bottom-5 min-[820px]:w-96"
      onMouseEnter={() => setIsHovered(true)}
      onMouseLeave={() => setIsHovered(false)}
      onFocusCapture={() => setIsFocused(true)}
      onBlurCapture={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) {
          setIsFocused(false)
        }
      }}
    >
      <Card className="border-border bg-card shadow-lg">
        <CardContent className="space-y-2.5 px-4 py-3">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0 space-y-0.5">
              <p className="text-sm font-medium text-foreground">{summary}</p>
              <p className="truncate text-xs text-muted-foreground">
                {verdict.policy.sender_email}
                {verdict.outOfRangeCount > 0
                  ? ` · ${verdict.outOfRangeCount} outside current date range`
                  : ""}
              </p>
            </div>
            <div className="flex shrink-0 items-center gap-1">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={isUndoing || isVerdictPending}
                onClick={onUndo}
              >
                {isUndoing ? "Undoing..." : "Undo"}
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                disabled={isUndoing}
                aria-label="Dismiss undo notification"
                onClick={onDismiss}
              >
                <XIcon className="size-4" aria-hidden="true" />
              </Button>
            </div>
          </div>

          {undoError && (
            <Alert variant="destructive">
              <AlertDescription>{undoError}</AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
