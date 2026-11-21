import { NextResponse } from 'next/server';
import { createStructuredExtractor } from '@/lib/llm';

export async function POST(request: Request) {
  let body: { modelPath: string; contextSize?: number; timeoutMs?: number };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: 'Invalid JSON request body' },
      { status: 400 },
    );
  }

  let extractor: ReturnType<typeof createStructuredExtractor> | null = null;
  try {
    extractor = createStructuredExtractor({
      modelPath: body.modelPath,
      contextSize: body.contextSize,
      timeoutMs: body.timeoutMs,
    });
    const result = await extractor.checkModel();
    return NextResponse.json(result);
  } catch (err) {
    return NextResponse.json(
      { error: err instanceof Error ? err.message : 'Model check failed' },
      { status: 500 },
    );
  } finally {
    if (extractor) {
      await extractor.dispose();
    }
  }
}
