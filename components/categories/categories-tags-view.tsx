"use client"

import { useQuery } from "@tanstack/react-query"
import { FolderKanbanIcon, PlusIcon, TagIcon } from "lucide-react"
import { parseAsStringLiteral, useQueryState } from "nuqs"
import { useCallback, useRef, useState, type MouseEvent } from "react"

import { PageLayout } from "@/components/PageLayout"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { categoriesQuery } from "@/lib/api/categories"
import { tagsQuery } from "@/lib/api/tags"
import {
  type CategorySummary,
  type TagSummary,
} from "@/lib/api/types.generated"
import { getErrorMessage } from "./taxonomy-ui"
import { CategoryEditor, DeleteCategoryDialog } from "./category-editor"
import { TagEditor, DeleteTagDialog } from "../tags/tag-editor"
import { CategoryRows } from "./category-rows"
import { TagRows } from "../tags/tag-rows"

const EMPTY_CATEGORIES: CategorySummary[] = []
const EMPTY_TAGS: TagSummary[] = []

const TAB_VALUES = ["categories", "tags"] as const

type TaxonomyOperation =
  | {
      kind: "edit-category"
      item: CategorySummary | null
      trigger: HTMLButtonElement
    }
  | { kind: "edit-tag"; item: TagSummary | null; trigger: HTMLButtonElement }
  | { kind: "delete-category"; item: CategorySummary }
  | { kind: "delete-tag"; item: TagSummary }

export function CategoriesTagsView() {
  const [tab, setTab] = useQueryState(
    "tab",
    parseAsStringLiteral(TAB_VALUES).withDefault("categories")
  )

  const catQuery = useQuery(categoriesQuery())
  const tagQuery = useQuery(tagsQuery())

  const categories = catQuery.data ?? EMPTY_CATEGORIES
  const tags = tagQuery.data ?? EMPTY_TAGS

  const newCategoryButton = useRef<HTMLButtonElement>(null)
  const newTagButton = useRef<HTMLButtonElement>(null)
  const [operation, setOperation] = useState<
    (TaxonomyOperation & { key: number }) | null
  >(null)
  const openOperation = useCallback((next: TaxonomyOperation) => {
    setOperation((previous) => ({ ...next, key: (previous?.key ?? 0) + 1 }))
  }, [])
  const openCategoryForm = useCallback(
    (item: CategorySummary | null, trigger: HTMLButtonElement) =>
      openOperation({ kind: "edit-category", item, trigger }),
    [openOperation]
  )
  const openTagForm = useCallback(
    (item: TagSummary | null, trigger: HTMLButtonElement) =>
      openOperation({ kind: "edit-tag", item, trigger }),
    [openOperation]
  )
  const openDeleteCategory = useCallback(
    (item: CategorySummary) => openOperation({ kind: "delete-category", item }),
    [openOperation]
  )
  const openDeleteTag = useCallback(
    (item: TagSummary) => openOperation({ kind: "delete-tag", item }),
    [openOperation]
  )
  const openCreateTag = (event: MouseEvent<HTMLButtonElement>) =>
    openTagForm(null, event.currentTarget)

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

      {operation?.kind === "edit-category" && (
        <CategoryEditor
          key={operation.key}
          category={operation.item}
          trigger={operation.trigger}
          fallback={newCategoryButton}
        />
      )}
      {operation?.kind === "edit-tag" && (
        <TagEditor
          key={operation.key}
          tag={operation.item}
          trigger={operation.trigger}
          fallback={newTagButton}
        />
      )}
      {operation?.kind === "delete-category" && (
        <DeleteCategoryDialog
          key={operation.key}
          category={operation.item}
          categories={categories}
        />
      )}
      {operation?.kind === "delete-tag" && (
        <DeleteTagDialog key={operation.key} tag={operation.item} />
      )}
    </PageLayout>
  )
}
