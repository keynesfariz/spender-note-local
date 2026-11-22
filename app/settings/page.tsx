import { SettingsView } from "@/components/settings-view"

export default async function SettingsPage({
  searchParams,
}: {
  searchParams: Promise<{ tab?: string; gmail?: string; error?: string }>
}) {
  const params = await searchParams
  return (
    <SettingsView
      tabParam={params.tab}
      gmailParam={params.gmail}
      errorParam={params.error}
    />
  )
}
