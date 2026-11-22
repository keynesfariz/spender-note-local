"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useMemo, useState, type FormEvent } from "react"

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
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
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
  dismissPendingQueueApi,
  gmailStatusQuery,
  probeLlmModelsApi,
  retryFailedExtractionsApi,
  sendersQuery,
  settingsQuery,
  syncStatusQuery,
  testLlmModelApi,
  updateLlmSettingsApi,
  updateLocaleSettingsApi,
  updateSenderApi,
  updateSyncSettingsApi,
} from "@/lib/api"
import { DatePicker } from "@/components/date-picker"
import {
  dateInputToUtcIso,
  formatRelativeUtc,
  formatUtcInTimezone,
  utcToDateInputValue,
} from "@/lib/format"

const DEFAULT_LLM_BASE_URL = "http://localhost:9931/v1"

const NO_DEFAULT_ACCOUNT = "__none__"

const SENDER_ROLE_OPTIONS = [
  { value: "auto", label: "Auto-detect role" },
  { value: "card_charge", label: "Card charge alert" },
  { value: "receipt", label: "Merchant receipt" },
] as const

export function SettingsView({
  gmailParam,
  errorParam,
}: {
  gmailParam?: string
  errorParam?: string
}) {
  const queryClient = useQueryClient()

  const settings = useQuery(settingsQuery())
  const senders = useQuery(sendersQuery())
  const gmail = useQuery(gmailStatusQuery())
  const accounts = useQuery(accountsQuery())
  const syncStatus = useQuery(syncStatusQuery())

  const [countryDraft, setCountryDraft] = useState<string | null>(null)
  const [timezoneDraft, setTimezoneDraft] = useState<string | null>(null)
  const [localeMessage, setLocaleMessage] = useState<string | null>(null)

  const [baseUrlDraft, setBaseUrlDraft] = useState<string | null>(null)
  const [modelDraft, setModelDraft] = useState<string | null>(null)
  const [customModelMode, setCustomModelMode] = useState(false)
  const [timeoutMsDraft, setTimeoutMsDraft] = useState<number | null>(null)
  const [probedState, setProbedState] = useState<{
    baseUrl: string
    availableModels: string[]
    serverReachable: boolean
    serverError: string | null
  } | null>(null)
  const [llmMessage, setLlmMessage] = useState<string | null>(null)
  const [batchLimitDraft, setBatchLimitDraft] = useState<number | null>(null)
  const [syncSettingsMessage, setSyncSettingsMessage] = useState<string | null>(
    null
  )
  const [modelTestResult, setModelTestResult] = useState<string | null>(null)

  const [newSenderEmail, setNewSenderEmail] = useState("")
  const [newSenderRole, setNewSenderRole] = useState<
    "auto" | "card_charge" | "receipt"
  >("auto")
  const [newSenderAccountId, setNewSenderAccountId] =
    useState(NO_DEFAULT_ACCOUNT)
  const [newSenderBackfill, setNewSenderBackfill] = useState("")
  const [senderMessage, setSenderMessage] = useState<string | null>(null)
  const [gmailError, setGmailError] = useState<string | null>(null)
  const [queueMessage, setQueueMessage] = useState<string | null>(null)

  const rawCountryCode = countryDraft ?? settings.data?.country_code ?? ""
  const countryCode = rawCountryCode.trim().toUpperCase()
  const timezone = timezoneDraft ?? settings.data?.timezone ?? ""
  const baseUrl =
    baseUrlDraft ?? settings.data?.llm_base_url ?? DEFAULT_LLM_BASE_URL
  const llmModel = modelDraft ?? settings.data?.llm_model ?? ""
  const timeoutMs = timeoutMsDraft ?? settings.data?.llm_timeout_ms ?? 60000
  const batchLimit = batchLimitDraft ?? settings.data?.sync_batch_limit ?? 50

  const activeProbe =
    probedState && probedState.baseUrl === baseUrl.trim() ? probedState : null
  const availableModels =
    activeProbe?.availableModels ?? settings.data?.availableModels ?? []
  const serverReachable =
    activeProbe?.serverReachable ?? settings.data?.serverReachable ?? false
  const serverError =
    activeProbe?.serverError ?? settings.data?.serverError ?? null

  const selectedCountry = (settings.data?.countryCatalog ?? []).find(
    (c) => c.code === countryCode
  )
  const savedCountryCode = (settings.data?.country_code ?? "")
    .trim()
    .toUpperCase()
  const hasUnsupportedSavedCountry =
    savedCountryCode !== "" &&
    !(settings.data?.countryCatalog ?? []).some(
      (c) => c.code === savedCountryCode
    )
  const canRepairLockedCountry =
    Boolean(settings.data?.currencyLocked) &&
    !selectedCountry &&
    (settings.data?.countryCatalog ?? []).some(
      (c) => c.currency === settings.data?.currency
    )

  const countryItems = useMemo(
    () =>
      (settings.data?.countryCatalog ?? [])
        .filter((c) =>
          canRepairLockedCountry ? c.currency === settings.data?.currency : true
        )
        .map((c) => ({
          value: c.code,
          label: `${c.name} (${c.currency})`,
        })),
    [
      settings.data?.countryCatalog,
      settings.data?.currency,
      canRepairLockedCountry,
    ]
  )

  const timezoneItems = useMemo(() => {
    const base = (selectedCountry?.timezones ?? []).map((tz) => ({
      value: tz,
      label: tz,
    }))
    if (
      selectedCountry &&
      timezone &&
      !base.some((item) => item.value === timezone)
    ) {
      base.push({ value: timezone, label: timezone })
    }
    return base
  }, [selectedCountry, timezone])

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

  const modelItems = useMemo(() => {
    const base = availableModels.map((id) => ({
      value: id,
      label: id,
    }))
    if (llmModel && !base.some((item) => item.value === llmModel)) {
      base.push({ value: llmModel, label: llmModel })
    }
    return base
  }, [availableModels, llmModel])

  const localeMutation = useMutation({
    mutationFn: updateLocaleSettingsApi,
    onSuccess: () => {
      setLocaleMessage("Locale settings saved.")
      void queryClient.invalidateQueries({ queryKey: ["settings"] })
      void queryClient.invalidateQueries({ queryKey: ["spending"] })
    },
    onError: (err) => {
      setLocaleMessage(
        err instanceof Error ? err.message : "Failed to save locale."
      )
    },
  })

  const llmMutation = useMutation({
    mutationFn: updateLlmSettingsApi,
    onSuccess: () => {
      setBaseUrlDraft(null)
      setModelDraft(null)
      setTimeoutMsDraft(null)
      setProbedState(null)
      setLlmMessage("LLM server settings saved.")
      void queryClient.invalidateQueries({ queryKey: ["settings"] })
    },
    onError: (err) => {
      setLlmMessage(
        err instanceof Error
          ? err.message
          : "Failed to save LLM server settings."
      )
    },
  })

  const probeModelsMutation = useMutation({
    mutationFn: (url: string) => probeLlmModelsApi(url),
    onSuccess: (res, queriedUrl) => {
      setProbedState({
        baseUrl: queriedUrl.trim(),
        availableModels: res.availableModels,
        serverReachable: res.serverReachable,
        serverError: res.serverError,
      })
    },
  })

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

  const testModelMutation = useMutation({
    mutationFn: testLlmModelApi,
    onSuccess: (res) => {
      setModelTestResult(
        `Connected to ${res.baseUrl} and verified model "${res.model}".`
      )
    },
    onError: (err) => {
      setModelTestResult(
        err instanceof Error ? err.message : "Connection test failed."
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

  const retryMutation = useMutation({
    mutationFn: retryFailedExtractionsApi,
    onSuccess: () => {
      setQueueMessage(null)
      void queryClient.invalidateQueries({ queryKey: ["sync-status"] })
    },
    onError: (err) => {
      setQueueMessage(
        err instanceof Error ? err.message : "Could not start retry."
      )
    },
  })

  const dismissQueueMutation = useMutation({
    mutationFn: dismissPendingQueueApi,
    onSuccess: () => {
      setQueueMessage(null)
      void queryClient.invalidateQueries({ queryKey: ["sync-status"] })
    },
    onError: (err) => {
      setQueueMessage(
        err instanceof Error ? err.message : "Could not dismiss pending queue."
      )
    },
  })

  function handleLocaleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!countryCode) return
    localeMutation.mutate({
      country_code: countryCode,
      timezone: timezone || undefined,
      currency: settings.data?.currencyLocked
        ? (settings.data.currency ?? undefined)
        : selectedCountry?.currency,
    })
  }

  function handleAddSender(e: FormEvent) {
    e.preventDefault()
    setSenderMessage(null)
    let backfillUtc: string | undefined
    if (newSenderBackfill.trim()) {
      try {
        backfillUtc = dateInputToUtcIso(
          newSenderBackfill,
          settings.data?.timezone
        )
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
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Configure your locale, Gmail connection, trusted senders, and local
          LLM server.
        </p>
      </div>

      {gmailParam === "connected" && (
        <Alert>
          <AlertDescription>Gmail connected successfully.</AlertDescription>
        </Alert>
      )}
      {errorParam && (
        <Alert variant="destructive">
          <AlertDescription>
            Gmail connection error: {errorParam}
          </AlertDescription>
        </Alert>
      )}

      {/* Locale */}
      <Card>
        <CardHeader>
          <CardTitle>Country, Timezone &amp; Currency</CardTitle>
          <CardDescription>
            All monthly reports and account balances use your selected
            country&apos;s IANA timezone and ISO 4217 currency.
          </CardDescription>
          {settings.data?.currency && (
            <CardAction>
              <Badge variant="secondary" className="font-mono">
                {settings.data.currency}
              </Badge>
            </CardAction>
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          {hasUnsupportedSavedCountry && (
            <p className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-900 dark:text-amber-200">
              Saved country <strong>{savedCountryCode}</strong> is not present
              in <code>countries/</code>.{" "}
              {settings.data?.currencyLocked
                ? `Add countries/${savedCountryCode}.json to keep using ${settings.data.currency}.`
                : `Select an available country below or add countries/${savedCountryCode}.json.`}
            </p>
          )}
          <form
            onSubmit={handleLocaleSubmit}
            className="grid gap-4 sm:grid-cols-2"
          >
            <Field>
              <FieldLabel>Country</FieldLabel>
              <Select
                items={countryItems}
                value={selectedCountry ? countryCode : null}
                disabled={
                  Boolean(settings.data?.currencyLocked) &&
                  !canRepairLockedCountry
                }
                onValueChange={(nextCode) => {
                  const code = nextCode ?? ""
                  setCountryDraft(code)
                  const match = settings.data?.countryCatalog.find(
                    (c) => c.code === code
                  )
                  if (match) setTimezoneDraft(match.defaultTimezone)
                }}
              >
                <SelectTrigger className="w-full" aria-label="Country">
                  <SelectValue placeholder="Select country..." />
                </SelectTrigger>
                <SelectContent>
                  {countryItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <FieldLabel>Timezone</FieldLabel>
              <Select
                items={timezoneItems}
                value={selectedCountry ? timezone || null : null}
                disabled={!selectedCountry}
                onValueChange={(val) => setTimezoneDraft(val ?? "")}
              >
                <SelectTrigger className="w-full" aria-label="Timezone">
                  <SelectValue placeholder="Select timezone..." />
                </SelectTrigger>
                <SelectContent>
                  {timezoneItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <div className="flex flex-wrap items-center justify-between gap-4 sm:col-span-2">
              <p className="text-xs text-muted-foreground">
                {settings.data?.currencyLocked
                  ? selectedCountry
                    ? `Currency (${settings.data.currency}) is locked because accounts or transactions exist. Timezone remains editable.`
                    : `Currency (${settings.data.currency}) is locked because accounts or transactions exist. Select a matching country or add its country file.`
                  : "Currency is derived from your country and locks once accounts or transactions exist."}
              </p>
              <Button
                type="submit"
                size="sm"
                disabled={!selectedCountry || localeMutation.isPending}
              >
                {localeMutation.isPending ? "Saving..." : "Save locale"}
              </Button>
            </div>
          </form>

          {localeMessage && <p className="text-sm">{localeMessage}</p>}
        </CardContent>
      </Card>

      {/* Gmail */}
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

      {/* Senders */}
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
                          settings.data?.timezone
                        )}
                        {sender.last_sync_at_utc ? (
                          <span
                            title={formatUtcInTimezone(
                              sender.last_sync_at_utc,
                              settings.data?.timezone
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
                          settings.data?.timezone
                        )}
                        onChange={(nextDate) => {
                          if (!nextDate) return
                          updateSenderMutation.mutate({
                            id: sender.id,
                            backfill_from_utc: dateInputToUtcIso(
                              nextDate,
                              settings.data?.timezone
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

      {/* Local LLM Server */}
      <Card>
        <CardHeader>
          <CardTitle>Local LLM Server</CardTitle>
          <CardDescription>
            OpenAI-compatible local endpoint (defaults to llama.app at{" "}
            {DEFAULT_LLM_BASE_URL})
          </CardDescription>
          <CardAction>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={probeModelsMutation.isPending}
              onClick={() => probeModelsMutation.mutate(baseUrl)}
            >
              {probeModelsMutation.isPending
                ? "Refreshing..."
                : "Refresh models"}
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className="space-y-4">
          <form
            className="grid gap-4 sm:grid-cols-3"
            onSubmit={(e) => {
              e.preventDefault()
              setLlmMessage(null)
              llmMutation.mutate({
                llm_base_url: baseUrl,
                llm_model: llmModel.trim() || null,
                llm_timeout_ms: timeoutMs || 60000,
              })
            }}
          >
            <Field className="sm:col-span-3">
              <FieldLabel>Server URL</FieldLabel>
              <Input
                type="url"
                placeholder={DEFAULT_LLM_BASE_URL}
                value={baseUrl}
                onChange={(e) => setBaseUrlDraft(e.target.value)}
              />
              {!serverReachable && serverError && (
                <FieldDescription>
                  Server unreachable: {serverError}
                </FieldDescription>
              )}
            </Field>

            <Field className="sm:col-span-2">
              <div className="flex items-center justify-between gap-2">
                <FieldLabel>Model</FieldLabel>
                {availableModels.length > 0 && (
                  <button
                    type="button"
                    className="text-xs text-muted-foreground underline hover:text-foreground"
                    onClick={() => setCustomModelMode((prev) => !prev)}
                  >
                    {customModelMode
                      ? "Choose from server list"
                      : "Enter model ID manually"}
                  </button>
                )}
              </div>
              {availableModels.length > 0 && !customModelMode ? (
                <Select
                  items={modelItems}
                  value={llmModel || null}
                  onValueChange={(v) => setModelDraft(v ?? "")}
                >
                  <SelectTrigger className="w-full" aria-label="Selected model">
                    <SelectValue placeholder="Select a model from your local server..." />
                  </SelectTrigger>
                  <SelectContent>
                    {modelItems.map((item) => (
                      <SelectItem key={item.value} value={item.value}>
                        {item.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  type="text"
                  placeholder="Enter model ID (e.g. qwen2.5-3b-instruct)"
                  value={llmModel}
                  onChange={(e) => setModelDraft(e.target.value)}
                />
              )}
              {!llmModel && (
                <FieldDescription>
                  Start llama.app (or your local server), click Refresh models,
                  and select a model before syncing.
                </FieldDescription>
              )}
            </Field>

            <Field>
              <FieldLabel>Per-email timeout (ms)</FieldLabel>
              <Input
                type="number"
                min={1000}
                max={600000}
                step={1000}
                value={timeoutMs}
                onChange={(e) => setTimeoutMsDraft(Number(e.target.value))}
              />
            </Field>

            <div className="flex items-end gap-2 sm:col-span-3">
              <Button type="submit" size="sm" disabled={llmMutation.isPending}>
                Save server settings
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={
                  syncStatus.data?.isRunning ||
                  testModelMutation.isPending ||
                  !llmModel.trim()
                }
                onClick={() => {
                  setModelTestResult(null)
                  testModelMutation.mutate({
                    llm_base_url: baseUrl,
                    llm_model: llmModel.trim(),
                    llm_timeout_ms: timeoutMs || 60000,
                  })
                }}
              >
                {testModelMutation.isPending ? "Testing..." : "Test connection"}
              </Button>
            </div>
          </form>

          {llmMessage && <p className="text-sm">{llmMessage}</p>}
          {modelTestResult && (
            <Alert>
              <AlertDescription>{modelTestResult}</AlertDescription>
            </Alert>
          )}

          <Separator />

          <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
            <span className="text-muted-foreground">
              Extraction queue: {syncStatus.data?.pendingEmailCount ?? 0}{" "}
              pending, {syncStatus.data?.failedEmailCount ?? 0} failed
            </span>
            <div className="flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={
                  syncStatus.data?.isRunning ||
                  retryMutation.isPending ||
                  dismissQueueMutation.isPending ||
                  ((syncStatus.data?.pendingEmailCount ?? 0) === 0 &&
                    (syncStatus.data?.failedEmailCount ?? 0) === 0)
                }
                onClick={() => retryMutation.mutate()}
              >
                Retry pending &amp; failed
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="sm"
                disabled={
                  syncStatus.data?.isRunning ||
                  retryMutation.isPending ||
                  dismissQueueMutation.isPending ||
                  ((syncStatus.data?.pendingEmailCount ?? 0) === 0 &&
                    (syncStatus.data?.failedEmailCount ?? 0) === 0)
                }
                onClick={() => dismissQueueMutation.mutate()}
              >
                Dismiss pending queue
              </Button>
            </div>
          </div>

          {(queueMessage || gmailError) && (
            <Alert variant="destructive">
              <AlertDescription>{queueMessage || gmailError}</AlertDescription>
            </Alert>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
