"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  useCallback,
  useId,
  useRef,
  useState,
  type ReactNode,
  type SubmitEvent,
} from "react"

import { CreateFormDrawer } from "@/components/detail-drawer-content"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { accountsQuery } from "@/lib/api/accounts"
import { transactionInspectorQuery } from "@/lib/api/transaction-inspector"
import {
  saveTransfersApi,
  type TransferMutationItem,
} from "@/lib/api/transactions"
import { invalidateTriageRelatedQueries } from "@/lib/api/triage"
import {
  type Draft,
  type TransferTarget,
  initialDraft,
  buildTransferItem,
} from "./transfer-draft"
import { TransferFields } from "./transfer-fields"

export function TransferFormDialog({
  targets,
  timezone,
  onClose,
  onSuccess,
  trigger,
}: {
  targets: TransferTarget[]
  timezone?: string | null
  onClose?: () => void
  onSuccess?: () => void
  trigger?: ReactNode
}) {
  const formId = useId()
  const queryClient = useQueryClient()
  const [open, setOpen] = useState(!trigger)
  const accounts = useQuery({ ...accountsQuery(), enabled: open })
  const [drafts, setDrafts] = useState(() =>
    trigger ? [] : targets.map((target) => initialDraft(target, timezone))
  )
  const updateDraft = useCallback((index: number, patch: Partial<Draft>) => {
    setDrafts((current) =>
      current.map((item, position) =>
        position === index ? { ...item, ...patch } : item
      )
    )
  }, [])
  const saved = useRef(false)
  const evidenceIds = useRef(new Map<string, string[]>())
  const mutation = useMutation({
    mutationFn: async () => {
      const items = await Promise.all(
        targets.map(async (target, index): Promise<TransferMutationItem> => {
          const draft = drafts[index]
          const tx = target.transaction
          const editing = tx?.kind === "transfer"
          let emailIds = target.skippedEmailId
            ? [target.skippedEmailId]
            : target.emailIds
          if (tx && !editing && !emailIds) {
            emailIds = evidenceIds.current.get(tx.id)
            if (!emailIds) {
              const detail = await queryClient.fetchQuery(
                transactionInspectorQuery(tx.id)
              )
              emailIds = detail.sources.map((source) => source.email_id)
              evidenceIds.current.set(tx.id, emailIds)
            }
          }
          return buildTransferItem(
            target,
            draft,
            accounts.data,
            emailIds,
            timezone
          )
        })
      )
      return saveTransfersApi(items)
    },
    onSuccess: () => {
      invalidateTriageRelatedQueries(queryClient)
      saved.current = true
      setOpen(false)
    },
  })

  function submit(event: SubmitEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!mutation.isPending) mutation.mutate()
  }

  const single = targets.length === 1 ? targets[0] : null
  const title = single?.cardPayment
    ? "Mark as card payment"
    : single?.transaction?.kind === "transfer"
      ? "Edit transfer"
      : single && !single.transaction
        ? "Transfer"
        : "Mark as transfer"

  return (
    <CreateFormDrawer
      open={open}
      trigger={trigger}
      onOpenChange={(nextOpen) => {
        if (mutation.isPending) return
        if (nextOpen && trigger) {
          saved.current = false
          evidenceIds.current.clear()
          setDrafts(targets.map((target) => initialDraft(target, timezone)))
          mutation.reset()
        }
        setOpen(nextOpen)
      }}
      onOpenChangeComplete={(nextOpen) => {
        if (nextOpen) return
        if (saved.current) onSuccess?.()
        onClose?.()
      }}
      formId={formId}
      title={title}
      description={`Account balances reflect each transfer. Transfers stay out of net spending. Times use ${timezone || "UTC"}.`}
      onSubmit={submit}
      submitLabel={mutation.isPending ? "Saving…" : "Confirm"}
      submitDisabled={mutation.isPending || !accounts.data}
    >
      {accounts.isPending && (
        <p role="status" className="text-sm">
          Loading accounts…
        </p>
      )}
      {(mutation.isError || accounts.isError) && (
        <Alert variant="destructive">
          <AlertDescription>
            {mutation.error?.message ?? accounts.error?.message}
          </AlertDescription>
        </Alert>
      )}
      {accounts.data &&
        drafts.map((draft, index) => (
          <TransferFields
            key={draft.requestId}
            draft={draft}
            target={targets[index]}
            accounts={accounts.data}
            timezone={timezone}
            disabled={mutation.isPending}
            index={index}
            updateDraft={updateDraft}
          />
        ))}
    </CreateFormDrawer>
  )
}
