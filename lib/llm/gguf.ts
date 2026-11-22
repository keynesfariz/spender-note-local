import "server-only"
import {
  LlmExtractionError,
  LlmRuntimeError,
  buildExtractionJsonSchema,
  buildExtractionPrompt,
  normalizeExtractedResult,
  parseAndValidateExtraction,
  resolveCategories,
  stripThinkBlocks,
  type ExtractTransactionCallOptions,
  type ExtractTransactionInput,
  type ExtractTransactionResult,
  type LlamaChatSessionLike,
  type LlamaContextLike,
  type LlamaContextSequenceLike,
  type LlamaLike,
  type LlamaModelLike,
  type ModelCheckResult,
  type StructuredExtractor,
} from "./types"

const LLAMA_SYMBOL = Symbol.for("spender:llama")
const DEFAULT_CONTEXT_SIZE = 8192
const DEFAULT_TIMEOUT_MS = 60_000

export interface GgufExtractorOptions {
  modelPath: string
  contextSize?: number
  timeoutMs?: number
  categories?: readonly string[]
  llamaLoader?: () => Promise<LlamaLike>
}

async function getDefaultLlama(): Promise<LlamaLike> {
  const g = globalThis as Record<symbol, unknown>
  const existing = g[LLAMA_SYMBOL] as Promise<LlamaLike> | undefined
  if (existing) {
    return existing
  }

  const loaderPromise = (async (): Promise<LlamaLike> => {
    try {
      const mod = await import("node-llama-cpp")
      const llama = await mod.getLlama({ build: "never" })
      return {
        loadModel: (opts) => llama.loadModel(opts),
        createGrammarForJsonSchema: (schema) =>
          llama.createGrammarForJsonSchema(
            schema as Parameters<typeof llama.createGrammarForJsonSchema>[0]
          ),
        createChatSession: (opts) => instantiateChatSession(mod, opts),
      }
    } catch (err) {
      delete g[LLAMA_SYMBOL]
      throw toRuntimeError("Failed to initialize node-llama-cpp runtime", err)
    }
  })()

  g[LLAMA_SYMBOL] = loaderPromise
  return loaderPromise
}

function instantiateChatSession(
  mod: typeof import("node-llama-cpp"),
  opts: {
    contextSequence: LlamaContextSequenceLike
    systemPrompt: string
    autoDisposeSequence?: boolean
  }
): LlamaChatSessionLike {
  return new mod.LlamaChatSession({
    contextSequence: opts.contextSequence as ConstructorParameters<
      typeof mod.LlamaChatSession
    >[0]["contextSequence"],
    systemPrompt: opts.systemPrompt,
    autoDisposeSequence: opts.autoDisposeSequence,
  }) as unknown as LlamaChatSessionLike
}

function toRuntimeError(prefix: string, err: unknown): LlmRuntimeError {
  if (err instanceof LlmRuntimeError) return err
  const msg = err instanceof Error ? err.message : String(err)
  return new LlmRuntimeError(`${prefix}: ${msg}`, { cause: err })
}

export class GgufStructuredExtractor implements StructuredExtractor {
  private readonly modelPath: string
  private readonly contextSize: number
  private readonly timeoutMs: number
  private readonly defaultCategories: readonly string[]
  private readonly llamaLoader: () => Promise<LlamaLike>

  private llama: LlamaLike | null = null
  private model: LlamaModelLike | null = null
  private context: LlamaContextLike | null = null
  private contextLoadPromise: Promise<{
    llama: LlamaLike
    model: LlamaModelLike
    context: LlamaContextLike
  }> | null = null
  private readonly grammarCache = new Map<string, unknown>()
  private readonly activeOperations = new Set<Promise<unknown>>()
  private disposed = false

  constructor(options: GgufExtractorOptions) {
    this.modelPath = options.modelPath
    this.contextSize =
      options.contextSize && options.contextSize > 0
        ? options.contextSize
        : DEFAULT_CONTEXT_SIZE
    this.timeoutMs =
      options.timeoutMs && options.timeoutMs > 0
        ? options.timeoutMs
        : DEFAULT_TIMEOUT_MS
    this.defaultCategories = resolveCategories(options.categories)
    this.llamaLoader = options.llamaLoader ?? getDefaultLlama
  }

  private trackOperation<T>(operation: () => Promise<T>): Promise<T> {
    const promise = operation()
    const tracked = promise.then(
      () => undefined,
      () => undefined
    )
    this.activeOperations.add(tracked)
    void tracked.finally(() => {
      this.activeOperations.delete(tracked)
    })
    return promise
  }

  private async ensureContextLoaded(): Promise<{
    llama: LlamaLike
    model: LlamaModelLike
    context: LlamaContextLike
  }> {
    if (this.disposed) {
      throw new LlmRuntimeError("Extractor has been disposed")
    }

    if (
      this.llama &&
      this.model &&
      !this.model.disposed &&
      this.context &&
      !this.context.disposed
    ) {
      return { llama: this.llama, model: this.model, context: this.context }
    }

    if (this.contextLoadPromise) {
      return this.contextLoadPromise
    }

    const loadPromise = (async () => {
      if (this.disposed) {
        throw new LlmRuntimeError("Extractor has been disposed")
      }

      if (!this.llama) {
        try {
          this.llama = await this.llamaLoader()
        } catch (err) {
          throw toRuntimeError("Failed to initialize llama runtime", err)
        }
      }

      if (!this.model || this.model.disposed) {
        try {
          this.model = await this.llama.loadModel({
            modelPath: this.modelPath,
          })
        } catch (err) {
          throw toRuntimeError(
            `Failed to load GGUF model at ${this.modelPath}`,
            err
          )
        }
      }

      if (!this.context || this.context.disposed) {
        try {
          this.context = await this.model.createContext({
            contextSize: this.contextSize,
          })
        } catch (err) {
          const failedModel = this.model
          this.model = null
          try {
            if (failedModel && !failedModel.disposed) {
              await failedModel.dispose()
            }
          } catch {}
          throw toRuntimeError("Failed to allocate LLM context", err)
        }
      }

      return { llama: this.llama, model: this.model, context: this.context }
    })()

    this.contextLoadPromise = loadPromise
    try {
      return await loadPromise
    } finally {
      if (this.contextLoadPromise === loadPromise) {
        this.contextLoadPromise = null
      }
    }
  }

  async checkModel(): Promise<ModelCheckResult> {
    return this.trackOperation(async () => {
      await this.ensureContextLoaded()
      return {
        ok: true,
        modelPath: this.modelPath,
        contextSize: this.contextSize,
      }
    })
  }

  async extractTransaction(
    input: ExtractTransactionInput,
    signal?: AbortSignal,
    options?: ExtractTransactionCallOptions
  ): Promise<ExtractTransactionResult> {
    return this.trackOperation(async () => {
      if (signal?.aborted) {
        throw new LlmExtractionError("Extraction aborted before starting")
      }

      const effectiveTimeoutMs =
        options?.timeoutMs && options.timeoutMs > 0
          ? options.timeoutMs
          : this.timeoutMs
      const { llama, context } = await this.ensureContextLoaded()
      const categories = resolveCategories(
        input.categories ?? options?.categories ?? this.defaultCategories
      )
      const { systemPrompt, userPrompt } = buildExtractionPrompt(
        input,
        categories
      )

      const accountIds = (input.accounts ?? []).map((a) => a.id)
      const grammarKey = `${categories.join("\0")}\x01${accountIds.join("\0")}`
      let grammar = this.grammarCache.get(grammarKey)
      if (!grammar) {
        const schema = buildExtractionJsonSchema(categories, accountIds)
        try {
          grammar = await llama.createGrammarForJsonSchema(schema)
          this.grammarCache.set(grammarKey, grammar)
        } catch (err) {
          throw toRuntimeError("Failed to compile JSON schema grammar", err)
        }
      }

      let sequence: LlamaContextSequenceLike | null = null
      let session: LlamaChatSessionLike | null = null

      const timeoutController = new AbortController()
      let timedOut = false
      const timeoutId = setTimeout(() => {
        timedOut = true
        timeoutController.abort(new Error("LLM extraction timed out"))
      }, effectiveTimeoutMs)

      const onCallerAbort = () => {
        timeoutController.abort(
          signal?.reason ?? new Error("Extraction aborted")
        )
      }
      if (signal?.aborted) {
        clearTimeout(timeoutId)
        throw new LlmExtractionError("LLM extraction aborted")
      }
      signal?.addEventListener("abort", onCallerAbort, { once: true })

      try {
        try {
          sequence = context.getSequence()
        } catch (err) {
          throw toRuntimeError("Failed to allocate LLM context sequence", err)
        }

        session = llama.createChatSession
          ? llama.createChatSession({
              contextSequence: sequence,
              systemPrompt,
              autoDisposeSequence: false,
            })
          : await this.createDefaultChatSession(sequence, systemPrompt)

        if (signal?.aborted || timeoutController.signal.aborted) {
          throw new LlmExtractionError("LLM extraction aborted")
        }

        let rawResponse: string
        try {
          rawResponse = await session.prompt(userPrompt, {
            grammar,
            temperature: 0,
            signal: timeoutController.signal,
          })
        } catch (err) {
          if (timedOut) {
            throw new LlmExtractionError(
              `LLM extraction timed out after ${effectiveTimeoutMs}ms`,
              { cause: err }
            )
          }
          if (signal?.aborted || timeoutController.signal.aborted) {
            throw new LlmExtractionError("LLM extraction aborted", {
              cause: err,
            })
          }
          throw toRuntimeError("LLM inference failed", err)
        }

        const cleanedJson = stripThinkBlocks(rawResponse)
        const parsed = parseAndValidateExtraction(cleanedJson, categories)
        return normalizeExtractedResult(parsed, cleanedJson, {
          ...input,
          categories,
        })
      } finally {
        clearTimeout(timeoutId)
        signal?.removeEventListener("abort", onCallerAbort)
        if (session) {
          try {
            session.dispose?.()
          } catch {}
        }
        if (sequence) {
          try {
            await sequence.dispose()
          } catch {}
        }
      }
    })
  }

  private async createDefaultChatSession(
    sequence: LlamaContextSequenceLike,
    systemPrompt: string
  ): Promise<LlamaChatSessionLike> {
    const mod = await import("node-llama-cpp")
    return instantiateChatSession(mod, {
      contextSequence: sequence,
      systemPrompt,
      autoDisposeSequence: false,
    })
  }

  async dispose(): Promise<void> {
    this.disposed = true
    while (this.activeOperations.size > 0) {
      await Promise.allSettled(this.activeOperations)
    }
    this.grammarCache.clear()
    const context = this.context
    this.context = null
    if (context && !context.disposed) {
      try {
        await context.dispose()
      } catch {}
    }
    const model = this.model
    this.model = null
    if (model && !model.disposed) {
      try {
        await model.dispose()
      } catch {}
    }
  }
}
