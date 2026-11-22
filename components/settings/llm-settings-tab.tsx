"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useMemo, useState } from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import {
  dismissPendingQueueApi,
  probeLlmModelsApi,
  retryFailedExtractionsApi,
  syncStatusQuery,
  testLlmModelApi,
  updateLlmSettingsApi,
  type AppSettingsView,
} from "@/lib/api"

const DEFAULT_LLM_BASE_URL = "http://localhost:9931/v1"

export function LlmSettingsTab({
  settings,
}: {
  settings: AppSettingsView | undefined
}) {
  const queryClient = useQueryClient()
  const syncStatus = useQuery(syncStatusQuery())

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
  const [modelTestResult, setModelTestResult] = useState<string | null>(null)
  const [queueMessage, setQueueMessage] = useState<string | null>(null)

  const baseUrl =
    baseUrlDraft ?? settings?.llm_base_url ?? DEFAULT_LLM_BASE_URL
  const llmModel = modelDraft ?? settings?.llm_model ?? ""
  const timeoutMs = timeoutMsDraft ?? settings?.llm_timeout_ms ?? 60000

  const activeProbe =
    probedState && probedState.baseUrl === baseUrl.trim() ? probedState : null
  const availableModels =
    activeProbe?.availableModels ?? settings?.availableModels ?? []
  const serverReachable =
    activeProbe?.serverReachable ?? settings?.serverReachable ?? false
  const serverError = activeProbe?.serverError ?? settings?.serverError ?? null

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

  return (
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
            {probeModelsMutation.isPending ? "Refreshing..." : "Refresh models"}
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
            Extraction queue: {syncStatus.data?.pendingEmailCount ?? 0} pending,{" "}
            {syncStatus.data?.failedEmailCount ?? 0} failed
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

        {queueMessage && (
          <Alert variant="destructive">
            <AlertDescription>{queueMessage}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </Card>
  )
}
