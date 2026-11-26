"use client"

import { IconTooltip } from "@/components/icon-tooltip"
import { CalendarIcon, XIcon } from "lucide-react"
import { lazy, Suspense, useId, useState } from "react"
import type { DateRange } from "react-day-picker"

import { Button } from "@/components/ui/button"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  dateToIsoDateString,
  formatDateRangeLabel,
  formatDateString,
  isoDateStringToDate,
} from "@/lib/format"
import { cn } from "@/lib/utils"

const loadCalendar = () => import("@/components/ui/calendar")
const Calendar = lazy(() =>
  loadCalendar().then(({ Calendar }) => ({ default: Calendar }))
)

function preloadCalendar() {
  void loadCalendar().catch(() => {})
}

function isKeyboardActivation(event: Event) {
  return (
    event instanceof KeyboardEvent ||
    (event instanceof MouseEvent && event.detail === 0)
  )
}

const calendarLoading = (
  <div role="status" className="p-4 text-muted-foreground">
    Loading calendar…
  </div>
)

export function DatePicker({
  value,
  onChange,
  placeholder = "Pick a date",
  clearable = false,
  disabled = false,
  id,
  ariaLabel,
  className,
  time,
}: {
  value: string
  onChange: (next: string) => void
  placeholder?: string
  clearable?: boolean
  disabled?: boolean
  id?: string
  ariaLabel?: string
  className?: string
  time?: { value: string; onChange: (next: string) => void; step?: number }
}) {
  const [open, setOpen] = useState(false)
  const [keyboardOpen, setKeyboardOpen] = useState(false)
  const timeId = useId()
  const selected = value ? isoDateStringToDate(value) : undefined
  const displayValue = value
    ? [formatDateString(value), time?.value].filter(Boolean).join(" · ")
    : placeholder

  if (disabled && open) setOpen(false)

  return (
    <div className={cn("flex items-center gap-1", className)}>
      <Popover
        open={open}
        onOpenChange={(nextOpen, { event }) => {
          if (nextOpen) setKeyboardOpen(isKeyboardActivation(event))
          setOpen(nextOpen)
        }}
      >
        <PopoverTrigger
          onPointerEnter={preloadCalendar}
          onFocus={preloadCalendar}
          render={
            <Button
              id={id}
              type="button"
              variant="outline"
              disabled={disabled}
              aria-label={
                time && ariaLabel ? `${ariaLabel}, ${displayValue}` : ariaLabel
              }
              className={cn(
                "min-w-0 flex-1 justify-start text-left font-normal",
                !value && "text-muted-foreground"
              )}
            />
          }
        >
          <CalendarIcon className="mr-2 size-4 shrink-0" />
          <span className="truncate">{displayValue}</span>
        </PopoverTrigger>
        <PopoverContent
          className={cn(
            "w-auto p-0",
            time && "max-h-(--available-height) gap-0 overflow-y-auto"
          )}
          align="start"
          aria-label={time ? ariaLabel : undefined}
        >
          <Suspense fallback={calendarLoading}>
            <Calendar
              autoFocus={keyboardOpen}
              mode="single"
              selected={selected}
              defaultMonth={selected}
              onSelect={(day) => {
                if (!day) {
                  if (clearable) {
                    onChange("")
                    setOpen(false)
                  }
                  return
                }
                onChange(dateToIsoDateString(day))
                if (!time) setOpen(false)
              }}
            />
          </Suspense>
          {time && (
            <Field className="border-t p-3">
              <FieldLabel htmlFor={timeId}>Time</FieldLabel>
              <Input
                id={timeId}
                type="time"
                step={time.step}
                required
                value={time.value}
                onChange={(event) => time.onChange(event.target.value)}
              />
            </Field>
          )}
        </PopoverContent>
      </Popover>

      {clearable && value && !disabled && (
        <IconTooltip label="Clear date">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="Clear date"
            className="shrink-0"
            onClick={() => onChange("")}
          >
            <XIcon className="size-3.5" />
          </Button>
        </IconTooltip>
      )}
    </div>
  )
}

export function DateRangePicker({
  from,
  to,
  onChange,
  disabled = false,
  ariaLabel = "Filter by date range",
  className,
}: {
  from: string
  to: string
  onChange: (next: { from: string; to: string }) => void
  disabled?: boolean
  ariaLabel?: string
  className?: string
}) {
  const [open, setOpen] = useState(false)
  const [keyboardOpen, setKeyboardOpen] = useState(false)
  const [draft, setDraft] = useState<DateRange | undefined>(undefined)

  const committedFrom = isoDateStringToDate(from)
  const committedTo = isoDateStringToDate(to || from)
  const committedRange: DateRange | undefined = committedFrom
    ? { from: committedFrom, to: committedTo ?? committedFrom }
    : undefined

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      setDraft(committedRange)
      setOpen(true)
      return
    }
    if (draft?.from && !draft.to) {
      const single = dateToIsoDateString(draft.from)
      onChange({ from: single, to: single })
    }
    setDraft(undefined)
    setOpen(false)
  }

  return (
    <Popover
      open={open}
      onOpenChange={(nextOpen, { event }) => {
        if (nextOpen) setKeyboardOpen(isKeyboardActivation(event))
        handleOpenChange(nextOpen)
      }}
    >
      <PopoverTrigger
        onPointerEnter={preloadCalendar}
        onFocus={preloadCalendar}
        render={
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled}
            aria-label={ariaLabel}
            className={cn("justify-start text-left font-normal", className)}
          />
        }
      >
        <CalendarIcon className="mr-2 size-4 shrink-0" />
        <span className="truncate">{formatDateRangeLabel(from, to)}</span>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Suspense fallback={calendarLoading}>
          <Calendar
            autoFocus={keyboardOpen}
            mode="range"
            resetOnSelect
            required
            numberOfMonths={1}
            selected={open ? draft : committedRange}
            defaultMonth={committedFrom}
            onSelect={(nextRange) => {
              if (!nextRange?.from) {
                setDraft(undefined)
                return
              }
              if (nextRange.to) {
                onChange({
                  from: dateToIsoDateString(nextRange.from),
                  to: dateToIsoDateString(nextRange.to),
                })
                setDraft(undefined)
                setOpen(false)
                return
              }
              setDraft({ from: nextRange.from, to: undefined })
            }}
          />
        </Suspense>
      </PopoverContent>
    </Popover>
  )
}
