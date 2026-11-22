'use client';

import { CalendarIcon, XIcon } from 'lucide-react';
import { useState } from 'react';
import type { DateRange } from 'react-day-picker';

import { Button } from '@/components/ui/button';
import { Calendar } from '@/components/ui/calendar';
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover';
import {
  dateToIsoDateString,
  formatDateRangeLabel,
  formatDateString,
  isoDateStringToDate,
} from '@/lib/format';
import { cn } from '@/lib/utils';

export function DatePicker({
  value,
  onChange,
  placeholder = 'Pick a date',
  clearable = false,
  disabled = false,
  id,
  ariaLabel,
  className,
}: {
  value: string;
  onChange: (next: string) => void;
  placeholder?: string;
  clearable?: boolean;
  disabled?: boolean;
  id?: string;
  ariaLabel?: string;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  const selected = value ? isoDateStringToDate(value) : undefined;

  return (
    <div className={cn('flex items-center gap-1', className)}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              id={id}
              type="button"
              variant="outline"
              disabled={disabled}
              aria-label={ariaLabel}
              className={cn(
                'min-w-0 flex-1 justify-start text-left font-normal',
                !value && 'text-muted-foreground',
              )}
            />
          }
        >
          <CalendarIcon className="mr-2 size-4 shrink-0" />
          <span className="truncate">
            {value ? formatDateString(value) : placeholder}
          </span>
        </PopoverTrigger>
        <PopoverContent className="w-auto p-0" align="start">
          <Calendar
            mode="single"
            selected={selected}
            defaultMonth={selected}
            onSelect={(day) => {
              if (!day) {
                if (clearable) {
                  onChange('');
                  setOpen(false);
                }
                return;
              }
              onChange(dateToIsoDateString(day));
              setOpen(false);
            }}
          />
        </PopoverContent>
      </Popover>

      {clearable && value && !disabled && (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Clear date"
          className="shrink-0"
          onClick={() => onChange('')}
        >
          <XIcon className="size-3.5" />
        </Button>
      )}
    </div>
  );
}

export function DateRangePicker({
  from,
  to,
  onChange,
  disabled = false,
  ariaLabel = 'Filter by date range',
  className,
}: {
  from: string;
  to: string;
  onChange: (next: { from: string; to: string }) => void;
  disabled?: boolean;
  ariaLabel?: string;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<DateRange | undefined>(undefined);

  const committedFrom = isoDateStringToDate(from);
  const committedTo = isoDateStringToDate(to || from);
  const committedRange: DateRange | undefined = committedFrom
    ? { from: committedFrom, to: committedTo ?? committedFrom }
    : undefined;

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      setDraft(committedRange);
      setOpen(true);
      return;
    }
    if (draft?.from && !draft.to) {
      const single = dateToIsoDateString(draft.from);
      onChange({ from: single, to: single });
    }
    setDraft(undefined);
    setOpen(false);
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled}
            aria-label={ariaLabel}
            className={cn('justify-start text-left font-normal', className)}
          />
        }
      >
        <CalendarIcon className="mr-2 size-4 shrink-0" />
        <span className="truncate">{formatDateRangeLabel(from, to)}</span>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="range"
          resetOnSelect
          required
          numberOfMonths={1}
          selected={open ? draft : committedRange}
          defaultMonth={committedFrom}
          onSelect={(nextRange) => {
            if (!nextRange?.from) {
              setDraft(undefined);
              return;
            }
            if (nextRange.to) {
              onChange({
                from: dateToIsoDateString(nextRange.from),
                to: dateToIsoDateString(nextRange.to),
              });
              setDraft(undefined);
              setOpen(false);
              return;
            }
            setDraft({ from: nextRange.from, to: undefined });
          }}
        />
      </PopoverContent>
    </Popover>
  );
}
