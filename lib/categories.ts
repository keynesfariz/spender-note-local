export interface DefaultCategoryDefinition {
  name: string
  icon: string
  isSystem?: boolean
}

export const DEFAULT_CATEGORY_DEFINITIONS: ReadonlyArray<DefaultCategoryDefinition> =
  [
    { name: "Housing", icon: "Home" },
    { name: "Bills & Utilities", icon: "Zap" },
    { name: "Food", icon: "ShoppingCart" },
    { name: "Dining Out", icon: "Utensils" },
    { name: "Entertainment", icon: "Ticket" },
    { name: "Shopping", icon: "ShoppingBag" },
    { name: "Health & Personal Care", icon: "HeartPulse" },
    { name: "Education", icon: "GraduationCap" },
    { name: "Transport", icon: "Car" },
    { name: "Travel", icon: "Plane" },
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
  if (activeCategory && !opts.some((opt) => opt.value === activeCategory)) {
    opts.push({ value: activeCategory, label: activeCategory })
  }
  return opts
}
