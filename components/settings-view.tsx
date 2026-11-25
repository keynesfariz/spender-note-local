"use client"

import { useQuery } from "@tanstack/react-query"
import { useRouter } from "next/navigation"
import { useState } from "react"

import { GmailSendersTab } from "@/components/settings/gmail-senders-tab"
import { LlmSettingsTab } from "@/components/settings/llm-settings-tab"
import { LocaleSettingsTab } from "@/components/settings/locale-settings-tab"
import { TriageRulesTab } from "@/components/settings/triage-rules-tab"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Tabs, TabsList, TabsPanel, TabsTab } from "@/components/ui/tabs"
import { settingsQuery } from "@/lib/api"

type SettingsTabValue = "general" | "gmail" | "triage" | "llm"

function resolveInitialTab(
  tabParam?: string,
  gmailParam?: string,
  errorParam?: string
): SettingsTabValue {
  if (
    tabParam === "general" ||
    tabParam === "gmail" ||
    tabParam === "triage" ||
    tabParam === "llm"
  ) {
    return tabParam
  }
  if (gmailParam || errorParam) {
    return "gmail"
  }
  return "general"
}

export function SettingsView({
  tabParam,
  gmailParam,
  errorParam,
}: {
  tabParam?: string
  gmailParam?: string
  errorParam?: string
}) {
  const router = useRouter()
  const settings = useQuery(settingsQuery())
  const [activeTab, setActiveTab] = useState<SettingsTabValue>(() =>
    resolveInitialTab(tabParam, gmailParam, errorParam)
  )

  function handleTabChange(nextValue: unknown) {
    if (
      nextValue !== "general" &&
      nextValue !== "gmail" &&
      nextValue !== "triage" &&
      nextValue !== "llm"
    ) {
      return
    }
    setActiveTab(nextValue)
    router.replace(`/settings?tab=${nextValue}`, { scroll: false })
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Configure your locale, Gmail connection, trusted senders, triage
          rules, and local LLM server.
        </p>
      </div>

      {gmailParam === "connected" && (
        <Alert>
          <AlertDescription>Gmail connected successfully.</AlertDescription>
        </Alert>
      )}
      {errorParam && (
        <Alert variant="destructive">
          <AlertDescription>
            Gmail connection error: {errorParam}
          </AlertDescription>
        </Alert>
      )}

      <Tabs value={activeTab} onValueChange={handleTabChange}>
        <TabsList variant="line" aria-label="Settings sections">
          <TabsTab value="general">General</TabsTab>
          <TabsTab value="gmail">Gmail &amp; Senders</TabsTab>
          <TabsTab value="triage">Triage &amp; Rules</TabsTab>
          <TabsTab value="llm">Local LLM</TabsTab>
        </TabsList>

        <TabsPanel value="general">
          <LocaleSettingsTab settings={settings.data} />
        </TabsPanel>

        <TabsPanel value="gmail">
          <GmailSendersTab settings={settings.data} />
        </TabsPanel>

        <TabsPanel value="triage">
          <TriageRulesTab settings={settings.data} />
        </TabsPanel>

        <TabsPanel value="llm">
          <LlmSettingsTab settings={settings.data} />
        </TabsPanel>
      </Tabs>
    </div>
  )
}
