"use client"

import { keepPreviousData, useQuery } from "@tanstack/react-query"
import Link from "next/link"
import { useId, useState } from "react"

import { BreakdownPanels } from "@/components/breakdown-panels"
import { MonthlySummary } from "@/components/monthly-summary"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { categoriesQuery } from "@/lib/api/categories"
import { tagsQuery } from "@/lib/api/tags"
import { monthlySummaryQuery } from "@/lib/api/transactions"
import { shiftYearMonth } from "@/lib/format"

const MONTH_OPTIONS = [
  { value: "01", label: "January" },
  { value: "02", label: "February" },
  { value: "03", label: "March" },
  { value: "04", label: "April" },
  { value: "05", label: "May" },
  { value: "06", label: "June" },
  { value: "07", label: "July" },
  { value: "08", label: "August" },
  { value: "09", label: "September" },
  { value: "10", label: "October" },
  { value: "11", label: "November" },
  { value: "12", label: "December" },
]

function parseYearMonth(yearMonth: string): { year: string; month: string } {
  const match = /^(\d{4})-(\d{2})$/.exec(yearMonth.trim())
  if (!match) return { year: "", month: "" }
  return { year: match[1], month: match[2] }
}

function buildYearOptions(activeYear: string): string[] {
  const center = /^\d{4}$/.test(activeYear)
    ? Number(activeYear)
    : new Date().getUTCFullYear()
  const years: string[] = []
  for (let y = center - 3; y <= center + 1; y += 1) {
    years.push(String(y))
  }
  return years
}

function ReportFilter({
  label,
  allLabel,
  entries,
  value,
  onValueChange,
  unavailable,
}: {
  label: string
  allLabel: string
  entries: { id: string; name: string }[]
  value: string | undefined
  onValueChange: (value: string) => void
  unavailable: boolean
}) {
  const id = useId()
  const items = [
    { value: "", label: allLabel },
    ...entries.map((entry) => ({ value: entry.id, label: entry.name })),
  ]
  return (
    <div className="grid gap-1">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      <Select
        items={items}
        value={value ?? ""}
        disabled={unavailable}
        onValueChange={(nextValue) => onValueChange(nextValue ?? "")}
      >
        <SelectTrigger id={id} className="min-w-40">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

export function DashboardView() {
  const [selectedMonth, setSelectedMonth] = useState<string | undefined>(
    undefined
  )

  const [categoryId, setCategoryId] = useState("")
  const [tagId, setTagId] = useState("")
  const categories = useQuery(categoriesQuery())
  const tags = useQuery(tagsQuery())
  const category = categories.data?.find((entry) => entry.id === categoryId)
  const tag = tags.data?.find((entry) => entry.id === tagId)
  const summaryQuery = useQuery({
    ...monthlySummaryQuery(selectedMonth, {
      category: category?.name,
      tag: tag?.id,
    }),
    placeholderData: keepPreviousData,
  })

  const activeMonth = selectedMonth ?? summaryQuery.data?.month ?? ""
  const activeParts = parseYearMonth(activeMonth)
  const yearItems = buildYearOptions(activeParts.year).map((yr) => ({
    value: yr,
    label: yr,
  }))
  const isConfigured = Boolean(summaryQuery.data?.configured)

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">
            Monthly Spending
          </h1>
          <p className="text-sm text-muted-foreground">
            Track spending across synced emails and manual entries.
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!isConfigured || !activeMonth}
            onClick={() => setSelectedMonth(shiftYearMonth(activeMonth, -1))}
          >
            Prev
          </Button>

          <Select
            items={MONTH_OPTIONS}
            value={activeParts.month || null}
            disabled={!isConfigured}
            onValueChange={(nextMonth) => {
              if (nextMonth && activeParts.year) {
                setSelectedMonth(`${activeParts.year}-${nextMonth}`)
              }
            }}
          >
            <SelectTrigger className="w-36" aria-label="Select month">
              <SelectValue placeholder="Month" />
            </SelectTrigger>
            <SelectContent>
              {MONTH_OPTIONS.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            items={yearItems}
            value={activeParts.year || null}
            disabled={!isConfigured}
            onValueChange={(nextYear) => {
              if (nextYear && activeParts.month) {
                setSelectedMonth(`${nextYear}-${activeParts.month}`)
              }
            }}
          >
            <SelectTrigger className="w-24" aria-label="Select year">
              <SelectValue placeholder="Year" />
            </SelectTrigger>
            <SelectContent>
              {yearItems.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={!isConfigured || !activeMonth}
            onClick={() => setSelectedMonth(shiftYearMonth(activeMonth, 1))}
          >
            Next
          </Button>

          <Link
            href="/transactions"
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            View transactions
          </Link>
        </div>
      </div>

      {isConfigured && (
        <section aria-label="Spending filters" className="flex flex-wrap gap-3">
          <ReportFilter
            label="Category"
            allLabel="All categories"
            entries={categories.data ?? []}
            value={category?.id}
            onValueChange={setCategoryId}
            unavailable={!categories.data}
          />
          <ReportFilter
            label="Tag"
            allLabel="All tags"
            entries={tags.data ?? []}
            value={tag?.id}
            onValueChange={setTagId}
            unavailable={!tags.data}
          />
        </section>
      )}

      {summaryQuery.isPending || summaryQuery.isPlaceholderData ? (
        <div className="space-y-4">
          <Skeleton className="h-32 w-full" />
          <div className="grid gap-4 md:grid-cols-3">
            <Skeleton className="h-40 w-full" />
            <Skeleton className="h-40 w-full" />
            <Skeleton className="h-40 w-full" />
          </div>
        </div>
      ) : summaryQuery.isError ? (
        <Alert variant="destructive">
          <AlertTitle>Couldn’t load spending</AlertTitle>
          <AlertDescription>
            <p>{summaryQuery.error.message}</p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => void summaryQuery.refetch()}
            >
              Try again
            </Button>
          </AlertDescription>
        </Alert>
      ) : summaryQuery.data.configured ? (
        <>
          <MonthlySummary summary={summaryQuery.data} />
          <BreakdownPanels summary={summaryQuery.data} />
        </>
      ) : (
        <Alert>
          <AlertTitle>Finish initial setup</AlertTitle>
          <AlertDescription className="mt-2 flex flex-wrap items-center justify-between gap-3">
            <span>
              Configure your country and currency in Settings to start tracking
              monthly spending.
            </span>
            <Link href="/settings" className={buttonVariants({ size: "sm" })}>
              Go to Settings
            </Link>
          </AlertDescription>
        </Alert>
      )}
    </div>
  )
}
