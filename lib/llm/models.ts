import "server-only"
import { accessSync, constants, realpathSync, statSync } from "node:fs"
import { isAbsolute, relative, resolve } from "node:path"
import { LlmRuntimeError } from "./types"

function isStrictlyInside(
  parentRealPath: string,
  childRealPath: string
): boolean {
  const rel = relative(parentRealPath, childRealPath)
  return rel !== "" && !rel.startsWith("..") && !isAbsolute(rel)
}

export function resolveSafeGgufPath(
  modelsDir: string,
  relativeModelPath: string
): string {
  if (!relativeModelPath || !relativeModelPath.trim()) {
    throw new LlmRuntimeError("Model path cannot be empty")
  }
  if (relativeModelPath.includes("\0")) {
    throw new LlmRuntimeError("Model path contains invalid null byte")
  }
  if (isAbsolute(relativeModelPath)) {
    throw new LlmRuntimeError(
      "Model path must be relative to the models directory"
    )
  }

  let realRoot: string
  try {
    realRoot = realpathSync(modelsDir)
  } catch (err) {
    throw new LlmRuntimeError(`Models directory does not exist: ${modelsDir}`, {
      cause: err,
    })
  }

  const candidate = resolve(realRoot, relativeModelPath)
  if (!isStrictlyInside(realRoot, candidate)) {
    throw new LlmRuntimeError(
      `Model path escapes models directory: ${relativeModelPath}`
    )
  }

  let realTarget: string
  try {
    realTarget = realpathSync(candidate)
  } catch (err) {
    throw new LlmRuntimeError(`Model file not found: ${relativeModelPath}`, {
      cause: err,
    })
  }

  if (!isStrictlyInside(realRoot, realTarget)) {
    throw new LlmRuntimeError(
      `Model symlink target escapes models directory: ${relativeModelPath}`
    )
  }

  let stats
  try {
    stats = statSync(realTarget)
  } catch (err) {
    throw new LlmRuntimeError(
      `Unable to stat model file: ${relativeModelPath}`,
      { cause: err }
    )
  }

  if (!stats.isFile()) {
    throw new LlmRuntimeError(
      `Model path is not a regular file: ${relativeModelPath}`
    )
  }
  if (!realTarget.toLowerCase().endsWith(".gguf")) {
    throw new LlmRuntimeError(
      `Model file must have .gguf extension: ${relativeModelPath}`
    )
  }

  try {
    accessSync(realTarget, constants.R_OK)
  } catch (err) {
    throw new LlmRuntimeError(
      `Model file is not readable: ${relativeModelPath}`,
      { cause: err }
    )
  }

  return realTarget
}
