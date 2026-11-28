"use client"

import { ImportPreviewTable } from "./import-preview-table"
import { ImportJsonInput } from "./import-json-input"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import {
  AlertCircleIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  CheckCircle2Icon,
  CheckIcon,
  CopyIcon,
  RotateCcwIcon,
  SparklesIcon,
} from "lucide-react"
import Link from "next/link"
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
} from "react"

import { LocaleSetupAlert } from "@/components/locale/locale-setup-alert"
import { PageLayout } from "@/components/PageLayout"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { accountOptionsQuery } from "@/lib/api/accounts"
import { categoriesQuery } from "@/lib/api/categories"
import { localeQuery } from "@/lib/api/settings"
import { tagsQuery } from "@/lib/api/tags"
import {
  importManualTransactionsApi,
  invalidateTransactionRelatedQueries,
} from "@/lib/api/transactions"
import { AccountSelect } from "./import-account-select"
import {
  buildManualImportPrompt,
  parseManualImportJson,
  type ParseManualImportResult,
} from "@/lib/manual-import"

type WizardStep = 1 | 2 | 3

const WIZARD_STEPS: ReadonlyArray<{ step: WizardStep; label: string }> = [
  { step: 1, label: "Generate prompt" },
  { step: 2, label: "Upload or paste JSON" },
  { step: 3, label: "Preview & import" },
]

export function ManualImportView() {
  const queryClient = useQueryClient()
  const localeDataQuery = useQuery(localeQuery())
  const accountsDataQuery = useQuery(accountOptionsQuery())
  const categoriesDataQuery = useQuery(categoriesQuery())
  const tagsDataQuery = useQuery(tagsQuery())

  const timezone = localeDataQuery.data?.timezone ?? ""
  const currency = localeDataQuery.data?.currency ?? ""
  const isConfigured = Boolean(timezone && currency)

  const accounts = useMemo(
    () => accountsDataQuery.data ?? [],
    [accountsDataQuery.data]
  )
  const categoryNames = useMemo(
    () => (categoriesDataQuery.data ?? []).map((c) => c.name),
    [categoriesDataQuery.data]
  )
  const tagNames = useMemo(
    () => (tagsDataQuery.data ?? []).map((t) => t.name),
    [tagsDataQuery.data]
  )

  const [requestedStep, setStep] = useState<WizardStep>(1)
  const [selectedAccountId, setSelectedAccountId] = useState<string>("")
  const [customInstructions, setCustomInstructions] = useState("")
  const [copied, setCopied] = useState(false)
  const [rawJson, setRawJson] = useState("")
  const [deselectedIndices, setDeselectedIndices] = useState<Set<number>>(
    () => new Set()
  )
  const copyTimerRef = useRef<number | null>(null)

  useEffect(() => {
    return () => {
      if (copyTimerRef.current !== null) {
        window.clearTimeout(copyTimerRef.current)
      }
    }
  }, [])

  const effectiveAccountId =
    selectedAccountId || (accounts.length > 0 ? accounts[0].id : "")
  const selectedAccount = accounts.find((a) => a.id === effectiveAccountId)

  const accountSelectItems = useMemo(
    () =>
      accounts.map((a) => ({
        value: a.id,
        label: `${a.name} (${a.currency})`,
      })),
    [accounts]
  )

  const generatedPrompt = useMemo(() => {
    if (!isConfigured || !selectedAccount) return ""
    return buildManualImportPrompt({
      accountName: selectedAccount.name,
      accountType: selectedAccount.type,
      currency,
      timezone,
      categories: categoryNames,
      tags: tagNames,
      customInstructions,
    })
  }, [
    isConfigured,
    selectedAccount,
    currency,
    timezone,
    categoryNames,
    tagNames,
    customInstructions,
  ])

  const parseOptions = useMemo(
    () => ({
      defaultCurrency: currency,
      timezone,
      categories: categoryNames,
      tags: tagNames,
    }),
    [currency, timezone, categoryNames, tagNames]
  )

  const validationContext = isConfigured ? parseOptions : null
  const currentContext = useRef(validationContext)
  useLayoutEffect(() => {
    currentContext.current = validationContext
  }, [validationContext])

  const [validation, setValidation] = useState<{
    context: typeof parseOptions
    result: ParseManualImportResult
  } | null>(null)
  const [isReadingFile, setIsReadingFile] = useState(false)
  const [fileError, setFileError] = useState("")
  const inputRevision = useRef(0)
  const submitting = useRef(false)
  const currentResult =
    validation?.context === validationContext ? validation.result : null
  const canPreview = Boolean(
    !isReadingFile &&
    currentResult &&
    !currentResult.error &&
    currentResult.rows.length > 0
  )
  const canValidate = Boolean(
    !isReadingFile && validationContext && rawJson.trim()
  )

  const importMutation = useMutation({
    mutationFn: ({
      request,
    }: {
      request: Parameters<typeof importManualTransactionsApi>[0]
      accountName: string
    }) => importManualTransactionsApi(request),
    onSuccess: () => {
      setRawJson("")
      setValidation(null)
      setDeselectedIndices(new Set())
      invalidateTransactionRelatedQueries(queryClient)
    },
    onSettled: () => {
      submitting.current = false
    },
  })

  const parsedResult = importMutation.isPending
    ? validation?.result
    : currentResult
  const validRows = useMemo(
    () => (parsedResult?.rows ?? []).filter((r) => r.valid),
    [parsedResult]
  )
  const invalidRowsCount = (parsedResult?.rows.length ?? 0) - validRows.length
  const selectedValidRows = useMemo(
    () => validRows.filter((r) => !deselectedIndices.has(r.index)),
    [validRows, deselectedIndices]
  )

  const receipt = importMutation.data
  const importedCount = receipt?.importedCount ?? null
  const importedAccountName = receipt
    ? importMutation.variables?.accountName
    : undefined
  const step =
    requestedStep === 3 && !canPreview && !receipt && !importMutation.isPending
      ? 2
      : requestedStep

  function updateRawJson(nextJson: string) {
    inputRevision.current += 1
    setIsReadingFile(false)
    setRawJson(nextJson)
    setFileError("")
    setValidation(null)
    setDeselectedIndices(new Set())
    importMutation.reset()
  }

  function preview(raw: string, context: typeof validationContext) {
    if (!context || !raw.trim()) return
    const result = parseManualImportJson(raw, context)
    setValidation({ context, result })
    setDeselectedIndices(new Set())
    if (!result.error && result.rows.length > 0) setStep(3)
  }

  function openPreview() {
    if (receipt || canPreview) {
      setStep(3)
    } else if (!currentResult) {
      preview(rawJson, validationContext)
    }
  }

  function submitImport() {
    if (submitting.current || !canPreview || !selectedAccount) return
    const transactions = selectedValidRows.flatMap((row) =>
      row.payload ? [row.payload] : []
    )
    if (transactions.length === 0) return
    submitting.current = true
    inputRevision.current += 1
    importMutation.mutate({
      request: { account_id: selectedAccount.id, transactions },
      accountName: selectedAccount.name,
    })
  }

  async function handleCopyPrompt() {
    if (!generatedPrompt) return
    await navigator.clipboard.writeText(generatedPrompt)
    setCopied(true)
    if (copyTimerRef.current !== null) {
      window.clearTimeout(copyTimerRef.current)
    }
    copyTimerRef.current = window.setTimeout(() => {
      setCopied(false)
      copyTimerRef.current = null
    }, 2000)
  }

  async function handleFileUpload(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0]
    event.target.value = ""
    if (!file) return

    const revision = ++inputRevision.current
    setIsReadingFile(true)
    setFileError("")
    let content: string
    try {
      content = await file.text()
    } catch {
      if (revision === inputRevision.current) {
        setIsReadingFile(false)
        setFileError("Could not read the JSON file. Select it again to retry.")
      }
      return
    }
    if (revision !== inputRevision.current) return

    updateRawJson(content)
    if (!content.trim()) {
      setFileError("The JSON file is empty.")
      return
    }
    preview(content, currentContext.current)
  }

  function handleStartOver() {
    updateRawJson("")
    setStep(1)
  }

  const toggleRowSelection = useCallback((index: number, checked: boolean) => {
    setDeselectedIndices((prev) => {
      const next = new Set(prev)
      if (checked) {
        next.delete(index)
      } else {
        next.add(index)
      }
      return next
    })
  }, [])

  function toggleAllValid(checked: boolean) {
    if (checked) {
      setDeselectedIndices(new Set())
    } else {
      setDeselectedIndices(new Set(validRows.map((r) => r.index)))
    }
  }

  const allValidSelected =
    validRows.length > 0 && selectedValidRows.length === validRows.length

  return (
    <PageLayout
      title="Manual Import"
      subtitle="Import bank statement PDFs by generating a tailored prompt for an external chat LLM (Gemini, ChatGPT, etc.) and uploading or pasting the JSON back here."
    >
      {!localeDataQuery.isLoading && !isConfigured && (
        <LocaleSetupAlert showTitle>
          Configure your currency and timezone in Settings before generating an
          import prompt.
        </LocaleSetupAlert>
      )}

      <nav aria-label="Import steps" className="grid gap-2 sm:grid-cols-3">
        {WIZARD_STEPS.map(({ step: s, label }) => {
          const isActive = step === s
          const isCompleted =
            (s === 1 && step > 1) ||
            (s === 2 && (step > 2 || canPreview)) ||
            (s === 3 && importedCount !== null)
          const isDisabled =
            importMutation.isPending ||
            (s === 3 && !canValidate && importedCount === null)

          return (
            <button
              key={s}
              type="button"
              disabled={isDisabled}
              aria-current={isActive ? "step" : undefined}
              onClick={() => (s === 3 ? openPreview() : setStep(s))}
              className={`flex items-center gap-3 rounded-lg border px-3.5 py-2.5 text-left text-sm transition-colors ${
                isActive
                  ? "border-primary bg-primary/5 font-medium text-foreground"
                  : isDisabled
                    ? "cursor-not-allowed border-border/60 bg-muted/30 text-muted-foreground opacity-60"
                    : "border-border bg-card text-muted-foreground hover:bg-muted/50 hover:text-foreground"
              }`}
            >
              <span
                className={`flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold ${
                  isActive
                    ? "bg-primary text-primary-foreground"
                    : isCompleted
                      ? "bg-emerald-600 text-white dark:bg-emerald-500"
                      : "bg-muted text-muted-foreground"
                }`}
              >
                {isCompleted && !isActive ? (
                  <CheckIcon className="size-3.5" />
                ) : (
                  s
                )}
              </span>
              <span className="truncate">{label}</span>
            </button>
          )
        })}
      </nav>

      {step === 1 && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <SparklesIcon className="size-4 text-muted-foreground" />
              1. Generate extraction prompt
            </CardTitle>
            <CardDescription>
              Local LLMs cannot parse PDF statements directly. Pick the single
              account this PDF belongs to, add any custom rules, and copy the
              generated prompt into Gemini or ChatGPT alongside your PDF.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label htmlFor="import-account">Target account</Label>
                {accounts.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    No accounts found.{" "}
                    <Link
                      href="/accounts"
                      className="underline underline-offset-4"
                    >
                      Create an account
                    </Link>{" "}
                    first.
                  </p>
                ) : (
                  <AccountSelect
                    id="import-account"
                    className="w-full"
                    items={accountSelectItems}
                    value={effectiveAccountId}
                    onValueChange={setSelectedAccountId}
                  />
                )}
              </div>

              <div className="space-y-2">
                <Label htmlFor="import-custom-instructions">
                  Additional instructions (optional)
                </Label>
                <Input
                  id="import-custom-instructions"
                  placeholder='e.g. Ignore any transactions labelled "cash cow"'
                  value={customInstructions}
                  onChange={(e) => setCustomInstructions(e.target.value)}
                />
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between gap-2">
                <Label htmlFor="generated-prompt">
                  Prompt + expected JSON example
                </Label>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={!generatedPrompt}
                  onClick={handleCopyPrompt}
                >
                  {copied ? (
                    <>
                      <CheckIcon className="mr-1.5 size-4" />
                      Copied
                    </>
                  ) : (
                    <>
                      <CopyIcon className="mr-1.5 size-4" />
                      Copy prompt
                    </>
                  )}
                </Button>
              </div>
              <Textarea
                id="generated-prompt"
                readOnly
                value={generatedPrompt}
                rows={12}
                className="font-mono text-xs"
                placeholder="Select an account and configure Settings to generate the prompt."
              />
            </div>
          </CardContent>
          <CardFooter className="flex justify-end border-t pt-4">
            <Button type="button" onClick={() => setStep(2)}>
              Next: Upload or paste JSON
              <ArrowRightIcon className="ml-1.5 size-4" />
            </Button>
          </CardFooter>
        </Card>
      )}

      {step === 2 && (
        <ImportJsonInput
          handleFileUpload={handleFileUpload}
          rawJson={rawJson}
          updateRawJson={updateRawJson}
          isReadingFile={isReadingFile}
          fileError={fileError}
          parsedResult={parsedResult}
          canValidate={canValidate}
          openPreview={openPreview}
          canPreview={canPreview}
          onBack={() => setStep(1)}
        />
      )}

      {step === 3 && importedCount !== null && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <CheckCircle2Icon className="size-5 text-emerald-600 dark:text-emerald-400" />
              Imported {importedCount}{" "}
              {importedCount === 1 ? "transaction" : "transactions"}
            </CardTitle>
            <CardDescription>
              Saved as unreviewed manual transactions
              {importedAccountName ? ` on ${importedAccountName}` : ""}.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap items-center gap-3">
            <Link
              href="/transactions?range=all&source=manual&unreviewed=true"
              className={buttonVariants()}
            >
              Review imported transactions
              <ArrowRightIcon className="ml-1.5 size-4" />
            </Link>
            <Button type="button" variant="outline" onClick={handleStartOver}>
              <RotateCcwIcon className="mr-1.5 size-4" />
              Import another statement
            </Button>
          </CardContent>
        </Card>
      )}

      {step === 3 && importedCount === null && parsedResult && (
        <Card>
          <CardHeader className="flex flex-row flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle className="text-base">
                3. Preview &amp; select rows to import
              </CardTitle>
              <CardDescription>
                {selectedValidRows.length} of {validRows.length} valid{" "}
                {validRows.length === 1 ? "row" : "rows"} selected
                {invalidRowsCount > 0
                  ? ` · ${invalidRowsCount} invalid ${invalidRowsCount === 1 ? "row" : "rows"} excluded`
                  : ""}
                . Unmatched categories default to Other and unmatched tags are
                dropped.
              </CardDescription>
            </div>
            {accounts.length > 0 && (
              <div className="flex items-center gap-2">
                <Label
                  htmlFor="preview-account"
                  className="text-xs whitespace-nowrap text-muted-foreground"
                >
                  Target account
                </Label>
                <AccountSelect
                  id="preview-account"
                  disabled={importMutation.isPending}
                  className="w-48"
                  items={accountSelectItems}
                  value={effectiveAccountId}
                  onValueChange={setSelectedAccountId}
                />
              </div>
            )}
          </CardHeader>
          <CardContent className="space-y-4">
            {importMutation.isError && (
              <Alert variant="destructive">
                <AlertCircleIcon className="size-4" />
                <AlertTitle>Import failed</AlertTitle>
                <AlertDescription>
                  {importMutation.error instanceof Error
                    ? importMutation.error.message
                    : "Failed to import transactions."}
                </AlertDescription>
              </Alert>
            )}

            <ImportPreviewTable
              allValidSelected={allValidSelected}
              importPending={importMutation.isPending}
              validCount={validRows.length}
              toggleAllValid={toggleAllValid}
              rows={parsedResult.rows}
              deselectedIndices={deselectedIndices}
              toggleRowSelection={toggleRowSelection}
            />
          </CardContent>
          <CardFooter className="flex items-center justify-between border-t pt-4">
            <Button
              type="button"
              variant="outline"
              disabled={importMutation.isPending}
              onClick={() => setStep(2)}
            >
              <ArrowLeftIcon className="mr-1.5 size-4" />
              Back
            </Button>
            <Button
              type="button"
              disabled={
                selectedValidRows.length === 0 ||
                !selectedAccount ||
                importMutation.isPending
              }
              onClick={submitImport}
            >
              {importMutation.isPending
                ? "Importing..."
                : `Import ${selectedValidRows.length} ${selectedValidRows.length === 1 ? "transaction" : "transactions"}`}
            </Button>
          </CardFooter>
        </Card>
      )}
    </PageLayout>
  )
}
