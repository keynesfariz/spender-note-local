import { iconNames, type IconName } from "lucide-react/dynamic"

const catalog = iconNames.toSorted()
const namesByKey = new Map<string, IconName>(
  catalog.map((name) => [name, name])
)

function normalizeIconKey(name: string): string {
  return name
    .trim()
    .replace(/[\s_]+/g, "-")
    .toLowerCase()
}

function legacyIconKey(name: string): string {
  return normalizeIconKey(
    name
      .replace(/([a-z0-9])([A-Z])/g, "$1-$2")
      .replace(/([a-z])([0-9])/g, "$1-$2")
  )
}

export function resolveCategoryIconName(
  name: string | null | undefined
): IconName | undefined {
  if (!name) return undefined
  return (
    namesByKey.get(normalizeIconKey(name)) ??
    namesByKey.get(legacyIconKey(name))
  )
}

export function searchCategoryIcons(query: string): readonly IconName[] {
  const key = normalizeIconKey(query)
  if (!key) return catalog

  const legacyKey = legacyIconKey(query)
  const exactMatch = namesByKey.get(key) ?? namesByKey.get(legacyKey)
  const partialMatches = catalog.filter(
    (name) =>
      name !== exactMatch && (name.includes(key) || name.includes(legacyKey))
  )
  return exactMatch ? [exactMatch, ...partialMatches] : partialMatches
}
