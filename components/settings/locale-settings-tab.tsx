"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useMemo, useState, type SubmitEvent } from "react"

import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { updateLocaleSettingsApi } from "@/lib/api/settings"
import { type SavedSettingsView } from "@/lib/api/types.generated"

export function LocaleSettingsTab({
  settings,
}: {
  settings: SavedSettingsView | undefined
}) {
  const queryClient = useQueryClient()
  const [countryDraft, setCountryDraft] = useState<string | null>(null)
  const [timezoneDraft, setTimezoneDraft] = useState<string | null>(null)
  const [localeMessage, setLocaleMessage] = useState<string | null>(null)

  const rawCountryCode = countryDraft ?? settings?.country_code ?? ""
  const countryCode = rawCountryCode.trim().toUpperCase()
  const timezone = timezoneDraft ?? settings?.timezone ?? ""

  const selectedCountry = (settings?.countryCatalog ?? []).find(
    (c) => c.code === countryCode
  )
  const savedCountryCode = (settings?.country_code ?? "").trim().toUpperCase()
  const hasUnsupportedSavedCountry =
    savedCountryCode !== "" &&
    !(settings?.countryCatalog ?? []).some((c) => c.code === savedCountryCode)
  const canRepairLockedCountry =
    Boolean(settings?.currencyLocked) &&
    !selectedCountry &&
    (settings?.countryCatalog ?? []).some(
      (c) => c.currency === settings?.currency
    )

  const countryItems = useMemo(
    () =>
      (settings?.countryCatalog ?? []).flatMap((c) =>
        canRepairLockedCountry && c.currency !== settings?.currency
          ? []
          : [
              {
                value: c.code,
                label: `${c.name} (${c.currency})`,
              },
            ]
      ),
    [settings?.countryCatalog, settings?.currency, canRepairLockedCountry]
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

  const localeMutation = useMutation({
    mutationFn: updateLocaleSettingsApi,
    onSuccess: () => {
      setLocaleMessage("Locale settings saved.")
      void queryClient.invalidateQueries({ queryKey: ["settings"] })
      void queryClient.invalidateQueries({ queryKey: ["spending"] })
      void queryClient.invalidateQueries({ queryKey: ["transactions"] })
      void queryClient.invalidateQueries({ queryKey: ["triage"] })
      void queryClient.invalidateQueries({ queryKey: ["accounts"] })
    },
    onError: (err) => {
      setLocaleMessage(
        err instanceof Error ? err.message : "Failed to save locale."
      )
    },
  })

  function handleLocaleSubmit(e: SubmitEvent<HTMLFormElement>) {
    e.preventDefault()
    if (!countryCode) return
    localeMutation.mutate({
      country_code: countryCode,
      timezone: timezone || undefined,
      currency: settings?.currencyLocked
        ? (settings.currency ?? undefined)
        : selectedCountry?.currency,
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Country, Timezone &amp; Currency</CardTitle>
        <CardDescription>
          All monthly reports and account balances use your selected
          country&apos;s IANA timezone and ISO 4217 currency.
        </CardDescription>
        {settings?.currency && (
          <CardAction>
            <Badge variant="secondary" className="font-mono">
              {settings.currency}
            </Badge>
          </CardAction>
        )}
      </CardHeader>
      <CardContent className="space-y-4">
        {hasUnsupportedSavedCountry && (
          <p className="rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-amber-900 dark:text-amber-200">
            Saved country <strong>{savedCountryCode}</strong> is not present in{" "}
            <code>countries/</code>.{" "}
            {settings?.currencyLocked
              ? `Add countries/${savedCountryCode}.json to keep using ${settings.currency}.`
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
                Boolean(settings?.currencyLocked) && !canRepairLockedCountry
              }
              onValueChange={(nextCode) => {
                const code = nextCode ?? ""
                setCountryDraft(code)
                const match = settings?.countryCatalog.find(
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
              {settings?.currencyLocked
                ? selectedCountry
                  ? `Currency (${settings.currency}) is locked because accounts or transactions exist. Timezone remains editable.`
                  : `Currency (${settings.currency}) is locked because accounts or transactions exist. Select a matching country or add its country file.`
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
  )
}
