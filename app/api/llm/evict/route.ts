import { NextResponse } from "next/server"
import { evictWarmExtractorIfMismatch } from "@/lib/llm"

export async function POST(request: Request) {
  try {
    const body = (await request.json()) as {
      modelPath: string | null
      contextSize: number
    }
    await evictWarmExtractorIfMismatch({
      modelPath: body.modelPath ?? null,
      contextSize: body.contextSize ?? 8192,
    })
    return NextResponse.json({ ok: true })
  } catch {
    return NextResponse.json({ ok: false }, { status: 400 })
  }
}
