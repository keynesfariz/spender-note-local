"use client"

import { IconTooltip } from "@/components/icon-tooltip"
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
import {
  memo,
  useCallback,
  useId,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
  type SubmitEvent,
} from "react"

import { CreateFormDrawer } from "@/components/detail-drawer-content"
import { DynamicIcon } from "@/components/dynamic-icon"
import { PageLayout } from "@/components/PageLayout"
import { PaginationControls } from "@/components/pagination-controls"
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import {
  categoriesQuery,
  createCategoryApi,
  deleteCategoryApi,
  updateCategoryApi,
} from "@/lib/api/categories"
import {
  createTagApi,
  deleteTagApi,
  tagsQuery,
  updateTagApi,
} from "@/lib/api/tags"
import {
  type CategorySummary,
  type TagSummary,
} from "@/lib/api/types.generated"
import { DEFAULT_CATEGORY } from "@/lib/categories"
import {
  resolveCategoryIconName,
  searchCategoryIcons,
} from "@/lib/category-icons"
import { cn } from "@/lib/utils"

const EMPTY_CATEGORIES: CategorySummary[] = []
const EMPTY_TAGS: TagSummary[] = []

const TAB_VALUES = ["categories", "tags"] as const
const ICONS_PER_PAGE = 48

function formatTxCount(count: number): string {
  return `${count} transaction${count === 1 ? "" : "s"}`
}

function getErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback
}

function CategoryIconPicker({
  value,
  onChange,
}: {
  value: string
  onChange: (value: string) => void
}) {
  const [iconSearch, setIconSearch] = useState("")
  const [iconPage, setIconPage] = useState(1)
  const iconGrid = useRef<HTMLDivElement>(null)
  const matchingIcons = useMemo(
    () => searchCategoryIcons(iconSearch),
    [iconSearch]
  )
  const iconTotalPages = Math.ceil(matchingIcons.length / ICONS_PER_PAGE)
  const visibleIcons = matchingIcons.slice(
    (iconPage - 1) * ICONS_PER_PAGE,
    iconPage * ICONS_PER_PAGE
  )
  const selectedIcon = resolveCategoryIconName(value)

  function changeIconPage(page: number) {
    setIconPage(page)
    if (iconGrid.current) iconGrid.current.scrollTop = 0
  }

  return (
    <Field>
      <div className="flex items-center justify-between">
        <FieldLabel>Icon</FieldLabel>
        <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          <DynamicIcon name={value} className="size-3.5" />
          {value}
        </span>
      </div>
      <Input
        type="search"
        aria-label="Search icons"
        placeholder="Search icons"
        value={iconSearch}
        onChange={(e) => {
          setIconSearch(e.target.value)
          changeIconPage(1)
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.nativeEvent.isComposing) {
            e.preventDefault()
          }
        }}
      />
      {visibleIcons.length > 0 ? (
        <div
          ref={iconGrid}
          className="grid max-h-72 grid-cols-6 gap-1.5 overflow-y-auto rounded-md border p-2 sm:grid-cols-8"
          role="radiogroup"
          aria-label="Category icon"
        >
          {visibleIcons.map((name) => {
            const label = name.replaceAll("-", " ")
            const checked = selectedIcon === name
            return (
              <IconTooltip key={name} label={label}>
                <label
                  className={cn(
                    "relative flex h-9 cursor-pointer items-center justify-center rounded-md border transition-colors focus-within:ring-2 focus-within:ring-ring",
                    checked
                      ? "border-primary bg-primary/10 text-primary"
                      : "border-transparent text-muted-foreground hover:bg-muted hover:text-foreground"
                  )}
                >
                  <input
                    type="radio"
                    name="category-icon"
                    value={name}
                    checked={checked}
                    onChange={() => onChange(name)}
                    aria-label={label}
                    className="sr-only"
                  />
                  <DynamicIcon name={name} />
                </label>
              </IconTooltip>
            )
          })}
        </div>
      ) : (
        <p role="status" className="text-sm text-muted-foreground">
          No icons match your search.
        </p>
      )}
      {iconTotalPages > 1 && (
        <div className="flex flex-wrap items-center justify-between gap-2 text-sm text-muted-foreground">
          <PaginationControls
            page={iconPage}
            totalPages={iconTotalPages}
            isLoading={false}
            onPageChange={changeIconPage}
          />
        </div>
      )}
    </Field>
  )
}

export function CategoriesTagsView() {
  const queryClient = useQueryClient()
  const [tab, setTab] = useQueryState(
    "tab",
    parseAsStringLiteral(TAB_VALUES).withDefault("categories")
  )

  const catQuery = useQuery(categoriesQuery())
  const tagQuery = useQuery(tagsQuery())

  const categories = catQuery.data ?? EMPTY_CATEGORIES
  const tags = tagQuery.data ?? EMPTY_TAGS

  const categoryFormId = useId()
  const categoryNameInput = useRef<HTMLInputElement>(null)
  const newCategoryButton = useRef<HTMLButtonElement>(null)
  const categoryFormTrigger = useRef<HTMLButtonElement>(null)
  const [categoryFormOpen, setCategoryFormOpen] = useState(false)
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

  const tagFormId = useId()
  const tagNameInput = useRef<HTMLInputElement>(null)
  const newTagButton = useRef<HTMLButtonElement>(null)
  const tagFormTrigger = useRef<HTMLButtonElement>(null)
  const [tagFormOpen, setTagFormOpen] = useState(false)
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
      queryClient.invalidateQueries({ queryKey: ["spending"] }),
    ])
  }

  const saveCategoryMutation = useMutation({
    mutationFn: (
      input:
        | { name: string; icon: string }
        | { id: string; name: string; icon?: string }
    ) => ("id" in input ? updateCategoryApi(input) : createCategoryApi(input)),
    onSuccess: invalidateAfterCategoryChange,
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
    onSuccess: invalidateAfterTagChange,
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

  const resetCategoryForm = saveCategoryMutation.reset
  const openCategoryForm = useCallback(
    (category: CategorySummary | null, trigger: HTMLButtonElement) => {
      categoryFormTrigger.current = trigger
      setEditingCategory(category)
      setCatName(category?.name ?? "")
      setCatIcon(category?.icon || "Tag")
      setCatFormError(null)
      resetCategoryForm()
      setCategoryFormOpen(true)
    },
    [resetCategoryForm]
  )

  const resetDeleteCategory = deleteCategoryMutation.reset
  const openDeleteCategory = useCallback(
    (cat: CategorySummary) => {
      setDeletingCategory(cat)
      setReplacementCategory(DEFAULT_CATEGORY)
      setDeleteCatError(null)
      resetDeleteCategory()
    },
    [resetDeleteCategory]
  )

  const resetTagForm = saveTagMutation.reset
  const openTagForm = useCallback(
    (tag: TagSummary | null, trigger: HTMLButtonElement) => {
      tagFormTrigger.current = trigger
      setEditingTag(tag)
      setTagName(tag?.name ?? "")
      setTagFormError(null)
      resetTagForm()
      setTagFormOpen(true)
    },
    [resetTagForm]
  )

  const openCreateTag = (event: MouseEvent<HTMLButtonElement>) =>
    openTagForm(null, event.currentTarget)

  const resetDeleteTag = deleteTagMutation.reset
  const openDeleteTag = useCallback(
    (tag: TagSummary) => {
      setDeletingTag(tag)
      setDeleteTagError(null)
      resetDeleteTag()
    },
    [resetDeleteTag]
  )

  const handleCategorySubmit = (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault()
    const trimmed = catName.trim()
    if (!trimmed) {
      setCatFormError("Category name is required.")
      return
    }
    setCatFormError(null)
    saveCategoryMutation.mutate(
      editingCategory
        ? {
            id: editingCategory.id,
            name: trimmed,
            icon: catIcon === editingCategory.icon ? undefined : catIcon,
          }
        : { name: trimmed, icon: catIcon },
      {
        onSuccess: () => setCategoryFormOpen(false),
        onError: (err) => {
          setCatFormError(getErrorMessage(err, "Failed to save category."))
        },
      }
    )
  }

  const handleTagSubmit = (e: SubmitEvent<HTMLFormElement>) => {
    e.preventDefault()
    const trimmed = tagName.trim()
    if (!trimmed) {
      setTagFormError("Tag name is required.")
      return
    }
    setTagFormError(null)
    saveTagMutation.mutate(
      { id: editingTag?.id, name: trimmed },
      {
        onSuccess: () => setTagFormOpen(false),
        onError: (err) => {
          setTagFormError(getErrorMessage(err, "Failed to save tag."))
        },
      }
    )
  }

  const replacementItems = useMemo(() => {
    const items = categories.flatMap((cat) =>
      cat.id === deletingCategory?.id
        ? []
        : [{ value: cat.name, label: cat.name }]
    )
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

  const categoryFields = (
    <>
      {catFormError ? (
        <Alert variant="destructive">
          <AlertDescription>{catFormError}</AlertDescription>
        </Alert>
      ) : null}

      <Field>
        <FieldLabel htmlFor={`${categoryFormId}-name`}>Name</FieldLabel>
        <Input
          ref={categoryNameInput}
          id={`${categoryFormId}-name`}
          value={catName}
          onChange={(e) => setCatName(e.target.value)}
          placeholder="e.g. Subscriptions"
        />
      </Field>

      <CategoryIconPicker value={catIcon} onChange={setCatIcon} />
    </>
  )

  const tagFields = (
    <>
      {tagFormError ? (
        <Alert variant="destructive">
          <AlertDescription>{tagFormError}</AlertDescription>
        </Alert>
      ) : null}

      <Field>
        <FieldLabel htmlFor={`${tagFormId}-name`}>Tag name</FieldLabel>
        <Input
          ref={tagNameInput}
          id={`${tagFormId}-name`}
          value={tagName}
          onChange={(e) => setTagName(e.target.value)}
          placeholder="e.g. Reimbursable"
        />
      </Field>
    </>
  )

  return (
    <PageLayout
      title="Categories & Tags"
      subtitle="Manage transaction categories used by AI classification and manual
            tags for custom filtering."
      actions={
        tab === "categories" ? (
          <Button
            ref={newCategoryButton}
            type="button"
            size="sm"
            aria-haspopup="dialog"
            onClick={(event) => openCategoryForm(null, event.currentTarget)}
          >
            <PlusIcon className="mr-1.5 size-4" />
            New category
          </Button>
        ) : (
          <Button
            ref={newTagButton}
            type="button"
            size="sm"
            aria-haspopup="dialog"
            onClick={openCreateTag}
          >
            <PlusIcon className="mr-1.5 size-4" />
            New tag
          </Button>
        )
      }
    >
      <Tabs
        className="gap-4"
        value={tab}
        onValueChange={(value) => {
          if (value === "categories" || value === "tags") void setTab(value)
        }}
      >
        <TabsList
          variant="line"
          className="max-w-full overflow-x-auto pb-1.5"
          aria-label="Categories and Tags sections"
        >
          <TabsTrigger value="categories">
            <FolderKanbanIcon className="size-4" />
            Categories
            <Badge variant="secondary" className="text-xs">
              {catQuery.isLoading ? "…" : categories.length}
            </Badge>
          </TabsTrigger>
          <TabsTrigger value="tags">
            <TagIcon className="size-4" />
            Tags
            <Badge variant="secondary" className="text-xs">
              {tagQuery.isLoading ? "…" : tags.length}
            </Badge>
          </TabsTrigger>
        </TabsList>
        <TabsContent value="categories">
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
                <CategoryRows
                  items={categories}
                  onEdit={openCategoryForm}
                  onDelete={openDeleteCategory}
                />
              )}
            </CardContent>
          </Card>
        </TabsContent>
        <TabsContent value="tags">
          <Card>
            <CardHeader>
              <CardTitle>Tags</CardTitle>
              <CardDescription>
                Tags are manual labels you can attach across multiple
                transactions for flexible tracking.
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
                    Create a tag here or add one inline while creating or
                    editing a transaction.
                  </p>
                  <Button
                    type="button"
                    size="sm"
                    className="mt-4"
                    aria-haspopup="dialog"
                    onClick={openCreateTag}
                  >
                    <PlusIcon className="mr-1.5 size-4" />
                    New tag
                  </Button>
                </div>
              ) : (
                <TagRows
                  items={tags}
                  onEdit={openTagForm}
                  onDelete={openDeleteTag}
                />
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <CreateFormDrawer
        open={categoryFormOpen}
        onOpenChange={setCategoryFormOpen}
        title={editingCategory ? "Edit category" : "New category"}
        description={
          editingCategory
            ? "Renaming updates existing transactions and email extractions using this category."
            : "Create a category for organizing and auto-classifying expenses."
        }
        formId={categoryFormId}
        onSubmit={handleCategorySubmit}
        submitLabel={
          saveCategoryMutation.isPending
            ? "Saving…"
            : editingCategory
              ? "Save changes"
              : "Create category"
        }
        submitDisabled={saveCategoryMutation.isPending}
        initialFocus={categoryNameInput}
        finalFocus={() => {
          const opener = categoryFormTrigger.current
          return opener?.isConnected ? opener : newCategoryButton.current
        }}
      >
        {categoryFields}
      </CreateFormDrawer>

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

      <Dialog open={tagFormOpen} onOpenChange={setTagFormOpen}>
        <DialogContent
          initialFocus={tagNameInput}
          finalFocus={() => {
            const opener = tagFormTrigger.current
            return opener?.isConnected ? opener : newTagButton.current
          }}
        >
          <form onSubmit={handleTagSubmit} className="space-y-4">
            <DialogHeader>
              <DialogTitle>{editingTag ? "Rename tag" : "New tag"}</DialogTitle>
              <DialogDescription>
                {editingTag
                  ? "Renaming updates this tag across all transactions using it."
                  : "Create a manual tag to group transactions across categories."}
              </DialogDescription>
            </DialogHeader>

            {tagFields}

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
    </PageLayout>
  )
}

const CategoryRows = memo(function CategoryRows({
  items,
  onEdit,
  onDelete,
}: {
  items: CategorySummary[]
  onEdit: (item: CategorySummary, trigger: HTMLButtonElement) => void
  onDelete: (item: CategorySummary) => void
}) {
  return (
    <div className="divide-y divide-border border-t border-border">
      {items.map((cat) => {
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
                <span className="font-medium text-foreground">{cat.name}</span>
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
                    onClick={(event) => onEdit(cat, event.currentTarget)}
                  >
                    <PencilIcon className="mr-1 size-3" />
                    Edit
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    onClick={() => onDelete(cat)}
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
  )
})

const TagRows = memo(function TagRows({
  items,
  onEdit,
  onDelete,
}: {
  items: TagSummary[]
  onEdit: (item: TagSummary, trigger: HTMLButtonElement) => void
  onDelete: (item: TagSummary) => void
}) {
  return (
    <div className="divide-y divide-border border-t border-border">
      {items.map((tag) => (
        <div
          key={tag.id}
          className="flex flex-wrap items-center justify-between gap-3 px-6 py-3"
        >
          <div className="flex min-w-0 items-center gap-3">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted/40">
              <TagIcon className="size-4 text-muted-foreground" />
            </div>
            <span className="font-medium text-foreground">{tag.name}</span>
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
              onClick={(event) => onEdit(tag, event.currentTarget)}
            >
              <PencilIcon className="mr-1 size-3" />
              Rename
            </Button>
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => onDelete(tag)}
            >
              <Trash2Icon className="mr-1 size-3" />
              Delete
            </Button>
          </div>
        </div>
      ))}
    </div>
  )
})
