import type { QueryClient } from "@tanstack/react-query"
import { tagsQuery } from "@/lib/api/tags"
import {
  type TagSummary,
  type TransactionEnrichmentPreview,
  type TransactionTagItem,
} from "@/lib/api/types.generated"
import { utcToDateInputValue, utcToTimeInputValue } from "@/lib/format"

function mergeEnrichedPreviewTags(
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
    let next: TagSummary[] | undefined
    const knownIds = new Set(prev.map((tag) => tag.id))
    for (const tag of tags) {
      if (!knownIds.has(tag.id)) {
        knownIds.add(tag.id)
        next ??= [...prev]
        next.push({
          id: tag.id,
          name: tag.name,
          transaction_count: 1,
          created_at_utc: fallbackCreatedAtUtc,
        })
        missingOrAdded = true
      }
    }
    return next ? next.sort((a, b) => a.name.localeCompare(b.name)) : prev
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
