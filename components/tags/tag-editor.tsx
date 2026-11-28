import { useMutation } from "@tanstack/react-query"
import { useId, useRef, useState } from "react"
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
import { createTagApi, deleteTagApi, updateTagApi } from "@/lib/api/tags"
import { type TagSummary } from "@/lib/api/types.generated"
import {
  type EditorFocus,
  formatTxCount,
  getErrorMessage,
  useTaxonomyInvalidation,
} from "../categories/taxonomy-ui"

export function TagEditor({
  tag: editingTag,
  trigger,
  fallback,
}: EditorFocus & { tag: TagSummary | null }) {
  const tagFormId = useId()
  const tagNameInput = useRef<HTMLInputElement>(null)
  const [open, setOpen] = useState(true)
  const [tagName, setTagName] = useState(editingTag?.name ?? "")
  const invalidate = useTaxonomyInvalidation("tags")
  const saveTagMutation = useMutation({
    mutationFn: async () => {
      const name = tagName.trim()
      if (!name) throw new Error("Tag name is required.")
      return editingTag
        ? updateTagApi({ id: editingTag.id, name })
        : createTagApi({ name })
    },
    onSuccess: async () => {
      await invalidate()
      setOpen(false)
    },
  })
  const tagFormError = saveTagMutation.error
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent
        initialFocus={tagNameInput}
        finalFocus={() => {
          return trigger.isConnected ? trigger : fallback.current
        }}
      >
        <form
          onSubmit={(event) => {
            event.preventDefault()
            if (!saveTagMutation.isPending) saveTagMutation.mutate()
          }}
          className="space-y-4"
        >
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
              <AlertDescription>
                {getErrorMessage(tagFormError, "Failed to save tag.")}
              </AlertDescription>
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
  )
}

export function DeleteTagDialog({ tag: deletingTag }: { tag: TagSummary }) {
  const [open, setOpen] = useState(true)
  const invalidate = useTaxonomyInvalidation("tags")
  const deleteTagMutation = useMutation({
    mutationFn: deleteTagApi,
    onSuccess: async () => {
      await invalidate()
      setOpen(false)
    },
  })
  const deleteTagError = deleteTagMutation.error
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete tag</DialogTitle>
          <DialogDescription>
            {deletingTag.transaction_count > 0
              ? `Delete tag "${deletingTag.name}"? It will be removed from ${formatTxCount(deletingTag.transaction_count)}, and those transactions will remain intact.`
              : `Delete tag "${deletingTag.name}"?`}
          </DialogDescription>
        </DialogHeader>

        {deleteTagError ? (
          <Alert variant="destructive">
            <AlertDescription>
              {getErrorMessage(deleteTagError, "Failed to delete tag.")}
            </AlertDescription>
          </Alert>
        ) : null}

        <DialogFooter>
          <DialogClose render={<Button type="button" variant="outline" />}>
            Cancel
          </DialogClose>
          <Button
            type="button"
            variant="destructive"
            disabled={deleteTagMutation.isPending}
            onClick={() => {
              deleteTagMutation.mutate(deletingTag.id)
            }}
          >
            {deleteTagMutation.isPending ? "Deleting…" : "Delete tag"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
