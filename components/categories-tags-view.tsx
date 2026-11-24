"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  FolderKanbanIcon,
  PencilIcon,
  PlusIcon,
  TagIcon,
  Trash2Icon,
} from "lucide-react"
import Link from "next/link"
import { parseAsStringLiteral, useQueryState } from "nuqs"
import { useMemo, useState, type SubmitEvent } from "react"

import { CATEGORY_ICON_OPTIONS, DynamicIcon } from "@/components/dynamic-icon"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import {
  categoriesQuery,
  createCategoryApi,
  createTagApi,
  deleteCategoryApi,
  deleteTagApi,
  tagsQuery,
  updateCategoryApi,
  updateTagApi,
  type CategorySummary,
  type TagSummary,
} from "@/lib/api"
import { DEFAULT_CATEGORY } from "@/lib/categories"
import { cn } from "@/lib/utils"

const TAB_VALUES = ["categories", "tags"] as const

function formatTxCount(count: number): string {
  return `${count} transaction${count === 1 ? "" : "s"}`
}

function getErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

export function CategoriesTagsView() {
  const queryClient = useQueryClient()
  const [tab, setTab] = useQueryState(
    "tab",
    parseAsStringLiteral(TAB_VALUES).withDefault("categories")
  )

  const catQuery = useQuery(categoriesQuery())
  const tagQuery = useQuery(tagsQuery())

  const categories = useMemo(() => catQuery.data ?? [], [catQuery.data])
  const tags = useMemo(() => tagQuery.data ?? [], [tagQuery.data])

  const [catDialogOpen, setCatDialogOpen] = useState(false)
  const [editingCategory, setEditingCategory] =
    useState<CategorySummary | null>(null)
  const [catName, setCatName] = useState("")
  const [catIcon, setCatIcon] = useState("Tag")
  const [catFormError, setCatFormError] = useState<string | null>(null)

  const [deletingCategory, setDeletingCategory] =
    useState<CategorySummary | null>(null)
  const [replacementCategory, setReplacementCategory] =
    useState(DEFAULT_CATEGORY)
  const [deleteCatError, setDeleteCatError] = useState<string | null>(null)

  const [tagDialogOpen, setTagDialogOpen] = useState(false)
  const [editingTag, setEditingTag] = useState<TagSummary | null>(null)
  const [tagName, setTagName] = useState("")
  const [tagFormError, setTagFormError] = useState<string | null>(null)

  const [deletingTag, setDeletingTag] = useState<TagSummary | null>(null)
  const [deleteTagError, setDeleteTagError] = useState<string | null>(null)

  const invalidateAfterCategoryChange = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["categories"] }),
      queryClient.invalidateQueries({ queryKey: ["transactions"] }),
      queryClient.invalidateQueries({ queryKey: ["spending"] }),
    ])
  }

  const invalidateAfterTagChange = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["tags"] }),
      queryClient.invalidateQueries({ queryKey: ["transactions"] }),
    ])
  }

  const saveCategoryMutation = useMutation({
    mutationFn: async (input: { id?: string; name: string; icon: string }) => {
      if (input.id) {
        return updateCategoryApi({
          id: input.id,
          name: input.name,
          icon: input.icon,
        })
      }
      return createCategoryApi({ name: input.name, icon: input.icon })
    },
    onSuccess: async () => {
      await invalidateAfterCategoryChange()
      setCatDialogOpen(false)
      setEditingCategory(null)
      setCatFormError(null)
    },
    onError: (err) => {
      setCatFormError(getErrorMessage(err, "Failed to save category."))
    },
  })

  const deleteCategoryMutation = useMutation({
    mutationFn: deleteCategoryApi,
    onSuccess: async () => {
      await invalidateAfterCategoryChange()
      setDeletingCategory(null)
      setDeleteCatError(null)
    },
    onError: (err) => {
      setDeleteCatError(getErrorMessage(err, "Failed to delete category."))
    },
  })

  const saveTagMutation = useMutation({
    mutationFn: async (input: { id?: string; name: string }) => {
      if (input.id) {
        return updateTagApi({ id: input.id, name: input.name })
      }
      return createTagApi({ name: input.name })
    },
    onSuccess: async () => {
      await invalidateAfterTagChange()
      setTagDialogOpen(false)
      setEditingTag(null)
      setTagFormError(null)
    },
    onError: (err) => {
      setTagFormError(getErrorMessage(err, "Failed to save tag."))
    },
  })

  const deleteTagMutation = useMutation({
    mutationFn: deleteTagApi,
    onSuccess: async () => {
      await invalidateAfterTagChange()
      setDeletingTag(null)
      setDeleteTagError(null)
    },
    onError: (err) => {
      setDeleteTagError(getErrorMessage(err, "Failed to delete tag."))
    },
  })

  const openCreateCategory = () => {
    setEditingCategory(null)
    setCatName("")
    setCatIcon("Tag")
    setCatFormError(null)
    saveCategoryMutation.reset()
    setCatDialogOpen(true)
  }

  const openEditCategory = (cat: CategorySummary) => {
    setEditingCategory(cat)
    setCatName(cat.name)
    setCatIcon(cat.icon || "Tag")
    setCatFormError(null)
    saveCategoryMutation.reset()
    setCatDialogOpen(true)
  }

  const openDeleteCategory = (cat: CategorySummary) => {
    setDeletingCategory(cat)
    setReplacementCategory(DEFAULT_CATEGORY)
    setDeleteCatError(null)
    deleteCategoryMutation.reset()
  }

  const openCreateTag = () => {
    setEditingTag(null)
    setTagName("")
    setTagFormError(null)
    saveTagMutation.reset()
    setTagDialogOpen(true)
  }

  const openEditTag = (tag: TagSummary) => {
    setEditingTag(tag)
    setTagName(tag.name)
    setTagFormError(null)
    saveTagMutation.reset()
    setTagDialogOpen(true)
  }

  const openDeleteTag = (tag: TagSummary) => {
    setDeletingTag(tag)
    setDeleteTagError(null)
    deleteTagMutation.reset()
  }

  const handleCategorySubmit = (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault()
    const trimmed = catName.trim()
    if (!trimmed) {
      setCatFormError("Category name is required.")
      return
    }
    setCatFormError(null)
    saveCategoryMutation.mutate({
      id: editingCategory?.id,
      name: trimmed,
      icon: catIcon,
    })
  }

  const handleTagSubmit = (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault()
    const trimmed = tagName.trim()
    if (!trimmed) {
      setTagFormError("Tag name is required.")
      return
    }
    setTagFormError(null)
    saveTagMutation.mutate({
      id: editingTag?.id,
      name: trimmed,
    })
  }

  const replacementItems = useMemo(() => {
    const filtered = categories.filter(
      (cat) => !deletingCategory || cat.id !== deletingCategory.id
    )
    const items = filtered.map((cat) => ({ value: cat.name, label: cat.name }))
    if (!items.some((item) => item.value === DEFAULT_CATEGORY)) {
      items.push({ value: DEFAULT_CATEGORY, label: DEFAULT_CATEGORY })
    }
    if (
      replacementCategory &&
      !items.some((item) => item.value === replacementCategory)
    ) {
      items.push({ value: replacementCategory, label: replacementCategory })
    }
    return items
  }, [categories, deletingCategory, replacementCategory])

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Categories &amp; Tags
          </h1>
          <p className="text-sm text-muted-foreground">
            Manage transaction categories used by AI classification and manual
            tags for custom filtering.
          </p>
        </div>

        {tab === "categories" ? (
          <Button type="button" size="sm" onClick={openCreateCategory}>
            <PlusIcon className="mr-1.5 size-4" />
            New category
          </Button>
        ) : (
          <Button type="button" size="sm" onClick={openCreateTag}>
            <PlusIcon className="mr-1.5 size-4" />
            New tag
          </Button>
        )}
      </div>

      <div
        role="tablist"
        aria-label="Categories and Tags sections"
        className="flex border-b border-border"
      >
        <button
          type="button"
          role="tab"
          aria-selected={tab === "categories"}
          onClick={() => void setTab("categories")}
          className={cn(
            "-mb-px flex items-center gap-2 border-b-2 px-5 py-2.5 text-sm font-medium transition-colors",
            tab === "categories"
              ? "border-primary text-foreground"
              : "border-transparent text-muted-foreground hover:border-border hover:text-foreground"
          )}
        >
          <FolderKanbanIcon className="size-4" />
          <span>Categories</span>
          <Badge variant="secondary" className="ml-0.5 text-xs">
            {catQuery.isLoading ? "…" : categories.length}
          </Badge>
        </button>

        <button
          type="button"
          role="tab"
          aria-selected={tab === "tags"}
          onClick={() => void setTab("tags")}
          className={cn(
            "-mb-px flex items-center gap-2 border-b-2 px-5 py-2.5 text-sm font-medium transition-colors",
            tab === "tags"
              ? "border-primary text-foreground"
              : "border-transparent text-muted-foreground hover:border-border hover:text-foreground"
          )}
        >
          <TagIcon className="size-4" />
          <span>Tags</span>
          <Badge variant="secondary" className="ml-0.5 text-xs">
            {tagQuery.isLoading ? "…" : tags.length}
          </Badge>
        </button>
      </div>

      {tab === "categories" ? (
        <Card>
          <CardHeader>
            <CardTitle>Categories</CardTitle>
            <CardDescription>
              Each transaction belongs to one category. Deleting a category
              reassigns its transactions to a replacement category.
            </CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {catQuery.isLoading ? (
              <div className="space-y-2 p-6">
                {Array.from({ length: 5 }).map((_, idx) => (
                  <Skeleton key={idx} className="h-12 w-full" />
                ))}
              </div>
            ) : catQuery.isError ? (
              <div className="p-6">
                <Alert variant="destructive">
                  <AlertDescription>
                    {getErrorMessage(
                      catQuery.error,
                      "Unable to load categories."
                    )}
                  </AlertDescription>
                </Alert>
              </div>
            ) : categories.length === 0 ? (
              <div className="p-6 text-sm text-muted-foreground">
                No categories found.
              </div>
            ) : (
              <div className="divide-y divide-border border-t border-border">
                {categories.map((cat) => {
                  const isSystem = cat.is_system === 1
                  return (
                    <div
                      key={cat.id}
                      className="flex flex-wrap items-center justify-between gap-3 px-6 py-3"
                    >
                      <div className="flex min-w-0 items-center gap-3">
                        <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted/40">
                          <DynamicIcon
                            name={cat.icon}
                            className="size-4 text-foreground"
                          />
                        </div>
                        <div className="flex min-w-0 flex-wrap items-center gap-2">
                          <span className="font-medium text-foreground">
                            {cat.name}
                          </span>
                          {isSystem ? (
                            <Badge variant="secondary">System default</Badge>
                          ) : null}
                        </div>
                      </div>

                      <div className="flex items-center gap-2">
                        <Link
                          href={`/transactions?category=${encodeURIComponent(cat.name)}&range=all`}
                          className={buttonVariants({
                            variant: "ghost",
                            size: "xs",
                          })}
                        >
                          {formatTxCount(cat.transaction_count)}
                        </Link>

                        {isSystem ? (
                          <span className="px-2 text-xs text-muted-foreground">
                            Read-only
                          </span>
                        ) : (
                          <>
                            <Button
                              type="button"
                              variant="outline"
                              size="xs"
                              onClick={() => openEditCategory(cat)}
                            >
                              <PencilIcon className="mr-1 size-3" />
                              Edit
                            </Button>
                            <Button
                              type="button"
                              variant="outline"
                              size="xs"
                              onClick={() => openDeleteCategory(cat)}
                            >
                              <Trash2Icon className="mr-1 size-3" />
                              Delete
                            </Button>
                          </>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>
            )}
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader>
            <CardTitle>Tags</CardTitle>
            <CardDescription>
              Tags are manual labels you can attach across multiple transactions
              for flexible tracking.
            </CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {tagQuery.isLoading ? (
              <div className="space-y-2 p-6">
                {Array.from({ length: 4 }).map((_, idx) => (
                  <Skeleton key={idx} className="h-12 w-full" />
                ))}
              </div>
            ) : tagQuery.isError ? (
              <div className="p-6">
                <Alert variant="destructive">
                  <AlertDescription>
                    {getErrorMessage(tagQuery.error, "Unable to load tags.")}
                  </AlertDescription>
                </Alert>
              </div>
            ) : tags.length === 0 ? (
              <div className="border-t border-border px-6 py-10 text-center">
                <p className="text-sm font-medium text-foreground">
                  No tags yet
                </p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Create a tag here or add one inline while creating or editing
                  a transaction.
                </p>
                <Button
                  type="button"
                  size="sm"
                  className="mt-4"
                  onClick={openCreateTag}
                >
                  <PlusIcon className="mr-1.5 size-4" />
                  New tag
                </Button>
              </div>
            ) : (
              <div className="divide-y divide-border border-t border-border">
                {tags.map((tag) => (
                  <div
                    key={tag.id}
                    className="flex flex-wrap items-center justify-between gap-3 px-6 py-3"
                  >
                    <div className="flex min-w-0 items-center gap-3">
                      <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted/40">
                        <TagIcon className="size-4 text-muted-foreground" />
                      </div>
                      <span className="font-medium text-foreground">
                        {tag.name}
                      </span>
                    </div>

                    <div className="flex items-center gap-2">
                      <Link
                        href={`/transactions?tag=${encodeURIComponent(tag.name)}&range=all`}
                        className={buttonVariants({
                          variant: "ghost",
                          size: "xs",
                        })}
                      >
                        {formatTxCount(tag.transaction_count)}
                      </Link>

                      <Button
                        type="button"
                        variant="outline"
                        size="xs"
                        onClick={() => openEditTag(tag)}
                      >
                        <PencilIcon className="mr-1 size-3" />
                        Rename
                      </Button>
                      <Button
                        type="button"
                        variant="outline"
                        size="xs"
                        onClick={() => openDeleteTag(tag)}
                      >
                        <Trash2Icon className="mr-1 size-3" />
                        Delete
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      )}

      <Dialog open={catDialogOpen} onOpenChange={setCatDialogOpen}>
        <DialogContent className="sm:max-w-lg">
          <form onSubmit={handleCategorySubmit} className="space-y-4">
            <DialogHeader>
              <DialogTitle>
                {editingCategory ? "Edit category" : "New category"}
              </DialogTitle>
              <DialogDescription>
                {editingCategory
                  ? "Renaming updates existing transactions and email extractions using this category."
                  : "Create a category for organizing and auto-classifying expenses."}
              </DialogDescription>
            </DialogHeader>

            {catFormError ? (
              <Alert variant="destructive">
                <AlertDescription>{catFormError}</AlertDescription>
              </Alert>
            ) : null}

            <Field>
              <FieldLabel htmlFor="category-name-input">Name</FieldLabel>
              <Input
                id="category-name-input"
                value={catName}
                onChange={(e) => setCatName(e.target.value)}
                placeholder="e.g. Subscriptions"
                autoFocus
              />
            </Field>

            <Field>
              <div className="flex items-center justify-between">
                <FieldLabel>Icon</FieldLabel>
                <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
                  <DynamicIcon name={catIcon} className="size-3.5" />
                  {catIcon}
                </span>
              </div>
              <div
                className="grid max-h-52 grid-cols-6 gap-1.5 overflow-y-auto rounded-md border p-2 sm:grid-cols-8"
                role="radiogroup"
                aria-label="Category icon"
              >
                {CATEGORY_ICON_OPTIONS.map((opt) => {
                  const selected = catIcon === opt.name
                  return (
                    <button
                      key={opt.name}
                      type="button"
                      role="radio"
                      aria-checked={selected}
                      aria-label={opt.label}
                      title={opt.label}
                      onClick={() => setCatIcon(opt.name)}
                      className={cn(
                        "flex h-9 items-center justify-center rounded-md border transition-colors",
                        selected
                          ? "border-primary bg-primary/10 text-primary"
                          : "border-transparent text-muted-foreground hover:bg-muted hover:text-foreground"
                      )}
                    >
                      <DynamicIcon name={opt.name} className="size-4" />
                    </button>
                  )
                })}
              </div>
            </Field>

            <DialogFooter>
              <DialogClose render={<Button type="button" variant="outline" />}>
                Cancel
              </DialogClose>
              <Button type="submit" disabled={saveCategoryMutation.isPending}>
                {saveCategoryMutation.isPending
                  ? "Saving…"
                  : editingCategory
                    ? "Save changes"
                    : "Create category"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(deletingCategory)}
        onOpenChange={(open) => {
          if (!open) setDeletingCategory(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete category</DialogTitle>
            <DialogDescription>
              {deletingCategory
                ? deletingCategory.transaction_count > 0
                  ? `"${deletingCategory.name}" has ${formatTxCount(deletingCategory.transaction_count)}. Choose a replacement category before deleting.`
                  : `Delete "${deletingCategory.name}"? Any matching rules or records will fall back to the selected replacement category.`
                : ""}
            </DialogDescription>
          </DialogHeader>

          {deleteCatError ? (
            <Alert variant="destructive">
              <AlertDescription>{deleteCatError}</AlertDescription>
            </Alert>
          ) : null}

          <Field>
            <FieldLabel>Replacement category</FieldLabel>
            <Select
              items={replacementItems}
              value={replacementCategory}
              onValueChange={(v) =>
                setReplacementCategory(v ?? DEFAULT_CATEGORY)
              }
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {replacementItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" />}>
              Cancel
            </DialogClose>
            <Button
              type="button"
              variant="destructive"
              disabled={deleteCategoryMutation.isPending || !deletingCategory}
              onClick={() => {
                if (!deletingCategory) return
                deleteCategoryMutation.mutate({
                  id: deletingCategory.id,
                  replaceWithCategory: replacementCategory,
                })
              }}
            >
              {deleteCategoryMutation.isPending
                ? "Deleting…"
                : "Delete category"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={tagDialogOpen} onOpenChange={setTagDialogOpen}>
        <DialogContent>
          <form onSubmit={handleTagSubmit} className="space-y-4">
            <DialogHeader>
              <DialogTitle>{editingTag ? "Rename tag" : "New tag"}</DialogTitle>
              <DialogDescription>
                {editingTag
                  ? "Renaming updates this tag across all transactions using it."
                  : "Create a manual tag to group transactions across categories."}
              </DialogDescription>
            </DialogHeader>

            {tagFormError ? (
              <Alert variant="destructive">
                <AlertDescription>{tagFormError}</AlertDescription>
              </Alert>
            ) : null}

            <Field>
              <FieldLabel htmlFor="tag-name-input">Tag name</FieldLabel>
              <Input
                id="tag-name-input"
                value={tagName}
                onChange={(e) => setTagName(e.target.value)}
                placeholder="e.g. Reimbursable"
                autoFocus
              />
            </Field>

            <DialogFooter>
              <DialogClose render={<Button type="button" variant="outline" />}>
                Cancel
              </DialogClose>
              <Button type="submit" disabled={saveTagMutation.isPending}>
                {saveTagMutation.isPending
                  ? "Saving…"
                  : editingTag
                    ? "Save changes"
                    : "Create tag"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={Boolean(deletingTag)}
        onOpenChange={(open) => {
          if (!open) setDeletingTag(null)
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete tag</DialogTitle>
            <DialogDescription>
              {deletingTag
                ? deletingTag.transaction_count > 0
                  ? `Delete tag "${deletingTag.name}"? It will be removed from ${formatTxCount(deletingTag.transaction_count)}, and those transactions will remain intact.`
                  : `Delete tag "${deletingTag.name}"?`
                : ""}
            </DialogDescription>
          </DialogHeader>

          {deleteTagError ? (
            <Alert variant="destructive">
              <AlertDescription>{deleteTagError}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <DialogClose render={<Button type="button" variant="outline" />}>
              Cancel
            </DialogClose>
            <Button
              type="button"
              variant="destructive"
              disabled={deleteTagMutation.isPending || !deletingTag}
              onClick={() => {
                if (!deletingTag) return
                deleteTagMutation.mutate(deletingTag.id)
              }}
            >
              {deleteTagMutation.isPending ? "Deleting…" : "Delete tag"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
