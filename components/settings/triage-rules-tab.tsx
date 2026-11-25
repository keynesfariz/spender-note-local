"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { ChevronDownIcon } from "lucide-react"
import Link from "next/link"
import { useMemo, useState } from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import {
  deleteTriagePolicyApi,
  invalidateTriageRelatedQueries,
  promoteIgnoredEmailApi,
  triageIgnoredEmailsQuery,
  triagePoliciesQuery,
  updateTriagePolicyApi,
  type AppSettingsView,
  type SenderTriagePolicyView,
} from "@/lib/api"
import { formatMinorCurrency, formatUtcInTimezone } from "@/lib/format"

const ALL_SENDERS = "__all__"
const IGNORED_PAGE_SIZE = 25

function policyActionBadgeLabel(action: SenderTriagePolicyView["action"]) {
  return action === "admit" ? "Always import" : "Always ignore"
}

export function TriageRulesTab({
  settings,
}: {
  settings: AppSettingsView | undefined
}) {
  const queryClient = useQueryClient()
  const [senderFilter, setSenderFilter] = useState(ALL_SENDERS)
  const [search, setSearch] = useState("")
  const [page, setPage] = useState(1)
  const [pendingPolicyId, setPendingPolicyId] = useState<string | null>(null)
  const [promotingEmailId, setPromotingEmailId] = useState<string | null>(null)
  const [policyError, setPolicyError] = useState<string | null>(null)
  const [promoteError, setPromoteError] = useState<string | null>(null)
  const [recoveryMessage, setRecoveryMessage] = useState<string | null>(null)

  const policiesQuery = useQuery(triagePoliciesQuery())
  const ignoredQuery = useQuery(
    triageIgnoredEmailsQuery({
      senderEmail: senderFilter === ALL_SENDERS ? undefined : senderFilter,
      search: search.trim() || undefined,
      page,
      pageSize: IGNORED_PAGE_SIZE,
    })
  )

  const availableSenders = ignoredQuery.data?.availableSenders
  const senderSelectItems = useMemo(() => {
    const items = [
      { value: ALL_SENDERS, label: "All senders" },
      ...(availableSenders ?? []).map((sender) => ({
        value: sender,
        label: sender,
      })),
    ]
    if (
      senderFilter !== ALL_SENDERS &&
      !items.some((item) => item.value === senderFilter)
    ) {
      items.push({ value: senderFilter, label: senderFilter })
    }
    return items
  }, [availableSenders, senderFilter])

  const updatePolicyMutation = useMutation({
    mutationFn: (vars: { id: string; action: "admit" | "ignore" }) =>
      updateTriagePolicyApi(vars.id, { action: vars.action }),
    onMutate: (vars) => {
      setPendingPolicyId(vars.id)
      setPolicyError(null)
      setRecoveryMessage(null)
    },
    onSuccess: (updated) => {
      invalidateTriageRelatedQueries(queryClient)
      const restoredCount = updated.restoredGhostCount ?? 0
      if (restoredCount > 0) {
        setRecoveryMessage(
          `Updated rule for ${updated.sender_email}. ${restoredCount} previously ignored ${restoredCount === 1 ? "email is" : "emails are"} now visible on Transactions.`
        )
      }
    },
    onError: (err) => {
      setPolicyError(
        err instanceof Error ? err.message : "Failed to update triage rule."
      )
    },
    onSettled: () => {
      setPendingPolicyId(null)
    },
  })

  const deletePolicyMutation = useMutation({
    mutationFn: deleteTriagePolicyApi,
    onMutate: (id) => {
      setPendingPolicyId(id)
      setPolicyError(null)
      setRecoveryMessage(null)
    },
    onSuccess: (res) => {
      invalidateTriageRelatedQueries(queryClient)
      if (res.restoredGhostCount > 0) {
        setRecoveryMessage(
          `Rule deleted. ${res.restoredGhostCount} previously ignored ${res.restoredGhostCount === 1 ? "email" : "emails"} returned to Transactions for review.`
        )
      }
    },
    onError: (err) => {
      setPolicyError(
        err instanceof Error ? err.message : "Failed to delete triage rule."
      )
    },
    onSettled: () => {
      setPendingPolicyId(null)
    },
  })

  const promoteIgnoredMutation = useMutation({
    mutationFn: promoteIgnoredEmailApi,
    onMutate: (emailId) => {
      setPromotingEmailId(emailId)
      setPromoteError(null)
    },
    onSuccess: () => {
      invalidateTriageRelatedQueries(queryClient)
    },
    onError: (err) => {
      setPromoteError(
        err instanceof Error ? err.message : "Failed to promote ignored email."
      )
    },
    onSettled: () => {
      setPromotingEmailId(null)
    },
  })

  const policies = policiesQuery.data ?? []
  const ignoredItems = ignoredQuery.data?.items ?? []
  const ignoredTotalCount = ignoredQuery.data?.totalCount ?? 0
  const ignoredTotalPages = Math.max(
    1,
    Math.ceil(ignoredTotalCount / IGNORED_PAGE_SIZE)
  )
  const clampedPage = Math.min(page, ignoredTotalPages)
  const isPolicyBusy = pendingPolicyId !== null

  return (
    <div className="space-y-4">
      {recoveryMessage && (
        <Alert>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>{recoveryMessage}</span>
            <div className="flex flex-wrap items-center gap-2">
              <Link
                href="/transactions?range=all"
                className={buttonVariants({ variant: "outline", size: "sm" })}
              >
                View on Transactions
              </Link>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setRecoveryMessage(null)}
              >
                Dismiss
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Learned Sender Policies</CardTitle>
          <CardDescription>
            Per-sender rules learned when you promote or always ignore skipped
            emails on Transactions. Deleting a rule stops automatic triage on
            future emails without removing imported transactions.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {policyError && (
            <Alert variant="destructive">
              <AlertDescription>{policyError}</AlertDescription>
            </Alert>
          )}

          {policiesQuery.isPending ? (
            <div className="space-y-3">
              <Skeleton className="h-16 w-full" />
              <Skeleton className="h-16 w-full" />
            </div>
          ) : policiesQuery.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {policiesQuery.error instanceof Error
                  ? policiesQuery.error.message
                  : "Failed to load learned triage policies."}
              </AlertDescription>
            </Alert>
          ) : policies.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No learned sender rules yet. Promoting or always ignoring a
              skipped row on Transactions saves its pattern here.
            </p>
          ) : (
            <div className="space-y-3">
              {policies.map((policy, idx) => {
                const nextAction =
                  policy.action === "ignore" ? "admit" : "ignore"
                const switchLabel =
                  policy.action === "ignore"
                    ? "Switch to Always import"
                    : "Switch to Always ignore"
                const showIgnoredCount =
                  policy.action === "ignore" || policy.ignored_count > 0

                return (
                  <div key={policy.id}>
                    {idx > 0 && <Separator className="mb-3" />}
                    <div className="flex flex-wrap items-start justify-between gap-3 text-sm">
                      <div className="min-w-0 flex-1 space-y-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-medium text-foreground">
                            {policy.sender_email}
                          </span>
                          <Badge
                            variant={
                              policy.action === "admit"
                                ? "secondary"
                                : "outline"
                            }
                          >
                            {policyActionBadgeLabel(policy.action)}
                          </Badge>
                          {showIgnoredCount && (
                            <Badge variant="outline">
                              {policy.ignored_count} ignored
                            </Badge>
                          )}
                          {policy.ghost_count > 0 && (
                            <Badge variant="outline">
                              {policy.ghost_count} skipped
                            </Badge>
                          )}
                        </div>
                        <p className="font-mono text-xs break-words text-muted-foreground">
                          Subject pattern: {policy.subject_pattern || "(any)"}
                          {policy.body_cue
                            ? ` · Body cue: ${policy.body_cue}`
                            : ""}
                        </p>
                      </div>

                      <div className="flex flex-wrap items-center gap-2">
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={isPolicyBusy}
                          onClick={() =>
                            updatePolicyMutation.mutate({
                              id: policy.id,
                              action: nextAction,
                            })
                          }
                        >
                          {pendingPolicyId === policy.id
                            ? "Updating..."
                            : switchLabel}
                        </Button>

                        <AlertDialog>
                          <AlertDialogTrigger
                            render={
                              <Button
                                type="button"
                                variant="destructive"
                                size="sm"
                                disabled={isPolicyBusy}
                              />
                            }
                          >
                            Delete
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>
                                Delete learned rule for {policy.sender_email}?
                              </AlertDialogTitle>
                              <AlertDialogDescription>
                                Future matching emails will be evaluated without
                                this rule. Existing imported transactions are
                                never deleted.
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel disabled={isPolicyBusy}>
                                Cancel
                              </AlertDialogCancel>
                              <AlertDialogAction
                                variant="destructive"
                                disabled={isPolicyBusy}
                                onClick={() =>
                                  deletePolicyMutation.mutate(policy.id)
                                }
                              >
                                {pendingPolicyId === policy.id
                                  ? "Deleting..."
                                  : "Delete rule"}
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
                      </div>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Ignored Emails</CardTitle>
          <CardDescription>
            Inspect emails suppressed by Always ignore rules and run a one-off
            local recovery without changing the sender rule.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_16rem]">
            <Input
              type="search"
              aria-label="Search ignored emails"
              placeholder="Search sender, subject, merchant, or cue..."
              value={search}
              onChange={(e) => {
                setSearch(e.target.value)
                setPage(1)
              }}
            />

            <Select
              items={senderSelectItems}
              value={senderFilter}
              onValueChange={(value) => {
                setSenderFilter(value ?? ALL_SENDERS)
                setPage(1)
              }}
            >
              <SelectTrigger
                className="w-full"
                aria-label="Filter ignored emails by sender"
              >
                <SelectValue placeholder="All senders" />
              </SelectTrigger>
              <SelectContent>
                {senderSelectItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    {item.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {promoteError && (
            <Alert variant="destructive">
              <AlertDescription>{promoteError}</AlertDescription>
            </Alert>
          )}

          {ignoredQuery.isPending ? (
            <div className="space-y-3">
              <Skeleton className="h-20 w-full" />
              <Skeleton className="h-20 w-full" />
            </div>
          ) : ignoredQuery.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {ignoredQuery.error instanceof Error
                  ? ignoredQuery.error.message
                  : "Failed to load ignored emails."}
              </AlertDescription>
            </Alert>
          ) : ignoredItems.length === 0 ? (
            <p className="py-4 text-sm text-muted-foreground">
              {senderFilter !== ALL_SENDERS || search.trim()
                ? "No ignored emails match the current filters."
                : "No ignored emails stored right now."}
            </p>
          ) : (
            <div className="space-y-3">
              {ignoredItems.map((item, idx) => {
                const hasDraftAmount =
                  item.amount_minor !== null &&
                  item.amount_minor > 0 &&
                  Boolean(item.currency)
                const isPromotingThis = promotingEmailId === item.email_id

                return (
                  <div key={item.email_id}>
                    {idx > 0 && <Separator className="mb-3" />}
                    <div className="space-y-2.5 rounded-lg border bg-muted/25 p-3 text-sm">
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div className="min-w-0 flex-1 space-y-1">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-medium break-words text-foreground">
                              {item.subject || "Untitled email"}
                            </span>
                            {hasDraftAmount && item.currency ? (
                              <Badge
                                variant="secondary"
                                className="tabular-nums"
                              >
                                {formatMinorCurrency(
                                  item.amount_minor!,
                                  item.currency
                                )}
                              </Badge>
                            ) : item.preLlmSkipped ? (
                              <Badge variant="outline">
                                Skipped before extraction
                              </Badge>
                            ) : (
                              <Badge variant="outline">No draft amount</Badge>
                            )}
                          </div>
                          <p className="text-xs text-muted-foreground">
                            {item.sender_email} ·{" "}
                            {formatUtcInTimezone(
                              item.occurred_at_utc ?? item.received_at_utc,
                              settings?.timezone
                            )}
                            {item.merchant_raw ? ` · ${item.merchant_raw}` : ""}
                          </p>
                          <p className="font-mono text-xs break-words text-muted-foreground">
                            Pattern: {item.triage_subject_pattern || "(any)"}
                            {item.triage_body_cue
                              ? ` · Cue: ${item.triage_body_cue}`
                              : ""}
                          </p>
                        </div>

                        <Button
                          type="button"
                          size="sm"
                          variant="outline"
                          disabled={promotingEmailId !== null}
                          onClick={() =>
                            promoteIgnoredMutation.mutate(item.email_id)
                          }
                        >
                          {isPromotingThis ? "Promoting..." : "Promote"}
                        </Button>
                      </div>

                      <details className="group">
                        <summary className="flex cursor-pointer list-none items-center gap-1.5 text-xs text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                          <ChevronDownIcon className="size-3.5 transition-transform group-open:rotate-180" />
                          <span>Inspect cleaned email text</span>
                        </summary>
                        <pre className="mt-2 max-h-60 overflow-auto rounded-md border bg-muted p-3 font-mono text-xs break-words whitespace-pre-wrap">
                          {item.cleaned_text || "(Empty email body)"}
                        </pre>
                      </details>
                    </div>
                  </div>
                )
              })}

              {ignoredTotalCount > IGNORED_PAGE_SIZE && (
                <div className="flex flex-wrap items-center justify-between gap-3 border-t pt-3 text-xs text-muted-foreground">
                  <span>
                    Showing {(clampedPage - 1) * IGNORED_PAGE_SIZE + 1}–
                    {Math.min(
                      clampedPage * IGNORED_PAGE_SIZE,
                      ignoredTotalCount
                    )}{" "}
                    of {ignoredTotalCount} ignored{" "}
                    {ignoredTotalCount === 1 ? "email" : "emails"}
                  </span>
                  <div className="flex items-center gap-2">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={clampedPage <= 1 || ignoredQuery.isFetching}
                      onClick={() => setPage((p) => Math.max(1, p - 1))}
                    >
                      Previous page
                    </Button>
                    <span className="tabular-nums">
                      Page {clampedPage} of {ignoredTotalPages}
                    </span>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={
                        clampedPage >= ignoredTotalPages ||
                        ignoredQuery.isFetching
                      }
                      onClick={() => setPage((p) => p + 1)}
                    >
                      Next page
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
