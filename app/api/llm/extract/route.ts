import { NextResponse } from 'next/server';
import {
  LlmExtractionError,
  LlmRuntimeError,
  createStructuredExtractor,
  type ExtractTransactionInput,
} from '@/lib/llm';

export async function POST(request: Request) {
  let body: ExtractTransactionInput & {
    modelPath: string;
    contextSize?: number;
    timeoutMs?: number;
  };
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: 'Invalid JSON request body', errorKind: 'runtime' },
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
    const result = await extractor.extractTransaction(body, request.signal, {
      timeoutMs: body.timeoutMs,
    });
    return NextResponse.json(result);
  } catch (err) {
    if (err instanceof LlmExtractionError) {
      return NextResponse.json(
        { error: err.message, errorKind: 'extraction' },
        { status: 422 },
      );
    }
    const message =
      err instanceof LlmRuntimeError || err instanceof Error
        ? err.message
        : 'LLM runtime error';
    return NextResponse.json(
      { error: message, errorKind: 'runtime' },
      { status: 500 },
    );
  } finally {
    if (extractor) {
      await extractor.dispose();
    }
  }
}
