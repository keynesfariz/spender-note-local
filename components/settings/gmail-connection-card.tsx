import { type SubmitEvent } from "react"
import { Alert, AlertDescription } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Field, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import type { GmailStatusView } from "@/lib/api/types.generated"

export function GmailConnectionCard({
  gmail,
  gmailError,
  disconnectPending,
  onDisconnect,
  batchLimit,
  setBatchLimitDraft,
  onSaveBatchLimit,
  batchLimitPending,
  syncSettingsMessage,
}: {
  gmail: GmailStatusView | undefined
  gmailError: string | null
  disconnectPending: boolean
  onDisconnect: () => void
  batchLimit: number
  setBatchLimitDraft: (value: number) => void
  onSaveBatchLimit: (event: SubmitEvent<HTMLFormElement>) => void
  batchLimitPending: boolean
  syncSettingsMessage: string | null
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Gmail Connection</CardTitle>
        <CardDescription>
          Read-only access (gmail.readonly) to fetch alerts and receipts from
          configured senders.
        </CardDescription>
        <CardAction className="flex items-center gap-2">
          {gmail?.status === "connected" ? (
            <>
              <Badge variant="secondary">
                Connected: {gmail.email_address}
              </Badge>
              <AlertDialog>
                <AlertDialogTrigger
                  render={
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={disconnectPending}
                    />
                  }
                >
                  Disconnect Gmail
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>
                      Disconnect Gmail account?
                    </AlertDialogTitle>
                    <AlertDialogDescription>
                      Spender will stop syncing alerts and receipts from{" "}
                      {gmail.email_address ?? "your Gmail account"} until you
                      reconnect.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                      variant="destructive"
                      onClick={onDisconnect}
                    >
                      Disconnect Gmail account
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </>
          ) : (
            <a
              href="/api/gmail/auth"
              className={buttonVariants({ size: "sm" })}
            >
              {gmail?.status === "expired"
                ? "Reconnect Gmail"
                : "Connect Gmail"}
            </a>
          )}
        </CardAction>
      </CardHeader>
      <CardContent className="space-y-4">
        {gmail && !gmail.oauthConfigured && (
          <Alert>
            <AlertDescription>
              Set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET in .env before
              connecting Gmail.
            </AlertDescription>
          </Alert>
        )}
        {gmailError && (
          <Alert variant="destructive">
            <AlertDescription>{gmailError}</AlertDescription>
          </Alert>
        )}

        <Separator />

        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={onSaveBatchLimit}
        >
          <Field className="w-44">
            <FieldLabel>Emails per sync batch</FieldLabel>
            <Input
              type="number"
              min={1}
              max={500}
              step={1}
              value={batchLimit}
              onChange={(e) => setBatchLimitDraft(Number(e.target.value))}
            />
          </Field>
          <Button type="submit" size="sm" disabled={batchLimitPending}>
            {batchLimitPending ? "Saving..." : "Save batch limit"}
          </Button>
          {syncSettingsMessage && (
            <p className="text-sm">{syncSettingsMessage}</p>
          )}
        </form>
      </CardContent>
    </Card>
  )
}
