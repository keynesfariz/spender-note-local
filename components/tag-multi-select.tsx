"use client"

import { IconTooltip } from "@/components/icon-tooltip"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { PlusIcon, TagIcon, XIcon } from "lucide-react"
import {
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import { createTagApi } from "@/lib/api/tags"
import { type TagSummary } from "@/lib/api/types.generated"

const EMPTY_DRAFT_TAG_NAMES: string[] = []

function normalizeTagName(name: string) {
  return name.trim().toLowerCase()
}

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
  onCreatePendingChange,
  disabled = false,
}: {
  tags: TagSummary[]
  selectedTagIds: string[]
  onChange: Dispatch<SetStateAction<string[]>>
  draftTagNames?: string[]
  onDraftTagNamesChange?: Dispatch<SetStateAction<string[]>>
  onCreatePendingChange: (pending: boolean) => void
  disabled?: boolean
}) {
  const queryClient = useQueryClient()
  const searchInputRef = useRef<HTMLInputElement>(null)
  const creatingTagRef = useRef(false)
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const [createError, setCreateError] = useState<string | null>(null)
  const [sessionDraftNames, setSessionDraftNames] = useState<string[]>([])

  const trimmedSearch = search.trim()
  const selectedSet = useMemo(() => new Set(selectedTagIds), [selectedTagIds])

  const tagById = useMemo(() => new Map(tags.map((t) => [t.id, t])), [tags])

  const selectedTags = useMemo(
    () =>
      selectedTagIds.flatMap((id) => {
        const tag = tagById.get(id)
        return tag ? [tag] : []
      }),
    [selectedTagIds, tagById]
  )

  const persistedNamesLower = useMemo(
    () => new Set(tags.map((tag) => normalizeTagName(tag.name))),
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

  const searchName = normalizeTagName(search)
  const matchingTag = tags.find(
    (tag) => normalizeTagName(tag.name) === searchName
  )
  const matchingDraft = popoverDraftNames.find(
    (name) => normalizeTagName(name) === searchName
  )

  const createTagMutation = useMutation({
    mutationFn: (name: string) => createTagApi({ name }),
    onSuccess: (created) => {
      queryClient.setQueryData<TagSummary[]>(["tags"], (prev) => {
        if (!prev) return [created]
        if (prev.some((t) => t.id === created.id)) return prev
        return [...prev, created].sort((a, b) => a.name.localeCompare(b.name))
      })
      void queryClient.invalidateQueries({ queryKey: ["tags"] })
    },
    onError: () => {
      void queryClient.invalidateQueries({ queryKey: ["tags"] })
    },
  })

  function selectTag(tag: TagSummary) {
    onChange((ids) => (ids.includes(tag.id) ? ids : [...ids, tag.id]))
    if (onDraftTagNamesChange) {
      const name = normalizeTagName(tag.name)
      onDraftTagNamesChange((drafts) =>
        drafts.some((draft) => normalizeTagName(draft) === name)
          ? drafts.filter((draft) => normalizeTagName(draft) !== name)
          : drafts
      )
    }
  }

  function toggleTag(tag: TagSummary) {
    if (selectedSet.has(tag.id)) {
      onChange((ids) => ids.filter((id) => id !== tag.id))
    } else {
      selectTag(tag)
    }
  }

  function selectDraftTag(name: string) {
    const needle = normalizeTagName(name)
    onDraftTagNamesChange?.((drafts) =>
      drafts.some((draft) => normalizeTagName(draft) === needle)
        ? drafts
        : [...drafts, name]
    )
  }

  function toggleDraftTag(name: string) {
    const needle = normalizeTagName(name)
    if (activeDraftSet.has(needle)) {
      onDraftTagNamesChange?.((drafts) =>
        drafts.filter((draft) => normalizeTagName(draft) !== needle)
      )
      return
    }
    selectDraftTag(name)
  }

  function addTypedTag() {
    if (disabled || !trimmedSearch || creatingTagRef.current) return
    setCreateError(null)
    if (matchingTag) {
      selectTag(matchingTag)
      setSearch("")
      return
    }
    if (matchingDraft) {
      selectDraftTag(matchingDraft)
      setSearch("")
      return
    }

    creatingTagRef.current = true
    onCreatePendingChange(true)
    // Per-call callbacks run only while the initiating picker is mounted.
    createTagMutation.mutate(trimmedSearch, {
      onSuccess: (tag) => {
        selectTag(tag)
        setSearch("")
      },
      onError: (error) => {
        setCreateError(error.message || "Failed to create tag.")
      },
      onSettled: () => {
        creatingTagRef.current = false
        onCreatePendingChange(false)
      },
    })
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
                <button
                  type="button"
                  disabled={disabled}
                  aria-label={`Remove tag ${tag.name}`}
                  onClick={() => toggleTag(tag)}
                  className="rounded-xs p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
                >
                  <XIcon className="size-3" />
                </button>
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
            if (!creatingTagRef.current && !createError) setSearch("")
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

        <PopoverContent
          align="start"
          initialFocus={searchInputRef}
          className="w-72 max-w-(--available-width) p-3"
        >
          <div className="space-y-2.5">
            <Input
              ref={searchInputRef}
              type="text"
              value={search}
              readOnly={createTagMutation.isPending}
              onChange={(e) => {
                setSearch(e.target.value)
                if (createError) setCreateError(null)
              }}
              onKeyDown={(event) => {
                if (event.key !== "Enter") return
                event.preventDefault()
                if (
                  event.nativeEvent.isComposing ||
                  event.nativeEvent.keyCode === 229
                )
                  return
                addTypedTag()
              }}
              placeholder="Search or create tag..."
              aria-label="Search or create tag"
            />

            {createError ? (
              <p role="alert" className="text-xs text-destructive">
                {createError}
              </p>
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
                          onCheckedChange={() => toggleTag(tag)}
                        />
                        <span className="truncate">{tag.name}</span>
                      </label>
                    )
                  })}
                </>
              )}
            </div>

            {trimmedSearch && !matchingTag && !matchingDraft ? (
              <Button
                type="button"
                variant="secondary"
                size="sm"
                className="w-full justify-start"
                disabled={disabled || createTagMutation.isPending}
                onClick={() => {
                  searchInputRef.current?.focus()
                  addTypedTag()
                }}
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
