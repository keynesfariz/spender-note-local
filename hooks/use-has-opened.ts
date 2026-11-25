import { useState } from "react"

// Keep a lazy overlay mounted after its first open so it can animate closed
// and return focus to its trigger.
export function useHasOpened(open: boolean) {
  const [hasOpened, setHasOpened] = useState(open)
  if (open && !hasOpened) {
    setHasOpened(true)
  }
  return open || hasOpened
}
