"use client"

import { useEffect, useEffectEvent, useState } from "react"
import { Input } from "@/components/ui/input"

export function DebouncedSearchInput({
  value,
  onSearch,
  label,
  placeholder,
}: {
  value: string
  onSearch: (value: string) => void
  label: string
  placeholder: string
}) {
  const [previousValue, setPreviousValue] = useState(value)
  const [draft, setDraft] = useState(value)
  if (value !== previousValue) {
    setPreviousValue(value)
    setDraft(value)
  }

  const commit = useEffectEvent(onSearch)
  useEffect(() => {
    if (draft === value || !draft.trim()) return
    const timer = window.setTimeout(() => commit(draft), 250)
    return () => window.clearTimeout(timer)
  }, [draft, value])

  return (
    <Input
      type="search"
      aria-label={label}
      placeholder={placeholder}
      value={draft}
      onChange={(event) => {
        const next = event.target.value
        setDraft(next)
        if (!next.trim() && value) onSearch("")
      }}
    />
  )
}
