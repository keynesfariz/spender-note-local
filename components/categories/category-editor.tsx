import { IconTooltip } from "@/components/icon-tooltip"
import { useMutation } from "@tanstack/react-query"
import { useId, useMemo, useRef, useState } from "react"
import { CreateFormDrawer } from "@/components/detail-drawer-content"
import { DynamicIcon } from "@/components/dynamic-icon"
import { PaginationControls } from "@/components/pagination-controls"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
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
import {
  createCategoryApi,
  deleteCategoryApi,
  updateCategoryApi,
} from "@/lib/api/categories"
import { type CategorySummary } from "@/lib/api/types.generated"
import { DEFAULT_CATEGORY } from "@/lib/categories"
import {
  resolveCategoryIconName,
  searchCategoryIcons,
} from "@/lib/category-icons"
import {
  formatTxCount,
  getErrorMessage,
  type EditorFocus,
  useTaxonomyInvalidation,
} from "./taxonomy-ui"
import { cn } from "@/lib/utils"

const ICONS_PER_PAGE = 48

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

export function CategoryEditor({
  category: editingCategory,
  trigger,
  fallback,
}: EditorFocus & { category: CategorySummary | null }) {
  const categoryFormId = useId()
  const categoryNameInput = useRef<HTMLInputElement>(null)
  const [open, setOpen] = useState(true)
  const [catName, setCatName] = useState(editingCategory?.name ?? "")
  const [catIcon, setCatIcon] = useState(editingCategory?.icon || "Tag")
  const invalidate = useTaxonomyInvalidation("categories")
  const saveCategoryMutation = useMutation({
    mutationFn: async () => {
      const name = catName.trim()
      if (!name) throw new Error("Category name is required.")
      return editingCategory
        ? updateCategoryApi({
            id: editingCategory.id,
            name,
            icon: catIcon === editingCategory.icon ? undefined : catIcon,
          })
        : createCategoryApi({ name, icon: catIcon })
    },
    onSuccess: async () => {
      await invalidate()
      setOpen(false)
    },
  })
  const catFormError = saveCategoryMutation.error
  return (
    <CreateFormDrawer
      open={open}
      onOpenChange={setOpen}
      title={editingCategory ? "Edit category" : "New category"}
      description={
        editingCategory
          ? "Renaming updates existing transactions and email extractions using this category."
          : "Create a category for organizing and auto-classifying expenses."
      }
      formId={categoryFormId}
      onSubmit={(event) => {
        event.preventDefault()
        if (!saveCategoryMutation.isPending) saveCategoryMutation.mutate()
      }}
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
        return trigger.isConnected ? trigger : fallback.current
      }}
    >
      {catFormError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {getErrorMessage(catFormError, "Failed to save category.")}
          </AlertDescription>
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
    </CreateFormDrawer>
  )
}

export function DeleteCategoryDialog({
  category: deletingCategory,
  categories,
}: {
  category: CategorySummary
  categories: CategorySummary[]
}) {
  const [open, setOpen] = useState(true)
  const [replacementCategory, setReplacementCategory] =
    useState(DEFAULT_CATEGORY)
  const invalidate = useTaxonomyInvalidation("categories")
  const deleteCategoryMutation = useMutation({
    mutationFn: deleteCategoryApi,
    onSuccess: async () => {
      await invalidate()
      setOpen(false)
    },
  })
  const deleteCatError = deleteCategoryMutation.error
  const replacementItems = useMemo(() => {
    const items = categories.flatMap((cat) =>
      cat.id === deletingCategory.id
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

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete category</DialogTitle>
          <DialogDescription>
            {deletingCategory.transaction_count > 0
              ? `"${deletingCategory.name}" has ${formatTxCount(deletingCategory.transaction_count)}. Choose a replacement category before deleting.`
              : `Delete "${deletingCategory.name}"? Any matching rules or records will fall back to the selected replacement category.`}
          </DialogDescription>
        </DialogHeader>

        {deleteCatError ? (
          <Alert variant="destructive">
            <AlertDescription>
              {getErrorMessage(deleteCatError, "Failed to delete category.")}
            </AlertDescription>
          </Alert>
        ) : null}

        <Field>
          <FieldLabel>Replacement category</FieldLabel>
          <Select
            items={replacementItems}
            value={replacementCategory}
            onValueChange={(v) => setReplacementCategory(v ?? DEFAULT_CATEGORY)}
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
            disabled={deleteCategoryMutation.isPending}
            onClick={() => {
              deleteCategoryMutation.mutate({
                id: deletingCategory.id,
                replaceWithCategory: replacementCategory,
              })
            }}
          >
            {deleteCategoryMutation.isPending ? "Deleting…" : "Delete category"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
