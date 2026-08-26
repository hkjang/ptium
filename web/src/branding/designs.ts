/**
 * Which design a stored theme value actually means.
 *
 * This product shipped four themes once — aurora, paper, mint, graphite — and
 * the library it ships today holds fifty designs in ten families. The server
 * still resolves the old four, so a value stored years ago keeps working; the
 * screens kept offering only those four, which is a different thing. An admin
 * whose deployment default is `slate-classic` was reading "Aurora" off the
 * screen, and neither screen could reach the other forty-six.
 *
 * The alias table mirrors the server's. A Go test fails if the two drift.
 */
export const legacyThemeAliases: Record<string, string> = {
  aurora: 'plum-rail',
  modern: 'slate-classic',
  paper: 'ivory-editorial',
  mint: 'forest-centered',
  dark: 'midnight-panel',
}

export type DesignChoice = { key: string; name: string; family: string; id: string; rank: number }

/**
 * The designs this deployment ships, in the library's own order.
 *
 * The order matters rather than merely looking tidy: it is what decides which
 * design a bare family name selects, and which one a value this product does
 * not ship falls back to. A listing sorted by name would answer both questions
 * with a different design than the server does.
 */
export function designChoices(templates: { id?: string; kind?: string; paletteKey?: string; name?: string; designRank?: number }[]): DesignChoice[] {
  const seen = new Set<string>()
  const choices: DesignChoice[] = []
  for (const template of templates) {
    const key = String(template.paletteKey || '')
    if (template.kind !== 'builtin' || !key || seen.has(key)) continue
    seen.add(key)
    choices.push({
      key, name: String(template.name || key), family: key.split('-')[0],
      id: String(template.id || ''), rank: Number(template.designRank) || 0,
    })
  }
  return choices.sort((one, other) => (one.rank || Number.MAX_SAFE_INTEGER) - (other.rank || Number.MAX_SAFE_INTEGER))
}

/**
 * The design a stored value selects, by the same rules the server uses: the key
 * itself, then a name an older version stored, then a bare family name — and
 * the library's first design when it means nothing at all.
 */
export function resolveDesignKey(stored: string, choices: DesignChoice[]): string {
  const wanted = String(stored || '').trim().toLowerCase()
  if (!choices.length) return wanted
  const keys = new Set(choices.map((choice) => choice.key))
  if (keys.has(wanted)) return wanted
  const alias = legacyThemeAliases[wanted]
  if (alias && keys.has(alias)) return alias
  const family = choices.find((choice) => choice.family === wanted)
  if (family) return family.key
  return choices[0].key
}

/** The families, in library order, so a picker can group fifty designs. */
export function designFamilies(choices: DesignChoice[]) {
  const families: { family: string; designs: DesignChoice[] }[] = []
  for (const choice of choices) {
    const found = families.find((entry) => entry.family === choice.family)
    if (found) found.designs.push(choice)
    else families.push({ family: choice.family, designs: [choice] })
  }
  return families
}
