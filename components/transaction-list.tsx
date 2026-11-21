'use client';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
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
import type { AccountSummary, TransactionListItem } from '@/lib/api';
import { DEFAULT_CATEGORIES } from '@/lib/categories';
import { formatMinorCurrency, formatUtcInTimezone } from '@/lib/format';

function sourceBadgeLabel(tx: TransactionListItem): string {
  if (tx.source_type === 'manual') return 'Manual';
  if (tx.sourceCount > 1) return `${tx.sourceCount} merged`;
  if (tx.sourceCount === 1) return '1 email';
  return '0 emails';
}

export function TransactionList({
  transactions,
  accounts,
  timezone,
  search,
  onSearchChange,
  category,
  onCategoryChange,
  accountId,
  onAccountIdChange,
  unmergedOnly,
  onUnmergedOnlyChange,
  onSelectTransaction,
}: {
  transactions: TransactionListItem[];
  accounts: AccountSummary[];
  timezone: string | null | undefined;
  search: string;
  onSearchChange: (value: string) => void;
  category: string;
  onCategoryChange: (value: string) => void;
  accountId: string;
  onAccountIdChange: (value: string) => void;
  unmergedOnly: boolean;
  onUnmergedOnlyChange: (value: boolean) => void;
  onSelectTransaction: (id: string) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="grid gap-3 md:grid-cols-4">
          <Input
            type="search"
            aria-label="Search transactions"
            placeholder="Search merchant or note..."
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
          />

          <Select
            value={category}
            onValueChange={(value) => onCategoryChange(value ?? 'all')}
          >
            <SelectTrigger className="w-full" aria-label="Filter by category">
              <SelectValue placeholder="All categories" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All categories</SelectItem>
              {DEFAULT_CATEGORIES.map((cat) => (
                <SelectItem key={cat} value={cat}>
                  {cat}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            value={accountId}
            onValueChange={(value) => onAccountIdChange(value ?? 'all')}
          >
            <SelectTrigger className="w-full" aria-label="Filter by account">
              <SelectValue placeholder="All accounts" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All accounts</SelectItem>
              <SelectItem value="unassigned">Unassigned</SelectItem>
              {accounts.map((acct) => (
                <SelectItem key={acct.id} value={acct.id}>
                  {acct.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <div className="flex items-center gap-2">
            <Checkbox
              id="unmerged-filter"
              checked={unmergedOnly}
              onCheckedChange={(checked) => onUnmergedOnlyChange(Boolean(checked))}
            />
            <Label htmlFor="unmerged-filter">Unmerged / Needs review</Label>
          </div>
        </div>
      </CardHeader>

      <CardContent>
        {transactions.length === 0 ? (
          <p className="py-8 text-center text-sm text-muted-foreground">
            No transactions match the current filters.
          </p>
        ) : (
          <ul className="space-y-1">
            {transactions.map((tx, idx) => (
              <li key={tx.id}>
                {idx > 0 && <Separator className="my-1" />}
                <Button
                  type="button"
                  variant="ghost"
                  aria-label={`Inspect ${tx.merchant} (${formatMinorCurrency(tx.amount_minor, tx.currency)})`}
                  onClick={() => onSelectTransaction(tx.id)}
                  className="flex h-auto w-full items-center justify-between gap-4 px-3 py-3 text-left font-normal"
                >
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="truncate font-medium">{tx.merchant}</span>
                      <Badge variant="secondary">{tx.category}</Badge>
                      <Badge variant="outline">{sourceBadgeLabel(tx)}</Badge>
                      {tx.manual_override === 1 && (
                        <Badge variant="secondary">manual_override</Badge>
                      )}
                      {tx.isForeignCurrency && (
                        <Badge variant="outline">Foreign ({tx.currency})</Badge>
                      )}
                      {tx.pendingSuggestionCount > 0 && (
                        <Badge>
                          {tx.pendingSuggestionCount} merge{' '}
                          {tx.pendingSuggestionCount === 1
                            ? 'suggestion'
                            : 'suggestions'}
                        </Badge>
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground">
                      {tx.account_name ??
                        (tx.payment_method_raw
                          ? `Unassigned (${tx.payment_method_raw})`
                          : 'Unassigned')}{' '}
                      · {formatUtcInTimezone(tx.occurred_at_utc, timezone)}
                    </p>
                  </div>
                  <span className="shrink-0 font-medium tabular-nums">
                    {formatMinorCurrency(tx.amount_minor, tx.currency)}
                  </span>
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
