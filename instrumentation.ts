const MIN_STDOUT_MAX_LISTENERS = 64

export function register() {
  // Next.js experimental.goRoutes pipes each long-lived Go route worker's
  // stdout into process.stdout (`ready.pipe(process.stdout, { end: false })`),
  // triggering MaxListenersExceededWarning once >10 Go routes are active.
  if (process.env.NEXT_RUNTIME !== "nodejs") {
    return
  }

  const currentLimit = process.stdout.getMaxListeners()
  if (currentLimit !== 0 && currentLimit < MIN_STDOUT_MAX_LISTENERS) {
    process.stdout.setMaxListeners(MIN_STDOUT_MAX_LISTENERS)
  }
}
