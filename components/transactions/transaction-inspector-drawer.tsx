"use client"

import { ChevronLeftIcon, ChevronRightIcon } from "lucide-react"
import { useLayoutEffect, useRef, useState } from "react"

import { DetailDrawerContent } from "@/components/detail-drawer-content"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import { Drawer, DrawerFooter } from "@/components/ui/drawer"
import { type AccountOption } from "@/lib/api/types.generated"
import {
  type NavigationGuard,
  type Review,
  InspectorBody,
} from "./transaction-inspector-form"
import { type ReviewDirection } from "@/lib/transaction-review"

export function TransactionInspectorDrawer({
  review,
  accounts,
  localCurrency,
  timezone,
  onTransactionMutated,
}: {
  review: Review
  accounts: AccountOption[]
  localCurrency: string
  timezone: string | null | undefined
  onTransactionMutated?: () => void
}) {
  const { transactionId } = review
  const navigationGuard = useRef<NavigationGuard>(null)
  const navigation = useRef<HTMLElement>(null)
  const previousTransactionId = useRef(transactionId)
  const [discard, setDiscard] = useState<{
    id: string
    direction: ReviewDirection
  } | null>(null)
  const discardDirection =
    discard?.id === transactionId ? discard.direction : null

  useLayoutEffect(() => {
    const previousId = previousTransactionId.current
    previousTransactionId.current = transactionId
    if (
      transactionId &&
      previousId &&
      transactionId !== previousId &&
      !navigation.current?.contains(document.activeElement)
    ) {
      navigation.current?.focus({ preventScroll: true })
    }
  }, [transactionId])

  function navigate(direction: ReviewDirection) {
    if (!transactionId || navigationGuard.current?.busy) return
    if (navigationGuard.current?.hasDraft) {
      setDiscard({ id: transactionId, direction })
      return
    }
    void review.navigate(direction)
  }

  return (
    <Drawer
      swipeDirection="right"
      open={transactionId !== null}
      onOpenChange={(open) => {
        if (!open) {
          setDiscard(null)
          review.close()
        }
      }}
    >
      <DetailDrawerContent
        title="Transaction details"
        description="Inspect attached source emails, mark reviewed, or edit transaction fields."
        scrollResetKey={transactionId}
        footer={
          <DrawerFooter className="items-center border-t px-5 py-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:px-6">
            <nav
              ref={navigation}
              tabIndex={-1}
              aria-label="Transaction navigation"
              className="flex flex-wrap items-center justify-center gap-2"
            >
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="aria-disabled:opacity-50"
                disabled={!review.previous || review.pending}
                focusableWhenDisabled
                onClick={() => navigate("previous")}
              >
                <ChevronLeftIcon aria-hidden="true" data-icon="inline-start" />
                Previous
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="aria-disabled:opacity-50"
                disabled={!review.next || review.pending}
                focusableWhenDisabled
                onClick={() => navigate("next")}
              >
                Next
                <ChevronRightIcon aria-hidden="true" data-icon="inline-end" />
              </Button>
            </nav>
            {review.error ? (
              <p role="alert" className="text-center text-sm text-destructive">
                {review.error}
              </p>
            ) : null}
          </DrawerFooter>
        }
      >
        {transactionId && (
          <InspectorBody
            key={transactionId}
            transactionId={transactionId}
            accounts={accounts}
            localCurrency={localCurrency}
            timezone={timezone}
            onTransactionMutated={onTransactionMutated}
            review={review}
            navigationGuard={navigationGuard}
          />
        )}
        <AlertDialog
          open={discardDirection !== null}
          onOpenChange={(open) => {
            if (!open) setDiscard(null)
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Discard unsaved changes?</AlertDialogTitle>
              <AlertDialogDescription>
                Your changes to this transaction will be lost if you move on.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Keep editing</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                onClick={() => {
                  setDiscard(null)
                  if (discardDirection) void review.navigate(discardDirection)
                }}
              >
                Discard and move
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </DetailDrawerContent>
    </Drawer>
  )
}
