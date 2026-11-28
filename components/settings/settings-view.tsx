"use client"

import { useQuery } from "@tanstack/react-query"
import dynamic from "next/dynamic"
import { parseAsString, parseAsStringLiteral, useQueryStates } from "nuqs"

import { PageLayout } from "@/components/PageLayout"
import { LocaleSettingsTab } from "@/components/settings/locale-settings-tab"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { settingsQuery } from "@/lib/api/settings"

function SettingsTabLoading() {
  return <Skeleton className="h-48 w-full" />
}

const GmailSendersTab = dynamic(
  () =>
    import("@/components/settings/gmail-senders-tab").then(
      (mod) => mod.GmailSendersTab
    ),
  { loading: SettingsTabLoading }
)

function preloadGmailSendersTab() {
  void import("@/components/settings/gmail-senders-tab").catch(() => {})
}

const LlmSettingsTab = dynamic(
  () =>
    import("@/components/settings/llm-settings-tab").then(
      (mod) => mod.LlmSettingsTab
    ),
  { loading: SettingsTabLoading }
)

function preloadLlmSettingsTab() {
  void import("@/components/settings/llm-settings-tab").catch(() => {})
}

const TriageRulesTab = dynamic(
  () =>
    import("@/components/settings/triage-rules-tab").then(
      (mod) => mod.TriageRulesTab
    ),
  { loading: SettingsTabLoading }
)

function preloadTriageRulesTab() {
  void import("@/components/settings/triage-rules-tab").catch(() => {})
}

const settingsTabParser = parseAsStringLiteral([
  "general",
  "gmail",
  "triage",
  "llm",
] as const)

const settingsSearchParsers = {
  tab: settingsTabParser,
  gmail: parseAsString,
  error: parseAsString,
}

export function SettingsView() {
  const [{ tab, gmail, error }, setSearch] = useQueryStates(
    settingsSearchParsers
  )
  const settings = useQuery(settingsQuery())
  const activeTab = tab ?? (gmail || error ? "gmail" : "general")

  function handleTabChange(nextValue: unknown) {
    const nextTab =
      typeof nextValue === "string" ? settingsTabParser.parse(nextValue) : null
    if (nextTab) {
      void setSearch({ tab: nextTab, gmail: null, error: null })
    }
  }

  return (
    <PageLayout
      title="Settings"
      subtitle="Configure your locale, Gmail connection, trusted senders, triage
          rules, and local LLM server."
    >
      {gmail === "connected" && (
        <Alert>
          <AlertDescription>Gmail connected successfully.</AlertDescription>
        </Alert>
      )}
      {error && (
        <Alert variant="destructive">
          <AlertDescription>Gmail connection error: {error}</AlertDescription>
        </Alert>
      )}

      <Tabs className="gap-4" value={activeTab} onValueChange={handleTabChange}>
        <TabsList
          variant="line"
          className="max-w-full overflow-x-auto pb-1.5"
          aria-label="Settings sections"
        >
          <TabsTrigger value="general">General</TabsTrigger>
          <TabsTrigger
            value="gmail"
            onPointerEnter={preloadGmailSendersTab}
            onFocus={preloadGmailSendersTab}
          >
            Gmail &amp; Senders
          </TabsTrigger>
          <TabsTrigger
            value="triage"
            onPointerEnter={preloadTriageRulesTab}
            onFocus={preloadTriageRulesTab}
          >
            Triage &amp; Rules
          </TabsTrigger>
          <TabsTrigger
            value="llm"
            onPointerEnter={preloadLlmSettingsTab}
            onFocus={preloadLlmSettingsTab}
          >
            Local LLM
          </TabsTrigger>
        </TabsList>

        <TabsContent value="general">
          <LocaleSettingsTab settings={settings.data} />
        </TabsContent>

        <TabsContent value="gmail">
          {activeTab === "gmail" && (
            <GmailSendersTab settings={settings.data} />
          )}
        </TabsContent>

        <TabsContent value="triage">
          {activeTab === "triage" && (
            <TriageRulesTab settings={settings.data} />
          )}
        </TabsContent>

        <TabsContent value="llm">
          {activeTab === "llm" && <LlmSettingsTab settings={settings.data} />}
        </TabsContent>
      </Tabs>
    </PageLayout>
  )
}
