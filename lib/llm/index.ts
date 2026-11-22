import "server-only"
import { GgufStructuredExtractor } from "./gguf"
import { resolveSafeGgufPath } from "./models"
import {
  LlmRuntimeError,
  type ExtractTransactionCallOptions,
  type ExtractTransactionInput,
  type ExtractTransactionResult,
  type ModelCheckResult,
  type StructuredExtractor,
} from "./types"

const WARM_CACHE_SYMBOL = Symbol.for("spender:ggufWarmCache")
const DEFAULT_IDLE_TTL_MS = 5 * 60 * 1000

interface WarmCacheEntry {
  modelPath: string
  contextSize: number
  extractor: GgufStructuredExtractor
  activeLeases: number
  idleTtlMs: number
  idleTimer: ReturnType<typeof setTimeout> | null
  evicted: boolean
  poisoned: boolean
  disposePromise: Promise<void> | null
}

interface WarmCacheState {
  current: WarmCacheEntry | null
  pendingDisposal: Promise<void>
}

function getWarmCacheState(): WarmCacheState {
  const g = globalThis as Record<symbol, unknown>
  let state = g[WARM_CACHE_SYMBOL] as WarmCacheState | undefined
  if (!state) {
    state = { current: null, pendingDisposal: Promise.resolve() }
    g[WARM_CACHE_SYMBOL] = state
  }
  return state
}

function clearEntryIdleTimer(entry: WarmCacheEntry): void {
  if (entry.idleTimer !== null) {
    clearTimeout(entry.idleTimer)
    entry.idleTimer = null
  }
}

function disposeEntry(
  state: WarmCacheState,
  entry: WarmCacheEntry
): Promise<void> {
  clearEntryIdleTimer(entry)
  entry.evicted = true
  if (state.current === entry) {
    state.current = null
  }
  if (!entry.disposePromise) {
    const prevDisposal = state.pendingDisposal
    entry.disposePromise = prevDisposal
      .catch(() => undefined)
      .then(() => entry.extractor.dispose())
      .catch(() => undefined)
    state.pendingDisposal = entry.disposePromise
  }
  return entry.disposePromise
}

function scheduleEntryIdleTimer(
  state: WarmCacheState,
  entry: WarmCacheEntry
): void {
  clearEntryIdleTimer(entry)
  if (entry.evicted || entry.poisoned || entry.activeLeases > 0) return
  const timer = setTimeout(() => {
    entry.idleTimer = null
    if (entry.activeLeases === 0) {
      void disposeEntry(state, entry)
    }
  }, entry.idleTtlMs)
  timer.unref?.()
  entry.idleTimer = timer
}

export async function evictWarmExtractorIfMismatch(params: {
  modelPath: string | null
  contextSize: number
}): Promise<void> {
  const state = getWarmCacheState()
  const current = state.current
  if (!current) {
    await state.pendingDisposal
    return
  }
  if (
    params.modelPath !== null &&
    current.modelPath === params.modelPath &&
    current.contextSize === params.contextSize &&
    !current.poisoned &&
    !current.evicted
  ) {
    return
  }
  clearEntryIdleTimer(current)
  current.evicted = true
  if (state.current === current) {
    state.current = null
  }
  if (current.activeLeases === 0) {
    await disposeEntry(state, current)
  }
}

export function createStructuredExtractor(options: {
  modelsDir?: string
  modelPath: string
  contextSize?: number
  timeoutMs?: number
  categories?: readonly string[]
}): StructuredExtractor {
  const modelsDir =
    options.modelsDir ?? (process.env.LLM_MODELS_DIR?.trim() || "models")
  const relativeModelPath = options.modelPath?.trim()
  if (!relativeModelPath) {
    throw new LlmRuntimeError("No GGUF model path configured")
  }

  const contextSize =
    options.contextSize && options.contextSize > 0 ? options.contextSize : 8192
  const timeoutMs =
    options.timeoutMs && options.timeoutMs > 0 ? options.timeoutMs : 60_000
  const safePath = resolveSafeGgufPath(modelsDir, relativeModelPath)
  const state = getWarmCacheState()

  let entry = state.current
  const isMatch =
    entry !== null &&
    !entry.evicted &&
    !entry.poisoned &&
    entry.modelPath === safePath &&
    entry.contextSize === contextSize

  if (entry && isMatch) {
    clearEntryIdleTimer(entry)
    entry.activeLeases += 1
  } else {
    if (entry) {
      clearEntryIdleTimer(entry)
      entry.evicted = true
      state.current = null
      if (entry.activeLeases === 0) {
        void disposeEntry(state, entry)
      }
    }

    const extractor = new GgufStructuredExtractor({
      modelPath: safePath,
      contextSize,
      timeoutMs,
      categories: options.categories,
    })

    entry = {
      modelPath: safePath,
      contextSize,
      extractor,
      activeLeases: 1,
      idleTtlMs: DEFAULT_IDLE_TTL_MS,
      idleTimer: null,
      evicted: false,
      poisoned: false,
      disposePromise: null,
    }
    state.current = entry
  }

  const leasedEntry = entry
  let leaseReleased = false

  return {
    async checkModel(): Promise<ModelCheckResult> {
      await state.pendingDisposal
      try {
        return await leasedEntry.extractor.checkModel()
      } catch (err) {
        if (err instanceof LlmRuntimeError) leasedEntry.poisoned = true
        throw err
      }
    },

    async extractTransaction(
      input: ExtractTransactionInput,
      signal?: AbortSignal,
      callOptions?: ExtractTransactionCallOptions
    ): Promise<ExtractTransactionResult> {
      await state.pendingDisposal
      try {
        return await leasedEntry.extractor.extractTransaction(input, signal, {
          timeoutMs: callOptions?.timeoutMs ?? timeoutMs,
          categories: callOptions?.categories ?? options.categories,
        })
      } catch (err) {
        if (err instanceof LlmRuntimeError) leasedEntry.poisoned = true
        throw err
      }
    },

    async dispose(): Promise<void> {
      if (leaseReleased) {
        if (leasedEntry.disposePromise) await leasedEntry.disposePromise
        return
      }
      leaseReleased = true
      leasedEntry.activeLeases = Math.max(0, leasedEntry.activeLeases - 1)
      if (leasedEntry.activeLeases > 0) return
      if (leasedEntry.poisoned || leasedEntry.evicted) {
        await disposeEntry(state, leasedEntry)
        return
      }
      scheduleEntryIdleTimer(state, leasedEntry)
    },
  }
}

export * from "./types"
export * from "./models"
export * from "./gguf"
