export const DEFAULT_CATEGORIES = [
  "Dining",
  "Groceries",
  "Transport",
  "Shopping",
  "Utilities & Bills",
  "Entertainment",
  "Health",
  "Travel",
  "Subscriptions",
  "Other",
] as const

export type DefaultCategory = (typeof DEFAULT_CATEGORIES)[number]

export const DEFAULT_CATEGORY: DefaultCategory = "Other"
