export interface DefaultCategoryDefinition {
  name: string
  icon: string
  isSystem?: boolean
}

export const DEFAULT_CATEGORY_DEFINITIONS: ReadonlyArray<DefaultCategoryDefinition> =
  [
    { name: "Housing", icon: "Home" },
    { name: "Food", icon: "ShoppingCart" },
    { name: "Utilities", icon: "Zap" },
    { name: "Transportation", icon: "Car" },
    { name: "Healthcare", icon: "HeartPulse" },
    { name: "Entertainment", icon: "Ticket" },
    { name: "Dining Out", icon: "Utensils" },
    { name: "Shopping", icon: "ShoppingBag" },
    { name: "Education", icon: "GraduationCap" },
    { name: "Personal Care", icon: "Sparkles" },
    { name: "Other", icon: "Ellipsis", isSystem: true },
  ]

export const DEFAULT_CATEGORIES = [
  "Housing",
  "Food",
  "Utilities",
  "Transportation",
  "Healthcare",
  "Entertainment",
  "Dining Out",
  "Shopping",
  "Education",
  "Personal Care",
  "Other",
] as const

export type DefaultCategory = (typeof DEFAULT_CATEGORIES)[number]

export const DEFAULT_CATEGORY: DefaultCategory = "Other"
