import { useQuery } from "@tanstack/react-query"
import { ChevronDownIcon } from "lucide-react"
import { useState } from "react"
import { triageIgnoredEmailBodyQuery } from "@/lib/api/triage"

function IgnoredEmailBody({ emailId }: { emailId: string }) {
  const bodyQuery = useQuery(triageIgnoredEmailBodyQuery(emailId))

  if (bodyQuery.isPending) {
    return (
      <p role="status" className="mt-2 text-xs text-muted-foreground">
        Loading cleaned email text…
      </p>
    )
  }
  if (bodyQuery.isError) {
    return (
      <p role="alert" className="mt-2 text-xs text-destructive">
        {bodyQuery.error.message}
      </p>
    )
  }
  return (
    <pre className="mt-2 max-h-60 overflow-auto rounded-md border bg-muted p-3 font-mono text-xs wrap-break-word whitespace-pre-wrap">
      {bodyQuery.data.cleaned_text || "(Empty email body)"}
    </pre>
  )
}

export function IgnoredEmailBodyPreview({ emailId }: { emailId: string }) {
  const [open, setOpen] = useState(false)
  return (
    <details
      className="group"
      onToggle={(event) => setOpen(event.currentTarget.open)}
    >
      <summary className="flex cursor-pointer list-none items-center gap-1.5 text-xs text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
        <span className="shrink-0 transition-transform group-open:rotate-180">
          <ChevronDownIcon className="size-3.5" />
        </span>
        <span>Inspect cleaned email text</span>
      </summary>
      {open ? <IgnoredEmailBody emailId={emailId} /> : null}
    </details>
  )
}
