const MIN_STDOUT_MAX_LISTENERS = 64

export function configureGoRouteStdout() {
  // Next.js pipes each Go route worker's stdout into process.stdout.
  // Raise the limit to avoid MaxListenersExceededWarning when workers start.
  const currentLimit = process.stdout.getMaxListeners()
  if (currentLimit !== 0 && currentLimit < MIN_STDOUT_MAX_LISTENERS) {
    process.stdout.setMaxListeners(MIN_STDOUT_MAX_LISTENERS)
  }
}
