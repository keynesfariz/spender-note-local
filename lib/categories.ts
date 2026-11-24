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

export const DEFAULT_CATEGORY = "Other"

export function buildCategoryIconMap(
  categories?: ReadonlyArray<{ name: string; icon: string }>
): Map<string, string> {
  const map = new Map<string, string>(
    DEFAULT_CATEGORY_DEFINITIONS.map((c) => [c.name, c.icon])
  )
  for (const cat of categories ?? []) {
    map.set(cat.name, cat.icon)
  }
  return map
}

export function buildCategorySelectOptions(
  categories?: ReadonlyArray<{ name: string }>,
  activeCategory?: string
): Array<{ value: string; label: string }> {
  const opts = (categories ?? DEFAULT_CATEGORY_DEFINITIONS).map((c) => ({
    value: c.name,
    label: c.name,
  }))
  if (
    activeCategory &&
    activeCategory !== "all" &&
    !opts.some((opt) => opt.value === activeCategory)
  ) {
    opts.push({ value: activeCategory, label: activeCategory })
  }
  return opts
}
