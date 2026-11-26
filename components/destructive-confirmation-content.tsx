import type { ReactNode } from "react"

import {
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"

export function DestructiveConfirmationContent({
  title,
  description,
  confirmLabel,
  onConfirm,
  confirmDisabled,
  cancelDisabled,
}: {
  title: ReactNode
  description: ReactNode
  confirmLabel: string
  onConfirm: () => void
  confirmDisabled?: boolean
  cancelDisabled?: boolean
}) {
  return (
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>{title}</AlertDialogTitle>
        <AlertDialogDescription>{description}</AlertDialogDescription>
      </AlertDialogHeader>
      <AlertDialogFooter>
        <AlertDialogCancel disabled={cancelDisabled}>Cancel</AlertDialogCancel>
        <AlertDialogAction
          variant="destructive"
          disabled={confirmDisabled}
          onClick={onConfirm}
        >
          {confirmLabel}
        </AlertDialogAction>
      </AlertDialogFooter>
    </AlertDialogContent>
  )
}
