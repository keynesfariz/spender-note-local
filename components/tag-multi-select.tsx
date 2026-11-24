"use client"

import { IconTooltip } from "@/components/ui/tooltip"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, TagIcon, XIcon } from "lucide-react"
import { useMemo, useState } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { createTagApi, type TagSummary } from "@/lib/api"

const EMPTY_DRAFT_TAG_NAMES: string[] = []

function dedupeDraftTagNames(
  names: readonly string[],
  persistedNamesLower: ReadonlySet<string>
): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of names) {
    const trimmed = raw.trim()
    if (!trimmed) continue
    const lower = trimmed.toLowerCase()
    if (persistedNamesLower.has(lower) || seen.has(lower)) continue
    seen.add(lower)
    out.push(trimmed)
  }
  return out
}

export function TagMultiSelect({
  tags,
  selectedTagIds,
  onChange,
  draftTagNames = EMPTY_DRAFT_TAG_NAMES,
  onDraftTagNamesChange,
  disabled = false,
}: {
  tags: TagSummary[]
  selectedTagIds: string[]
  onChange: (nextIds: string[]) => void
  draftTagNames?: string[]
  onDraftTagNamesChange?: (nextDrafts: string[]) => void
  disabled?: boolean
}) {
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const [createError, setCreateError] = useState<string | null>(null)
  const [sessionDraftNames, setSessionDraftNames] = useState<string[]>([])

  const trimmedSearch = search.trim()
  const selectedSet = useMemo(() => new Set(selectedTagIds), [selectedTagIds])

  const tagById = useMemo(() => new Map(tags.map((t) => [t.id, t])), [tags])

  const selectedTags = useMemo(
    () =>
      selectedTagIds
        .map((id) => tagById.get(id))
        .filter((t): t is TagSummary => Boolean(t)),
    [selectedTagIds, tagById]
  )

  const persistedNamesLower = useMemo(
    () => new Set(tags.map((t) => t.name.trim().toLowerCase())),
    [tags]
  )

  const activeDraftNames = useMemo(
    () => dedupeDraftTagNames(draftTagNames, persistedNamesLower),
    [draftTagNames, persistedNamesLower]
  )

  const activeDraftSet = useMemo(
    () => new Set(activeDraftNames.map((n) => n.toLowerCase())),
    [activeDraftNames]
  )

  const popoverDraftNames = useMemo(
    () =>
      dedupeDraftTagNames(
        [...sessionDraftNames, ...activeDraftNames],
        persistedNamesLower
      ),
    [sessionDraftNames, activeDraftNames, persistedNamesLower]
  )

  const filteredTags = useMemo(() => {
    if (!trimmedSearch) return tags
    const needle = trimmedSearch.toLowerCase()
    return tags.filter((t) => t.name.toLowerCase().includes(needle))
  }, [tags, trimmedSearch])

  const filteredDraftNames = useMemo(() => {
    if (!trimmedSearch) return popoverDraftNames
    const needle = trimmedSearch.toLowerCase()
    return popoverDraftNames.filter((name) =>
      name.toLowerCase().includes(needle)
    )
  }, [popoverDraftNames, trimmedSearch])

  const hasExactMatch = useMemo(() => {
    if (!trimmedSearch) return true
    const needle = trimmedSearch.toLowerCase()
    return persistedNamesLower.has(needle) || activeDraftSet.has(needle)
  }, [persistedNamesLower, activeDraftSet, trimmedSearch])

  const createTagMutation = useMutation({
    mutationFn: (name: string) => createTagApi({ name }),
    onSuccess: (created) => {
      setCreateError(null)
      queryClient.setQueryData<TagSummary[]>(["tags"], (prev) => {
        if (!prev) return [created]
        if (prev.some((t) => t.id === created.id)) return prev
        return [...prev, created].sort((a, b) => a.name.localeCompare(b.name))
      })
      void queryClient.invalidateQueries({ queryKey: ["tags"] })
      if (!selectedTagIds.includes(created.id)) {
        onChange([...selectedTagIds, created.id])
      }
      setSearch("")
    },
    onError: (err) => {
      setCreateError(
        err instanceof Error && err.message
          ? err.message
          : "Failed to create tag."
      )
    },
  })

  const toggleTag = (tagId: string) => {
    if (selectedSet.has(tagId)) {
      onChange(selectedTagIds.filter((id) => id !== tagId))
    } else {
      onChange([...selectedTagIds, tagId])
    }
  }

  const toggleDraftTag = (name: string) => {
    if (!onDraftTagNamesChange) return
    const needle = name.trim().toLowerCase()
    if (activeDraftSet.has(needle)) {
      onDraftTagNamesChange(
        activeDraftNames.filter((n) => n.toLowerCase() !== needle)
      )
    } else {
      onDraftTagNamesChange([...activeDraftNames, name])
    }
  }

  const totalSelectedCount = selectedTags.length + activeDraftNames.length

  return (
    <div className="space-y-2">
      {totalSelectedCount > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {selectedTags.map((tag) => (
            <Badge key={tag.id} variant="secondary" className="gap-1 pr-1">
              <span>{tag.name}</span>
              <IconTooltip label="Remove tag">
                <IconTooltip label="Remove tag">
                  <button
                    type="button"
                    disabled={disabled}
                    aria-label={`Remove tag ${tag.name}`}
                    onClick={() => toggleTag(tag.id)}
                    className="rounded-xs p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
                  >
                    <XIcon className="size-3" />
                  </button>
                </IconTooltip>
              </IconTooltip>
            </Badge>
          ))}
          {activeDraftNames.map((name) => (
            <Badge
              key={`draft:${name.toLowerCase()}`}
              variant="outline"
              className="gap-1 border-dashed pr-1"
            >
              <span>{name}</span>
              <span className="text-[10px] text-muted-foreground">New</span>
              <button
                type="button"
                disabled={disabled}
                aria-label={`Remove draft tag ${name}`}
                onClick={() => toggleDraftTag(name)}
                className="rounded-xs p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
              >
                <XIcon className="size-3" />
              </button>
            </Badge>
          ))}
        </div>
      ) : null}

      <Popover
        open={open}
        onOpenChange={(next) => {
          setOpen(next)
          if (next) {
            setSessionDraftNames(activeDraftNames)
          } else {
            setSessionDraftNames([])
            setSearch("")
            setCreateError(null)
          }
        }}
      >
        <PopoverTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={disabled}
              className="w-full justify-start font-normal text-muted-foreground"
            />
          }
        >
          <TagIcon className="mr-1.5 size-3.5" />
          {totalSelectedCount > 0
            ? `Tags (${totalSelectedCount} selected)`
            : "Add tags..."}
        </PopoverTrigger>

        <PopoverContent align="start" className="w-72 p-3">
          <div className="space-y-2.5">
            <Input
              type="text"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value)
                if (createError) setCreateError(null)
              }}
              placeholder="Search or create tag..."
              aria-label="Search or create tag"
              autoFocus
            />

            {createError ? (
              <p className="text-xs text-destructive">{createError}</p>
            ) : null}

            <div className="max-h-48 space-y-1 overflow-y-auto">
              {filteredTags.length === 0 &&
              filteredDraftNames.length === 0 &&
              !trimmedSearch ? (
                <p className="py-2 text-center text-xs text-muted-foreground">
                  No tags yet. Type a name above to create one.
                </p>
              ) : filteredTags.length === 0 &&
                filteredDraftNames.length === 0 ? (
                <p className="py-1.5 text-center text-xs text-muted-foreground">
                  No matching tags.
                </p>
              ) : (
                <>
                  {filteredDraftNames.map((name) => {
                    const lower = name.toLowerCase()
                    const checked = activeDraftSet.has(lower)
                    return (
                      <label
                        key={`draft-opt:${lower}`}
                        className="flex cursor-pointer items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-muted"
                      >
                        <span className="flex min-w-0 items-center gap-2">
                          <Checkbox
                            checked={checked}
                            onCheckedChange={() => toggleDraftTag(name)}
                          />
                          <span className="truncate">{name}</span>
                        </span>
                        <span className="shrink-0 text-[10px] text-muted-foreground">
                          New
                        </span>
                      </label>
                    )
                  })}
                  {filteredTags.map((tag) => {
                    const checked = selectedSet.has(tag.id)
                    return (
                      <label
                        key={tag.id}
                        className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-muted"
                      >
                        <Checkbox
                          checked={checked}
                          onCheckedChange={() => toggleTag(tag.id)}
                        />
                        <span className="truncate">{tag.name}</span>
                      </label>
                    )
                  })}
                </>
              )}
            </div>

            {trimmedSearch && !hasExactMatch ? (
              <Button
                type="button"
                variant="secondary"
                size="sm"
                className="w-full justify-start"
                disabled={createTagMutation.isPending}
                onClick={() => createTagMutation.mutate(trimmedSearch)}
              >
                <PlusIcon className="mr-1.5 size-3.5" />
                {createTagMutation.isPending
                  ? "Creating..."
                  : `+ Create "${trimmedSearch}"`}
              </Button>
            ) : null}
          </div>
        </PopoverContent>
      </Popover>
    </div>
  )
}
