import {
  AlertCircleIcon,
  ArrowLeftIcon,
  ArrowRightIcon,
  FileUpIcon,
} from "lucide-react"
import { type ChangeEvent } from "react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
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
import { type ParseManualImportResult } from "@/lib/manual-import"

export function ImportJsonInput({
  handleFileUpload,
  rawJson,
  updateRawJson,
  isReadingFile,
  fileError,
  parsedResult,
  canValidate,
  openPreview,
  canPreview,
  onBack,
}: {
  handleFileUpload: (event: ChangeEvent<HTMLInputElement>) => void
  rawJson: string
  updateRawJson: (nextJson: string) => void
  isReadingFile: boolean
  fileError: string
  parsedResult: ParseManualImportResult | null | undefined
  canValidate: boolean
  openPreview: () => void
  canPreview: boolean
  onBack: () => void
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <FileUpIcon className="size-4 text-muted-foreground" />
          2. Upload or paste JSON result
        </CardTitle>
        <CardDescription>
          Upload the <code>.json</code> file (auto-advances to preview) or paste
          the JSON block returned by your chat model.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="json-file-upload">Upload JSON file</Label>
          <Input
            id="json-file-upload"
            type="file"
            accept=".json,application/json"
            onChange={handleFileUpload}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="json-paste-input">Or paste JSON directly</Label>
          <Textarea
            id="json-paste-input"
            rows={10}
            className="font-mono text-xs"
            placeholder='{"transactions": [...]}'
            value={rawJson}
            onChange={(e) => updateRawJson(e.target.value)}
          />
        </div>

        {isReadingFile && <p role="status">Reading JSON file…</p>}

        {fileError && (
          <Alert variant="destructive">
            <AlertCircleIcon className="size-4" />
            <AlertDescription>{fileError}</AlertDescription>
          </Alert>
        )}

        {parsedResult?.error && (
          <Alert variant="destructive">
            <AlertCircleIcon className="size-4" />
            <AlertTitle>Could not parse JSON</AlertTitle>
            <AlertDescription>{parsedResult.error}</AlertDescription>
          </Alert>
        )}
      </CardContent>
      <CardFooter className="flex items-center justify-between border-t pt-4">
        <Button type="button" variant="outline" onClick={onBack}>
          <ArrowLeftIcon className="mr-1.5 size-4" />
          Back
        </Button>
        <Button type="button" disabled={!canValidate} onClick={openPreview}>
          Next: Preview{" "}
          {canPreview && parsedResult ? `(${parsedResult.rows.length})` : ""}
          <ArrowRightIcon className="ml-1.5 size-4" />
        </Button>
      </CardFooter>
    </Card>
  )
}
