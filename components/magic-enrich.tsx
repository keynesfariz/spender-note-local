"use client"

import type { QueryClient } from "@tanstack/react-query"
import { CircleAlertIcon, Loader2Icon, SparklesIcon } from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"

import { IconTooltip } from "@/components/icon-tooltip"
import { Button } from "@/components/ui/button"
import { tagsQuery } from "@/lib/api/tags"
import { enrichTransactionsApi } from "@/lib/api/transactions"
import {
  type TagSummary,
  type TransactionEnrichmentPreview,
  type TransactionTagItem,
} from "@/lib/api/types.generated"
import { utcToDateInputValue, utcToTimeInputValue } from "@/lib/format"

export function isAbortError(err: unknown): boolean {
  return err instanceof DOMException && err.name === "AbortError"
}

export function mergeEnrichedPreviewTags(
  preview: TransactionEnrichmentPreview,
  currentSelectedTagIds: string[],
  knownTags: TagSummary[]
): { nextSelectedTagIds: string[]; nextDraftTagNames: string[] } {
  const nextSelected = [...currentSelectedTagIds]
  const selectedSet = new Set(currentSelectedTagIds)
  const knownByLower = new Map<string, TagSummary>()
  const selectedNamesLower = new Set<string>()
  for (const tag of knownTags) {
    const lower = tag.name.trim().toLowerCase()
    knownByLower.set(lower, tag)
    if (selectedSet.has(tag.id)) {
      selectedNamesLower.add(lower)
    }
  }

  for (const matched of preview.existing_tags ?? []) {
    if (!selectedSet.has(matched.id)) {
      selectedSet.add(matched.id)
      nextSelected.push(matched.id)
    }
    selectedNamesLower.add(matched.name.trim().toLowerCase())
  }

  const nextDrafts: string[] = []
  const seenDraftLower = new Set<string>()
  for (const rawName of preview.draft_tag_names ?? []) {
    const trimmed = rawName.trim()
    if (!trimmed) continue
    const lower = trimmed.toLowerCase()
    if (seenDraftLower.has(lower)) continue

    const known = knownByLower.get(lower)
    if (known) {
      if (!selectedSet.has(known.id)) {
        selectedSet.add(known.id)
        nextSelected.push(known.id)
      }
      selectedNamesLower.add(lower)
      continue
    }

    if (selectedNamesLower.has(lower)) continue
    seenDraftLower.add(lower)
    nextDrafts.push(trimmed)
  }

  return {
    nextSelectedTagIds: nextSelected,
    nextDraftTagNames: nextDrafts,
  }
}

function syncAndMergeEnrichedTags({
  queryClient,
  preview,
  currentSelectedTagIds,
  fallbackKnownTags,
}: {
  queryClient: QueryClient
  preview: TransactionEnrichmentPreview
  currentSelectedTagIds: string[]
  fallbackKnownTags: TagSummary[]
}): { nextSelectedTagIds: string[]; nextDraftTagNames: string[] } {
  const knownTags =
    queryClient.getQueryData<TagSummary[]>(tagsQuery().queryKey) ??
    fallbackKnownTags
  if (preview.existing_tags?.length) {
    const addedToCache = patchTagsQueryCache(
      queryClient,
      preview.existing_tags,
      new Date().toISOString()
    )
    if (addedToCache) {
      void queryClient.invalidateQueries({
        queryKey: tagsQuery().queryKey,
      })
    }
  }
  return mergeEnrichedPreviewTags(preview, currentSelectedTagIds, knownTags)
}

function resolveEnrichedDateTimeInputs(
  occurredAtUtc: string | null | undefined,
  timezone: string | null | undefined
): { date: string; time: string } | null {
  if (!occurredAtUtc) return null
  const nextDate = utcToDateInputValue(occurredAtUtc, timezone)
  const nextTime = utcToTimeInputValue(occurredAtUtc, timezone)
  if (!nextDate || !nextTime) return null
  return { date: nextDate, time: nextTime }
}

export function patchTagsQueryCache(
  queryClient: QueryClient,
  tags: readonly TransactionTagItem[],
  fallbackCreatedAtUtc: string
): boolean {
  if (tags.length === 0) return false
  let missingOrAdded = false
  queryClient.setQueryData<TagSummary[]>(tagsQuery().queryKey, (prev) => {
    if (!prev) {
      missingOrAdded = true
      return prev
    }
    const next = [...prev]
    const knownIds = new Set(prev.map((tag) => tag.id))
    let changed = false
    for (const tag of tags) {
      if (!knownIds.has(tag.id)) {
        knownIds.add(tag.id)
        next.push({
          id: tag.id,
          name: tag.name,
          transaction_count: 1,
          created_at_utc: fallbackCreatedAtUtc,
        })
        changed = true
        missingOrAdded = true
      }
    }
    return changed ? next.sort((a, b) => a.name.localeCompare(b.name)) : prev
  })
  return missingOrAdded
}

export type MagicField =
  | "Remark"
  | "Merchant for reports"
  | "Category"
  | "Date"
  | "Time"
  | "Note"
  | "Tags"

type MagicDraft = {
  remark: string
  merchant: string
  remarkUserEdited: boolean
  category: string
  date: string
  time: string
  note: string
  selectedTagIds: string[]
  draftTagNames: string[]
}

export function resolveMagicDraft(
  draft: MagicDraft,
  preview: TransactionEnrichmentPreview,
  queryClient: QueryClient,
  fallbackKnownTags: TagSummary[],
  timezone: string | null | undefined
) {
  const dateTime = resolveEnrichedDateTimeInputs(
    preview.occurred_at_utc,
    timezone
  )
  const { nextSelectedTagIds, nextDraftTagNames } = syncAndMergeEnrichedTags({
    queryClient,
    preview,
    currentSelectedTagIds: draft.selectedTagIds,
    fallbackKnownTags,
  })
  const preserveRemark = draft.remarkUserEdited && Boolean(draft.remark.trim())
  const next: MagicDraft = {
    remark: preserveRemark ? draft.remark : preview.remark,
    merchant: preview.merchant,
    remarkUserEdited: preserveRemark,
    category: preview.category,
    date: dateTime?.date ?? draft.date,
    time: dateTime?.time ?? draft.time,
    note: preview.note ?? "",
    selectedTagIds: nextSelectedTagIds,
    draftTagNames: nextDraftTagNames,
  }
  const changed = new Set<MagicField>()
  if (draft.remark.trim() !== next.remark.trim()) changed.add("Remark")
  if (draft.merchant !== next.merchant) changed.add("Merchant for reports")
  if (draft.category !== next.category) changed.add("Category")
  if (draft.date !== next.date) changed.add("Date")
  if (draft.time !== next.time) changed.add("Time")
  if (draft.note.trim() !== next.note.trim()) changed.add("Note")
  if (
    draft.selectedTagIds.length !== next.selectedTagIds.length ||
    draft.draftTagNames.length !== next.draftTagNames.length ||
    JSON.stringify(draft.draftTagNames.toSorted()) !==
      JSON.stringify(next.draftTagNames.toSorted())
  ) {
    changed.add("Tags")
  }
  const message = changed.size
    ? `Magic updated: ${[...changed].join(", ")}. Review before saving.`
    : "Magic made no changes."
  return { next, changed, message }
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
