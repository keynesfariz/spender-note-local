"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useMemo, useState, type FormEvent } from "react"

import { DatePicker } from "@/components/date-picker"
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
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import {
  accountsQuery,
  createSenderApi,
  deleteSenderApi,
  disconnectGmailApi,
  gmailStatusQuery,
  sendersQuery,
  updateSenderApi,
  updateSyncSettingsApi,
  type AppSettingsView,
} from "@/lib/api"
import {
  dateInputToUtcIso,
  formatRelativeUtc,
  formatUtcInTimezone,
  utcToDateInputValue,
} from "@/lib/format"

const NO_DEFAULT_ACCOUNT = "__none__"

const SENDER_ROLE_OPTIONS = [
  { value: "auto", label: "Auto-detect role" },
  { value: "card_charge", label: "Card charge alert" },
  { value: "receipt", label: "Merchant receipt" },
] as const

export function GmailSendersTab({
  settings,
}: {
  settings: AppSettingsView | undefined
}) {
  const queryClient = useQueryClient()
  const senders = useQuery(sendersQuery())
  const gmail = useQuery(gmailStatusQuery())
  const accounts = useQuery(accountsQuery())

  const [batchLimitDraft, setBatchLimitDraft] = useState<number | null>(null)
  const [syncSettingsMessage, setSyncSettingsMessage] = useState<string | null>(
    null
  )
  const [newSenderEmail, setNewSenderEmail] = useState("")
  const [newSenderRole, setNewSenderRole] = useState<
    "auto" | "card_charge" | "receipt"
  >("auto")
  const [newSenderAccountId, setNewSenderAccountId] =
    useState(NO_DEFAULT_ACCOUNT)
  const [newSenderBackfill, setNewSenderBackfill] = useState("")
  const [senderMessage, setSenderMessage] = useState<string | null>(null)
  const [gmailError, setGmailError] = useState<string | null>(null)

  const batchLimit = batchLimitDraft ?? settings?.sync_batch_limit ?? 50

  const defaultAccountItems = useMemo(
    () => [
      { value: NO_DEFAULT_ACCOUNT, label: "Default account (optional)" },
      ...(accounts.data ?? []).map((acct) => ({
        value: acct.id,
        label: acct.name,
      })),
    ],
    [accounts.data]
  )

  const syncSettingsMutation = useMutation({
    mutationFn: updateSyncSettingsApi,
    onSuccess: () => {
      setSyncSettingsMessage("Sync settings saved.")
      void queryClient.invalidateQueries({ queryKey: ["settings"] })
    },
    onError: (err) => {
      setSyncSettingsMessage(
        err instanceof Error ? err.message : "Failed to save sync settings."
      )
    },
  })

  const createSenderMutation = useMutation({
    mutationFn: createSenderApi,
    onSuccess: () => {
      setNewSenderEmail("")
      setNewSenderRole("auto")
      setNewSenderAccountId(NO_DEFAULT_ACCOUNT)
      setNewSenderBackfill("")
      setSenderMessage("Sender added.")
      void queryClient.invalidateQueries({ queryKey: ["senders"] })
    },
    onError: (err) => {
      setSenderMessage(
        err instanceof Error ? err.message : "Could not add sender."
      )
    },
  })

  const updateSenderMutation = useMutation({
    mutationFn: updateSenderApi,
    onSuccess: () => {
      setSenderMessage("Sender updated.")
      void queryClient.invalidateQueries({ queryKey: ["senders"] })
    },
    onError: (err) => {
      setSenderMessage(
        err instanceof Error ? err.message : "Could not update sender."
      )
    },
  })

  const deleteSenderMutation = useMutation({
    mutationFn: deleteSenderApi,
    onSuccess: () => {
      setSenderMessage("Sender removed.")
      void queryClient.invalidateQueries({ queryKey: ["senders"] })
    },
    onError: (err) => {
      setSenderMessage(
        err instanceof Error ? err.message : "Could not remove sender."
      )
    },
  })

  const disconnectGmailMutation = useMutation({
    mutationFn: disconnectGmailApi,
    onSuccess: () => {
      setGmailError(null)
      void queryClient.invalidateQueries({ queryKey: ["gmail-status"] })
    },
    onError: (err) => {
      setGmailError(
        err instanceof Error ? err.message : "Could not disconnect Gmail."
      )
    },
  })

  function handleAddSender(e: FormEvent) {
    e.preventDefault()
    setSenderMessage(null)
    let backfillUtc: string | undefined
    if (newSenderBackfill.trim()) {
      try {
        backfillUtc = dateInputToUtcIso(newSenderBackfill, settings?.timezone)
      } catch (err) {
        setSenderMessage(
          err instanceof Error ? err.message : "Invalid backfill date."
        )
        return
      }
    }
    createSenderMutation.mutate({
      email: newSenderEmail,
      role_hint: newSenderRole,
      default_account_id:
        newSenderAccountId !== NO_DEFAULT_ACCOUNT ? newSenderAccountId : null,
      backfill_from_utc: backfillUtc,
    })
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>Gmail Connection</CardTitle>
          <CardDescription>
            Read-only access (gmail.readonly) to fetch alerts and receipts from
            configured senders.
          </CardDescription>
          <CardAction className="flex items-center gap-2">
            {gmail.data?.status === "connected" ? (
              <>
                <Badge variant="secondary">
                  Connected: {gmail.data.email_address}
                </Badge>
                <AlertDialog>
                  <AlertDialogTrigger
                    render={
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={disconnectGmailMutation.isPending}
                      />
                    }
                  >
                    Disconnect Gmail
                  </AlertDialogTrigger>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>
                        Disconnect Gmail account?
                      </AlertDialogTitle>
                      <AlertDialogDescription>
                        Spender will stop syncing alerts and receipts from{" "}
                        {gmail.data.email_address ?? "your Gmail account"} until
                        you reconnect.
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>Cancel</AlertDialogCancel>
                      <AlertDialogAction
                        variant="destructive"
                        onClick={() => disconnectGmailMutation.mutate()}
                      >
                        Disconnect Gmail account
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </>
            ) : (
              <a
                href="/api/gmail/auth"
                className={buttonVariants({ size: "sm" })}
              >
                {gmail.data?.status === "expired"
                  ? "Reconnect Gmail"
                  : "Connect Gmail"}
              </a>
            )}
          </CardAction>
        </CardHeader>
        <CardContent className="space-y-4">
          {gmail.data && !gmail.data.oauthConfigured && (
            <Alert>
              <AlertDescription>
                Set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET in .env before
                connecting Gmail.
              </AlertDescription>
            </Alert>
          )}
          {gmailError && (
            <Alert variant="destructive">
              <AlertDescription>{gmailError}</AlertDescription>
            </Alert>
          )}

          <Separator />

          <form
            className="flex flex-wrap items-end gap-3"
            onSubmit={(e) => {
              e.preventDefault()
              setSyncSettingsMessage(null)
              syncSettingsMutation.mutate({ sync_batch_limit: batchLimit })
            }}
          >
            <Field className="w-44">
              <FieldLabel>Emails per sync batch</FieldLabel>
              <Input
                type="number"
                min={1}
                max={500}
                step={1}
                value={batchLimit}
                onChange={(e) => setBatchLimitDraft(Number(e.target.value))}
              />
            </Field>
            <Button
              type="submit"
              size="sm"
              disabled={syncSettingsMutation.isPending}
            >
              {syncSettingsMutation.isPending
                ? "Saving..."
                : "Save batch limit"}
            </Button>
            {syncSettingsMessage && (
              <p className="text-sm">{syncSettingsMessage}</p>
            )}
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Email Senders</CardTitle>
          <CardDescription>
            Each sender maintains an independent sync cursor. Moving a backfill
            date earlier rewinds that sender&apos;s cursor to fetch older
            emails.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <form
            onSubmit={handleAddSender}
            className="grid gap-3 rounded-lg border p-4 sm:grid-cols-5"
          >
            <Field className="sm:col-span-2">
              <FieldLabel>Sender email</FieldLabel>
              <Input
                type="email"
                required
                placeholder="alerts@bank.com"
                value={newSenderEmail}
                onChange={(e) => setNewSenderEmail(e.target.value)}
              />
            </Field>

            <Field>
              <FieldLabel>Sender role</FieldLabel>
              <Select
                items={SENDER_ROLE_OPTIONS}
                value={newSenderRole}
                onValueChange={(v) => {
                  if (v === "auto" || v === "card_charge" || v === "receipt") {
                    setNewSenderRole(v)
                  }
                }}
              >
                <SelectTrigger className="w-full" aria-label="Sender role">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SENDER_ROLE_OPTIONS.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <FieldLabel>Default account</FieldLabel>
              <Select
                items={defaultAccountItems}
                value={newSenderAccountId}
                onValueChange={(v) =>
                  setNewSenderAccountId(v ?? NO_DEFAULT_ACCOUNT)
                }
              >
                <SelectTrigger className="w-full" aria-label="Default account">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {defaultAccountItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <div className="flex items-end gap-2">
              <Field className="min-w-0 flex-1">
                <FieldLabel>Backfill start date</FieldLabel>
                <DatePicker
                  value={newSenderBackfill}
                  onChange={setNewSenderBackfill}
                  placeholder="Optional"
                  clearable
                  ariaLabel="Backfill start date"
                />
              </Field>
              <Button
                type="submit"
                size="sm"
                disabled={createSenderMutation.isPending}
              >
                Add
              </Button>
            </div>
          </form>

          {senderMessage && <p className="text-sm">{senderMessage}</p>}

          {(senders.data ?? []).length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No senders configured yet.
            </p>
          ) : (
            <div className="space-y-3">
              {(senders.data ?? []).map((sender, idx) => (
                <div key={sender.id}>
                  {idx > 0 && <Separator className="mb-3" />}
                  <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
                    <div>
                      <p className="font-medium">{sender.email}</p>
                      <p className="text-xs text-muted-foreground">
                        Role: {sender.role_hint} · Backfill from{" "}
                        {utcToDateInputValue(
                          sender.backfill_from_utc,
                          settings?.timezone
                        )}
                        {sender.last_sync_at_utc ? (
                          <span
                            title={formatUtcInTimezone(
                              sender.last_sync_at_utc,
                              settings?.timezone
                            )}
                          >
                            {" "}
                            · Last synced{" "}
                            {formatRelativeUtc(sender.last_sync_at_utc)}
                          </span>
                        ) : (
                          " · Never synced"
                        )}
                      </p>
                    </div>

                    <div className="flex flex-wrap items-center gap-3">
                      <DatePicker
                        ariaLabel={`Backfill date for ${sender.email}`}
                        className="w-44"
                        value={utcToDateInputValue(
                          sender.backfill_from_utc,
                          settings?.timezone
                        )}
                        onChange={(nextDate) => {
                          if (!nextDate) return
                          updateSenderMutation.mutate({
                            id: sender.id,
                            backfill_from_utc: dateInputToUtcIso(
                              nextDate,
                              settings?.timezone
                            ),
                          })
                        }}
                      />

                      <div className="flex items-center gap-2">
                        <Checkbox
                          id={`sender-enabled-${sender.id}`}
                          checked={sender.enabled === 1}
                          onCheckedChange={(checked) =>
                            updateSenderMutation.mutate({
                              id: sender.id,
                              enabled: Boolean(checked),
                            })
                          }
                        />
                        <Label htmlFor={`sender-enabled-${sender.id}`}>
                          {sender.enabled === 1 ? "Enabled" : "Disabled"}
                        </Label>
                      </div>

                      <AlertDialog>
                        <AlertDialogTrigger
                          render={
                            <Button
                              type="button"
                              variant="destructive"
                              size="sm"
                            />
                          }
                        >
                          Remove
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>
                              Remove {sender.email}?
                            </AlertDialogTitle>
                            <AlertDialogDescription>
                              This permanently removes {sender.email} and its
                              sync cursor from your trusted senders list.
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>Cancel</AlertDialogCancel>
                            <AlertDialogAction
                              variant="destructive"
                              onClick={() =>
                                deleteSenderMutation.mutate(sender.id)
                              }
                            >
                              Remove sender permanently
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </>
  )
}
