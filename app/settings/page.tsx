import { SettingsView } from '@/components/settings-view';

export default async function SettingsPage({
  searchParams,
}: {
  searchParams: Promise<{ gmail?: string; error?: string }>;
}) {
  const params = await searchParams;
  return <SettingsView gmailParam={params.gmail} errorParam={params.error} />;
}
