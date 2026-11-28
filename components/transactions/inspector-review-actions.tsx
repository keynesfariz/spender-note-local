import { CheckIcon, SaveIcon } from "lucide-react"
import { IconTooltip } from "@/components/icon-tooltip"
import { Button } from "@/components/ui/button"
import { ButtonGroup } from "@/components/ui/button-group"
import { ReviewedIcon } from "./reviewed"

export function InspectorReviewActions({
  form,
  canAdvance,
  reviewed,
  saveDisabled,
  onMarkReviewed,
  saving,
}: {
  form?: string
  canAdvance: boolean
  reviewed: boolean
  saveDisabled: boolean
  onMarkReviewed: () => void
  saving: boolean
}) {
  const skipNativeValidation = Boolean(form)
  const hasSecondaryActions = !reviewed || canAdvance
  return (
    <div className="flex items-center gap-2">
      {reviewed ? <ReviewedIcon /> : null}
      {hasSecondaryActions ? (
        <ButtonGroup>
          {!reviewed ? (
            <IconTooltip label="Mark reviewed">
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                aria-label="Mark reviewed"
                disabled={saveDisabled}
                onClick={onMarkReviewed}
              >
                <CheckIcon className="size-4" aria-hidden="true" />
              </Button>
            </IconTooltip>
          ) : null}
          {canAdvance ? (
            <IconTooltip label="Save changes">
              <Button
                type="submit"
                form={form}
                formNoValidate={skipNativeValidation}
                variant="outline"
                size="icon-sm"
                aria-label="Save changes"
                disabled={saveDisabled}
              >
                <SaveIcon className="size-4" aria-hidden="true" />
              </Button>
            </IconTooltip>
          ) : null}
        </ButtonGroup>
      ) : null}
      <Button
        type="submit"
        form={form}
        formNoValidate={skipNativeValidation}
        value={canAdvance ? "save-and-next" : undefined}
        size="sm"
        disabled={saveDisabled}
      >
        {saving
          ? "Saving..."
          : canAdvance
            ? "Save changes & next"
            : "Save changes"}
      </Button>
    </div>
  )
}
