'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';

import { Alert, AlertDescription } from '@/components/ui/alert';
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
} from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Separator } from '@/components/ui/separator';
import {
  accountsQuery,
  createSenderApi,
  deleteSenderApi,
  disconnectGmailApi,
  dismissPendingQueueApi,
  gmailStatusQuery,
  retryFailedExtractionsApi,
  sendersQuery,
  settingsQuery,
  syncStatusQuery,
  testLlmModelApi,
  updateLlmSettingsApi,
  updateLocaleSettingsApi,
  updateSenderApi,
  updateSyncSettingsApi,
} from '@/lib/api';
import { dateInputToUtcIso, utcToDateInputValue } from '@/lib/format';

function formatBytes(bytes: number): string {
  if (bytes >= 1024 ** 3) return `${(bytes / 1024 ** 3).toFixed(2)} GB`;
  return `${Math.round(bytes / 1024 ** 2)} MB`;
}

const NO_DEFAULT_ACCOUNT = '__none__';

export function SettingsView({
  gmailParam,
  errorParam,
}: {
  gmailParam?: string;
  errorParam?: string;
}) {
  const queryClient = useQueryClient();

  const settings = useQuery(settingsQuery());
  const senders = useQuery(sendersQuery());
  const gmail = useQuery(gmailStatusQuery());
  const accounts = useQuery(accountsQuery());
  const syncStatus = useQuery(syncStatusQuery());

  const [countryDraft, setCountryDraft] = useState<string | null>(null);
  const [timezoneDraft, setTimezoneDraft] = useState<string | null>(null);
  const [localeMessage, setLocaleMessage] = useState<string | null>(null);

  const [modelPathDraft, setModelPathDraft] = useState<string | null>(null);
  const [contextSizeDraft, setContextSizeDraft] = useState<number | null>(null);
  const [timeoutMsDraft, setTimeoutMsDraft] = useState<number | null>(null);
  const [llmMessage, setLlmMessage] = useState<string | null>(null);
  const [batchLimitDraft, setBatchLimitDraft] = useState<number | null>(null);
  const [syncSettingsMessage, setSyncSettingsMessage] = useState<string | null>(
    null,
  );
  const [modelTestResult, setModelTestResult] = useState<string | null>(null);

  const [newSenderEmail, setNewSenderEmail] = useState('');
  const [newSenderRole, setNewSenderRole] = useState<
    'auto' | 'card_charge' | 'receipt'
  >('auto');
  const [newSenderAccountId, setNewSenderAccountId] =
    useState(NO_DEFAULT_ACCOUNT);
  const [newSenderBackfill, setNewSenderBackfill] = useState('');
  const [senderMessage, setSenderMessage] = useState<string | null>(null);
  const [gmailError, setGmailError] = useState<string | null>(null);
  const [queueMessage, setQueueMessage] = useState<string | null>(null);

  const countryCode = countryDraft ?? settings.data?.country_code ?? '';
  const timezone = timezoneDraft ?? settings.data?.timezone ?? '';
  const modelPath = modelPathDraft ?? settings.data?.llm_model_path ?? '';
  const contextSize = contextSizeDraft ?? settings.data?.llm_context_size ?? 8192;
  const timeoutMs = timeoutMsDraft ?? settings.data?.llm_timeout_ms ?? 60000;
  const batchLimit = batchLimitDraft ?? settings.data?.sync_batch_limit ?? 50;

  const selectedCountry = (settings.data?.countryCatalog ?? []).find(
    (c) => c.code === countryCode,
  );

  const localeMutation = useMutation({
    mutationFn: updateLocaleSettingsApi,
    onSuccess: () => {
      setLocaleMessage('Locale settings saved.');
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
      void queryClient.invalidateQueries({ queryKey: ['spending'] });
    },
    onError: (err) => {
      setLocaleMessage(
        err instanceof Error ? err.message : 'Failed to save locale.',
      );
    },
  });

  const llmMutation = useMutation({
    mutationFn: updateLlmSettingsApi,
    onSuccess: () => {
      setLlmMessage('Model settings saved.');
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
    onError: (err) => {
      setLlmMessage(
        err instanceof Error ? err.message : 'Failed to save model settings.',
      );
    },
  });

  const syncSettingsMutation = useMutation({
    mutationFn: updateSyncSettingsApi,
    onSuccess: () => {
      setSyncSettingsMessage('Sync settings saved.');
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
    onError: (err) => {
      setSyncSettingsMessage(
        err instanceof Error ? err.message : 'Failed to save sync settings.',
      );
    },
  });

  const testModelMutation = useMutation({
    mutationFn: testLlmModelApi,
    onSuccess: (res) => {
      setModelTestResult(
        `Loaded ${res.modelPath} cleanly (context ${res.contextSize}).`,
      );
    },
    onError: (err) => {
      setModelTestResult(
        err instanceof Error ? err.message : 'Model test failed.',
      );
    },
  });

  const createSenderMutation = useMutation({
    mutationFn: createSenderApi,
    onSuccess: () => {
      setNewSenderEmail('');
      setNewSenderRole('auto');
      setNewSenderAccountId(NO_DEFAULT_ACCOUNT);
      setNewSenderBackfill('');
      setSenderMessage('Sender added.');
      void queryClient.invalidateQueries({ queryKey: ['senders'] });
    },
    onError: (err) => {
      setSenderMessage(
        err instanceof Error ? err.message : 'Could not add sender.',
      );
    },
  });

  const updateSenderMutation = useMutation({
    mutationFn: updateSenderApi,
    onSuccess: () => {
      setSenderMessage('Sender updated.');
      void queryClient.invalidateQueries({ queryKey: ['senders'] });
    },
    onError: (err) => {
      setSenderMessage(
        err instanceof Error ? err.message : 'Could not update sender.',
      );
    },
  });

  const deleteSenderMutation = useMutation({
    mutationFn: deleteSenderApi,
    onSuccess: () => {
      setSenderMessage('Sender removed.');
      void queryClient.invalidateQueries({ queryKey: ['senders'] });
    },
    onError: (err) => {
      setSenderMessage(
        err instanceof Error ? err.message : 'Could not remove sender.',
      );
    },
  });

  const disconnectGmailMutation = useMutation({
    mutationFn: disconnectGmailApi,
    onSuccess: () => {
      setGmailError(null);
      void queryClient.invalidateQueries({ queryKey: ['gmail-status'] });
    },
    onError: (err) => {
      setGmailError(
        err instanceof Error ? err.message : 'Could not disconnect Gmail.',
      );
    },
  });

  const retryMutation = useMutation({
    mutationFn: retryFailedExtractionsApi,
    onSuccess: () => {
      setQueueMessage(null);
      void queryClient.invalidateQueries({ queryKey: ['sync-status'] });
    },
    onError: (err) => {
      setQueueMessage(
        err instanceof Error ? err.message : 'Could not start retry.',
      );
    },
  });

  const dismissQueueMutation = useMutation({
    mutationFn: dismissPendingQueueApi,
    onSuccess: () => {
      setQueueMessage(null);
      void queryClient.invalidateQueries({ queryKey: ['sync-status'] });
    },
    onError: (err) => {
      setQueueMessage(
        err instanceof Error ? err.message : 'Could not dismiss pending queue.',
      );
    },
  });

  function handleLocaleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!countryCode) return;
    localeMutation.mutate({
      country_code: countryCode,
      timezone: timezone || undefined,
      currency: settings.data?.currencyLocked
        ? (settings.data.currency ?? undefined)
        : selectedCountry?.currency,
    });
  }

  function handleAddSender(e: FormEvent) {
    e.preventDefault();
    setSenderMessage(null);
    let backfillUtc: string | undefined;
    if (newSenderBackfill.trim()) {
      try {
        backfillUtc = dateInputToUtcIso(
          newSenderBackfill,
          settings.data?.timezone,
        );
      } catch (err) {
        setSenderMessage(
          err instanceof Error ? err.message : 'Invalid backfill date.',
        );
        return;
      }
    }
    createSenderMutation.mutate({
      email: newSenderEmail,
      role_hint: newSenderRole,
      default_account_id:
        newSenderAccountId !== NO_DEFAULT_ACCOUNT ? newSenderAccountId : null,
      backfill_from_utc: backfillUtc,
    });
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Configure your locale, Gmail connection, trusted senders, and local
          GGUF model.
        </p>
      </div>

      {gmailParam === 'connected' && (
        <Alert>
          <AlertDescription>Gmail connected successfully.</AlertDescription>
        </Alert>
      )}
      {errorParam && (
        <Alert variant="destructive">
          <AlertDescription>Gmail connection error: {errorParam}</AlertDescription>
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
          <form onSubmit={handleLocaleSubmit} className="grid gap-4 sm:grid-cols-2">
            <Field>
              <FieldLabel>Country</FieldLabel>
              <Select
                value={countryCode}
                disabled={settings.data?.currencyLocked}
                onValueChange={(nextCode) => {
                  const code = nextCode ?? '';
                  setCountryDraft(code);
                  const match = settings.data?.countryCatalog.find(
                    (c) => c.code === code,
                  );
                  if (match) setTimezoneDraft(match.defaultTimezone);
                }}
              >
                <SelectTrigger className="w-full" aria-label="Country">
                  <SelectValue placeholder="Select country..." />
                </SelectTrigger>
                <SelectContent>
                  {(settings.data?.countryCatalog ?? []).map((c) => (
                    <SelectItem key={c.code} value={c.code}>
                      {c.name} ({c.currency})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <FieldLabel>Timezone</FieldLabel>
              <Select
                value={timezone}
                onValueChange={(val) => setTimezoneDraft(val ?? '')}
              >
                <SelectTrigger className="w-full" aria-label="Timezone">
                  <SelectValue placeholder="Select timezone..." />
                </SelectTrigger>
                <SelectContent>
                  {(selectedCountry?.timezones ?? []).map((tz) => (
                    <SelectItem key={tz} value={tz}>
                      {tz}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <div className="flex flex-wrap items-center justify-between gap-4 sm:col-span-2">
              <p className="text-xs text-muted-foreground">
                {settings.data?.currencyLocked
                  ? `Currency (${settings.data.currency}) is locked because accounts or transactions exist. Timezone remains editable.`
                  : 'Currency is derived from your country and locks once accounts or transactions exist.'}
              </p>
              <Button
                type="submit"
                size="sm"
                disabled={!countryCode || localeMutation.isPending}
              >
                {localeMutation.isPending ? 'Saving...' : 'Save locale'}
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
            {gmail.data?.status === 'connected' ? (
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
                        Spender will stop syncing alerts and receipts from{' '}
                        {gmail.data.email_address ?? 'your Gmail account'} until
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
              <Button size="sm" render={<a href="/api/gmail/auth" />}>
                {gmail.data?.status === 'expired'
                  ? 'Reconnect Gmail'
                  : 'Connect Gmail'}
              </Button>
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
              e.preventDefault();
              setSyncSettingsMessage(null);
              syncSettingsMutation.mutate({ sync_batch_limit: batchLimit });
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
              {syncSettingsMutation.isPending ? 'Saving...' : 'Save batch limit'}
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
            date earlier rewinds that sender&apos;s cursor to fetch older emails.
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
                value={newSenderRole}
                onValueChange={(v) => {
                  if (v === 'auto' || v === 'card_charge' || v === 'receipt') {
                    setNewSenderRole(v);
                  }
                }}
              >
                <SelectTrigger className="w-full" aria-label="Sender role">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="auto">Auto-detect role</SelectItem>
                  <SelectItem value="card_charge">Card charge alert</SelectItem>
                  <SelectItem value="receipt">Merchant receipt</SelectItem>
                </SelectContent>
              </Select>
            </Field>

            <Field>
              <FieldLabel>Default account</FieldLabel>
              <Select
                value={newSenderAccountId}
                onValueChange={(v) =>
                  setNewSenderAccountId(v ?? NO_DEFAULT_ACCOUNT)
                }
              >
                <SelectTrigger className="w-full" aria-label="Default account">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_DEFAULT_ACCOUNT}>
                    Default account (optional)
                  </SelectItem>
                  {(accounts.data ?? []).map((acct) => (
                    <SelectItem key={acct.id} value={acct.id}>
                      {acct.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>

            <div className="flex items-end gap-2">
              <Field className="min-w-0 flex-1">
                <FieldLabel>Backfill start date</FieldLabel>
                <Input
                  type="date"
                  value={newSenderBackfill}
                  onChange={(e) => setNewSenderBackfill(e.target.value)}
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
                        Role: {sender.role_hint} · Backfill from{' '}
                        {utcToDateInputValue(
                          sender.backfill_from_utc,
                          settings.data?.timezone,
                        )}
                        {sender.last_sync_at_utc
                          ? ` · Last synced ${utcToDateInputValue(
                              sender.last_sync_at_utc,
                              settings.data?.timezone,
                            )}`
                          : ' · Never synced'}
                      </p>
                    </div>

                    <div className="flex flex-wrap items-center gap-3">
                      <Input
                        type="date"
                        aria-label={`Backfill date for ${sender.email}`}
                        className="w-40"
                        value={utcToDateInputValue(
                          sender.backfill_from_utc,
                          settings.data?.timezone,
                        )}
                        onChange={(e) => {
                          if (!e.target.value) return;
                          updateSenderMutation.mutate({
                            id: sender.id,
                            backfill_from_utc: dateInputToUtcIso(
                              e.target.value,
                              settings.data?.timezone,
                            ),
                          });
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
                          {sender.enabled === 1 ? 'Enabled' : 'Disabled'}
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

      {/* Local GGUF Model */}
      <Card>
        <CardHeader>
          <CardTitle>Local GGUF Model</CardTitle>
          <CardDescription>
            Models directory: {settings.data?.modelsDir ?? 'models'}
          </CardDescription>
          <CardAction>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() =>
                void queryClient.invalidateQueries({ queryKey: ['settings'] })
              }
            >
              Rescan models
            </Button>
          </CardAction>
        </CardHeader>
        <CardContent className="space-y-4">
          <form
            className="grid gap-4 sm:grid-cols-3"
            onSubmit={(e) => {
              e.preventDefault();
              setLlmMessage(null);
              llmMutation.mutate({
                llm_model_path: modelPath || null,
                llm_context_size: contextSize || 8192,
                llm_timeout_ms: timeoutMs || 60000,
              });
            }}
          >
            <Field className="sm:col-span-3">
              <FieldLabel>Selected .gguf model</FieldLabel>
              <Select
                value={modelPath}
                onValueChange={(v) => setModelPathDraft(v ?? '')}
              >
                <SelectTrigger className="w-full" aria-label="Selected .gguf model">
                  <SelectValue placeholder="Select a local GGUF model..." />
                </SelectTrigger>
                <SelectContent>
                  {(settings.data?.availableModels ?? []).map((m) => (
                    <SelectItem key={m.relativePath} value={m.relativePath}>
                      {m.relativePath} ({formatBytes(m.sizeBytes)})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {(settings.data?.availableModels.length ?? 0) === 0 && (
                <FieldDescription>
                  Place a .gguf model file inside{' '}
                  {settings.data?.modelsDir ?? 'models'} and click Rescan models.
                </FieldDescription>
              )}
            </Field>

            <Field>
              <FieldLabel>Context size</FieldLabel>
              <Input
                type="number"
                min={512}
                max={131072}
                step={512}
                value={contextSize}
                onChange={(e) => setContextSizeDraft(Number(e.target.value))}
              />
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

            <div className="flex items-end gap-2">
              <Button type="submit" size="sm" disabled={llmMutation.isPending}>
                Save model
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={
                  syncStatus.data?.isRunning ||
                  testModelMutation.isPending ||
                  !modelPath
                }
                onClick={() => {
                  setModelTestResult(null);
                  testModelMutation.mutate();
                }}
              >
                {testModelMutation.isPending ? 'Testing...' : 'Test load'}
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
              Extraction queue: {syncStatus.data?.pendingEmailCount ?? 0}{' '}
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
  );
}
