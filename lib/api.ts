import { queryOptions } from '@tanstack/react-query';

export type AccountType = 'credit_card' | 'debit_card' | 'digital_wallet';
export type AccountIdentifierKind = 'card_last4' | 'bank_account' | 'alias';
export type AccountStatus = 'provisional' | 'confirmed';

export interface AccountIdentifierInput {
  kind: AccountIdentifierKind;
  value: string;
}

export interface AccountIdentifierRow {
  id: string;
  account_id: string;
  kind: AccountIdentifierKind;
  value: string;
  normalized_value: string;
  created_at_utc: string;
}

export interface ReconciliationRow {
  id: string;
  account_id: string;
  currency: string;
  computed_balance_minor: number;
  actual_balance_minor: number;
  adjustment_minor: number;
  note: string | null;
  recorded_at_utc: string;
}

export interface AccountSummary {
  id: string;
  name: string;
  type: AccountType;
  currency: string;
  initial_balance_minor: number | null;
  initial_balance_as_of_utc: string;
  last4: string | null;
  status: AccountStatus;
  created_at_utc: string;
  identifiers: AccountIdentifierRow[];
  computed_balance_minor: number | null;
  net_flow_minor: number;
  uncounted_foreign_count: number;
  latest_reconciliation: ReconciliationRow | null;
  type_locked: boolean;
}

export interface CountryLocaleInfo {
  code: string;
  name: string;
  currency: string;
  defaultTimezone: string;
  timezones: string[];
}

export interface LocalGgufModelInfo {
  relativePath: string;
  sizeBytes: number;
}

export interface AppSettingsView {
  country_code: string | null;
  timezone: string | null;
  currency: string | null;
  llm_provider: 'gguf';
  llm_model_path: string | null;
  llm_context_size: number;
  llm_timeout_ms: number;
  sync_batch_limit: number;
  currencyLocked: boolean;
  modelsDir: string;
  availableModels: LocalGgufModelInfo[];
  countryCatalog: CountryLocaleInfo[];
}

export interface SenderRecord {
  id: string;
  email: string;
  role_hint: 'auto' | 'card_charge' | 'receipt';
  default_account_id: string | null;
  enabled: 0 | 1;
  backfill_from_utc: string;
  last_internal_date_ms: number | null;
  last_sync_at_utc: string | null;
}

export interface GmailStatusView {
  status: 'disconnected' | 'connected' | 'expired';
  email_address: string | null;
  expires_at_utc: string | null;
  scope: string | null;
  oauthConfigured: boolean;
}

export interface SpendingGroupRow {
  key: string;
  label: string;
  total_minor: number;
  count: number;
}

export interface MonthlySpendingSummary {
  configured: boolean;
  month: string | null;
  timezone: string | null;
  currency: string | null;
  startUtc: string | null;
  endUtc: string | null;
  totalSpentMinor: number;
  transactionCount: number;
  foreignCurrencyCount: number;
  byCategory: SpendingGroupRow[];
  byMerchant: SpendingGroupRow[];
  byAccount: SpendingGroupRow[];
}

export interface TransactionListItem {
  id: string;
  source_type: 'gmail' | 'manual';
  account_id: string | null;
  account_name: string | null;
  payment_method_raw: string | null;
  amount_minor: number;
  currency: string;
  occurred_at_utc: string;
  merchant: string;
  category: string;
  manual_override: 0 | 1;
  reviewed: 0 | 1;
  note: string | null;
  created_at_utc: string;
  updated_at_utc: string;
  sourceCount: number;
  sourceRoles: Array<'card_charge' | 'receipt'>;
  isForeignCurrency: boolean;
}

export interface PaginatedTransactionsResponse {
  items: TransactionListItem[];
  totalCount: number;
  page: number;
  pageSize: number;
  totalInRangeCount: number;
  unreviewedInRangeCount: number;
}

export interface TransactionSourceView {
  id: string;
  email_id: string;
  role: 'card_charge' | 'receipt';
  match_score: number;
  match_state: 'auto' | 'confirmed' | 'single';
  from_address: string;
  subject: string;
  cleaned_text: string;
  received_at_utc: string;
  extraction_model: string | null;
  raw_llm_json: string | null;
}

export interface TransactionDetailView {
  transaction: TransactionListItem;
  sources: TransactionSourceView[];
}

export interface SyncRunRow {
  id: string;
  status: 'running' | 'completed' | 'partial_error' | 'failed' | 'cancelled' | 'interrupted';
  phase: 'preflight' | 'fetching' | 'extracting' | 'done';
  llm_provider: string;
  llm_model: string;
  fetched_count: number;
  cleaned_count: number;
  imported_count: number;
  merged_count: number;
  skipped_count: number;
  failed_count: number;
  has_more_emails: number;
  provisional_accounts_created: number;
  error_message: string | null;
  started_at_utc: string;
  finished_at_utc: string | null;
}

export interface SyncStatusView {
  latestRun: SyncRunRow | null;
  isRunning: boolean;
  pendingEmailCount: number;
  failedEmailCount: number;
}

async function apiFetch<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    ...init,
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(
      typeof data?.error === 'string' ? data.error : `Request failed (${res.status})`,
    );
  }
  return data as T;
}

// Queries
export const settingsQuery = () =>
  queryOptions({
    queryKey: ['settings'] as const,
    queryFn: () => apiFetch<AppSettingsView>('/api/settings'),
  });

export const sendersQuery = () =>
  queryOptions({
    queryKey: ['senders'] as const,
    queryFn: () => apiFetch<SenderRecord[]>('/api/senders'),
  });

export const gmailStatusQuery = () =>
  queryOptions({
    queryKey: ['gmail-status'] as const,
    queryFn: () => apiFetch<GmailStatusView>('/api/gmail/status'),
  });

export const accountsQuery = () =>
  queryOptions({
    queryKey: ['accounts'] as const,
    queryFn: () => apiFetch<AccountSummary[]>('/api/accounts'),
  });

export const accountReconciliationsQuery = (accountId: string) =>
  queryOptions({
    queryKey: ['accounts', 'reconciliations', accountId] as const,
    queryFn: () =>
      apiFetch<ReconciliationRow[]>(
        `/api/accounts/${encodeURIComponent(accountId)}/reconciliations`,
      ),
  });

export const monthlySummaryQuery = (month?: string) =>
  queryOptions({
    queryKey: ['spending', 'summary', month ?? 'current'] as const,
    queryFn: () => {
      const qs = month ? `?month=${encodeURIComponent(month)}` : '';
      return apiFetch<MonthlySpendingSummary>(`/api/spending/summary${qs}`);
    },
  });

export const transactionsQuery = (params?: {
  month?: string;
  startDate?: string;
  endDate?: string;
  category?: string;
  accountId?: string;
  search?: string;
  unreviewedOnly?: boolean;
  page?: number;
  pageSize?: number;
}) =>
  queryOptions({
    queryKey: ['transactions', 'list', params ?? {}] as const,
    queryFn: () => {
      const sp = new URLSearchParams();
      if (params?.month) sp.set('month', params.month);
      if (params?.startDate) sp.set('startDate', params.startDate);
      if (params?.endDate) sp.set('endDate', params.endDate);
      if (params?.category) sp.set('category', params.category);
      if (params?.accountId) sp.set('accountId', params.accountId);
      if (params?.search) sp.set('search', params.search);
      if (params?.unreviewedOnly) sp.set('unreviewedOnly', 'true');
      if (params?.page) sp.set('page', String(params.page));
      if (params?.pageSize) sp.set('pageSize', String(params.pageSize));
      const qs = sp.toString();
      return apiFetch<PaginatedTransactionsResponse>(
        `/api/transactions${qs ? `?${qs}` : ''}`,
      );
    },
  });

export const transactionDetailQuery = (id: string) =>
  queryOptions({
    queryKey: ['transactions', 'detail', id] as const,
    queryFn: () =>
      apiFetch<TransactionDetailView>(
        `/api/transactions/${encodeURIComponent(id)}`,
      ),
  });

export const syncStatusQuery = () =>
  queryOptions({
    queryKey: ['sync-status'] as const,
    queryFn: () => apiFetch<SyncStatusView>('/api/sync'),
  });

// Mutations
export const updateLocaleSettingsApi = (input: {
  country_code: string;
  timezone?: string;
  currency?: string;
}) =>
  apiFetch('/api/settings', {
    method: 'PATCH',
    body: JSON.stringify({ section: 'locale', ...input }),
  });

export const updateLlmSettingsApi = (input: {
  llm_model_path: string | null;
  llm_context_size?: number;
  llm_timeout_ms?: number;
}) =>
  apiFetch('/api/settings', {
    method: 'PATCH',
    body: JSON.stringify({ section: 'llm', ...input }),
  });

export const updateSyncSettingsApi = (input: { sync_batch_limit: number }) =>
  apiFetch('/api/settings', {
    method: 'PATCH',
    body: JSON.stringify({ section: 'sync', ...input }),
  });

export const testLlmModelApi = () =>
  apiFetch<{ ok: true; modelPath: string; contextSize: number }>('/api/settings', {
    method: 'POST',
  });

export const createSenderApi = (input: {
  email: string;
  role_hint?: 'auto' | 'card_charge' | 'receipt';
  default_account_id?: string | null;
  enabled?: boolean;
  backfill_from_utc?: string;
}) =>
  apiFetch<SenderRecord>('/api/senders', {
    method: 'POST',
    body: JSON.stringify(input),
  });

export const updateSenderApi = (input: {
  id: string;
  email?: string;
  role_hint?: 'auto' | 'card_charge' | 'receipt';
  default_account_id?: string | null;
  enabled?: boolean;
  backfill_from_utc?: string;
}) => {
  const { id, ...patch } = input;
  return apiFetch<SenderRecord>(`/api/senders/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(patch),
  });
};

export const deleteSenderApi = (id: string) =>
  apiFetch(`/api/senders/${encodeURIComponent(id)}`, { method: 'DELETE' });

export const disconnectGmailApi = () =>
  apiFetch('/api/gmail/status', { method: 'DELETE' });

export const createAccountApi = (input: {
  name: string;
  type: AccountType;
  initial_balance_minor: number;
  initial_balance_as_of_utc?: string;
  last4?: string | null;
  identifiers?: AccountIdentifierInput[];
}) =>
  apiFetch<AccountSummary>('/api/accounts', {
    method: 'POST',
    body: JSON.stringify(input),
  });

export const updateAccountApi = (input: {
  id: string;
  name?: string;
  type?: AccountType;
  initial_balance_minor?: number | null;
  initial_balance_as_of_utc?: string;
  last4?: string | null;
  identifiers?: AccountIdentifierInput[];
}) => {
  const { id, ...patch } = input;
  return apiFetch<AccountSummary>(`/api/accounts/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(patch),
  });
};

export const confirmAccountApi = (id: string) =>
  apiFetch<AccountSummary>(`/api/accounts/${encodeURIComponent(id)}/confirm`, {
    method: 'POST',
  });

export const deleteAccountApi = (id: string) =>
  apiFetch(`/api/accounts/${encodeURIComponent(id)}`, { method: 'DELETE' });

export const reconcileAccountApi = (input: {
  account_id: string;
  actual_balance_minor: number;
  note?: string | null;
  recorded_at_utc?: string;
}) =>
  apiFetch(`/api/accounts/${encodeURIComponent(input.account_id)}/reconciliations`, {
    method: 'POST',
    body: JSON.stringify(input),
  });

export const mergeAccountsApi = (input: {
  sourceAccountId: string;
  targetAccountId: string;
}) =>
  apiFetch('/api/accounts/merge', {
    method: 'POST',
    body: JSON.stringify(input),
  });

export const addManualTransactionApi = (input: {
  amount_minor: number;
  occurred_at_utc: string;
  merchant: string;
  category?: string;
  account_id?: string | null;
  note?: string | null;
}) =>
  apiFetch<TransactionListItem>('/api/transactions', {
    method: 'POST',
    body: JSON.stringify(input),
  });

export const updateTransactionApi = (input: {
  id: string;
  amount_minor?: number;
  occurred_at_utc?: string;
  merchant?: string;
  category?: string;
  account_id?: string | null;
  note?: string | null;
  convertToLocalCurrency?: boolean;
}) => {
  const { id, ...patch } = input;
  return apiFetch<TransactionListItem>(
    `/api/transactions/${encodeURIComponent(id)}`,
    {
      method: 'PATCH',
      body: JSON.stringify(patch),
    },
  );
};

export const deleteTransactionApi = (id: string) =>
  apiFetch(`/api/transactions/${encodeURIComponent(id)}`, { method: 'DELETE' });

export const splitTransactionSourceApi = (input: {
  transactionId: string;
  emailId: string;
}) =>
  apiFetch<{ transactionId: string }>(
    `/api/transactions/${encodeURIComponent(input.transactionId)}/split`,
    {
      method: 'POST',
      body: JSON.stringify({ emailId: input.emailId }),
    },
  );

export const duplicateMergeTransactionsApi = (input: {
  keepTransactionId: string;
  dropTransactionId: string;
}) =>
  apiFetch<TransactionListItem>('/api/transactions/merge', {
    method: 'POST',
    body: JSON.stringify({ mode: 'duplicate', ...input }),
  });

export const combineTransactionsApi = (input: {
  transactionIds: string[];
  merchant: string;
  category?: string;
  account_id?: string | null;
  occurred_at_utc: string;
  note?: string | null;
}) =>
  apiFetch<TransactionListItem>('/api/transactions/merge', {
    method: 'POST',
    body: JSON.stringify({ mode: 'combine', ...input }),
  });

export const markTransactionsReviewedApi = (transactionIds: string[]) =>
  apiFetch<{ updatedCount: number }>('/api/transactions/review', {
    method: 'POST',
    body: JSON.stringify({ transactionIds }),
  });

export const triggerSyncApi = () =>
  apiFetch<{ runId: string }>('/api/sync', {
    method: 'POST',
    body: JSON.stringify({ action: 'trigger' }),
  });

export const retryFailedExtractionsApi = () =>
  apiFetch<{ runId: string }>('/api/sync', {
    method: 'POST',
    body: JSON.stringify({ action: 'retry' }),
  });

export const cancelSyncApi = () =>
  apiFetch<{ cancelled: boolean }>('/api/sync', {
    method: 'POST',
    body: JSON.stringify({ action: 'cancel' }),
  });

export const dismissPendingQueueApi = () =>
  apiFetch<{ dismissedCount: number }>('/api/sync', {
    method: 'POST',
    body: JSON.stringify({ action: 'dismiss' }),
  });
