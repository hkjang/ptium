import { useMemo, useState } from 'react'
import { Check, Search, Shapes, Star, X } from 'lucide-react'
import { api } from '../api/client'
import { SlidePreview } from './SlidePreview'
import type { Template } from '../types'

/**
 * Choosing a design out of forty.
 *
 * A gallery that lists everything makes the person do the sorting, and the way
 * out of that is not a shorter library — it is a shorter question. So a few
 * designs are recommended for what is being written, the rest are one click
 * away, and the ones that are shown can be narrowed by what someone actually
 * cares about: light or dark, how it is composed, what it is for.
 */

/** The compositions the library ships, in the words the tags use. */
const structureTags = ['기본', '레일', '중앙', '패널', '편집', '미니멀', '2단', '사이드바', '분할', '밴드', '플레이트', '코너']
const toneTags = ['밝은', '어두운', '세리프', '내 템플릿']

/** The tags a set of templates carries, in the order a filter bar reads best. */
export function templateTags(templates: Template[]) {
  const groups = templateTagGroups(templates)
  return [...groups.tone, ...groups.use, ...groups.structure]
}

/**
 * The same tags, grouped the way a person narrows: first how it looks, then what
 * it is for, then how it is composed. Twenty-five chips in one row is a wall;
 * three short labelled rows is a filter.
 */
export function templateTagGroups(templates: Template[]) {
  const present = new Set<string>()
  for (const template of templates) for (const tag of template.tags || []) present.add(tag)
  const tone = toneTags.filter((tag) => present.has(tag))
  const structure = structureTags.filter((tag) => present.has(tag))
  const use = [...present].filter((tag) => !tone.includes(tag) && !structure.includes(tag))
    .sort((first, second) => first.localeCompare(second, 'ko'))
  return { tone, use, structure }
}

/**
 * The order a library is browsed in.
 *
 * Stored order groups the designs by palette, so the first screenful comes out
 * all one mood — which is the very thing that makes a library look like it holds
 * one design. Taking one composition at a time in turn puts twelve different
 * silhouettes in the first twelve tiles.
 */
export function orderTemplates(templates: Template[]) {
  // Pinned designs and the ones this person keeps building on lead, ahead of
  // even their own uploads: a library that does not learn is a catalogue.
  const personal = templates.filter((template) => template.favorite || (template.usageCount || 0) > 0)
    .sort((first, second) => Number(Boolean(second.favorite)) - Number(Boolean(first.favorite))
      || (second.usageCount || 0) - (first.usageCount || 0))
  const mine = [...personal, ...templates.filter((template) => template.kind === 'uploaded' && !personal.includes(template))]
  const groups = new Map<string, Template[]>()
  for (const template of templates) {
    if (mine.includes(template)) continue
    const structure = (template.tags || []).find((tag) => structureTags.includes(tag)) || 'etc'
    groups.set(structure, [...(groups.get(structure) || []), template])
  }
  const ordered = [...mine]
  const buckets = structureTags.filter((tag) => groups.has(tag)).map((tag) => groups.get(tag)!)
  if (groups.has('etc')) buckets.push(groups.get('etc')!)
  // Alternate which mood each composition leads with, so a row of tiles is not
  // all one colour temperature either.
  buckets.forEach((bucket, index) => bucket.sort((first, second) =>
    (Number(Boolean(first.dark)) - Number(Boolean(second.dark))) * (index % 2 === 0 ? 1 : -1)))
  for (let round = 0; ordered.length < templates.length; round += 1) {
    let placed = false
    for (const bucket of buckets) {
      if (round < bucket.length) {
        ordered.push(bucket[round])
        placed = true
      }
    }
    if (!placed) break
  }
  return ordered
}

/** Templates matching a search and a set of tags. Tags narrow; they never widen. */
export function filterTemplates(templates: Template[], query: string, tags: string[]) {
  const needle = query.trim().toLowerCase()
  return templates.filter((template) => {
    if (tags.length > 0 && !tags.every((tag) => (template.tags || []).includes(tag))) return false
    if (!needle) return true
    return [template.name, template.description, ...(template.tags || [])]
      .filter(Boolean).some((field) => String(field).toLowerCase().includes(needle))
  })
}

/** What the brief suggests, in the words people write briefs in. */
const briefHints: [RegExp, string[]][] = [
  [/투자|피치|ir\b|시리즈|밸류|출시|런칭|비전/i, ['제품 출시', '브랜드·마케팅', '임원 브리핑']],
  [/보고|실적|분기|월간|주간|현황|운영/i, ['사내 보고', '금융·공공', '기술 리뷰']],
  [/리스크|위험|감사|규제|컴플라이언스|사고|장애/i, ['리스크·의사결정', '금융·공공']],
  [/제안|입찰|rfp|계약|고객사/i, ['제안서', '브랜드·마케팅']],
  [/연구|조사|리서치|정책|백서|논문/i, ['리서치·정책', '제안서']],
  [/esg|환경|지속가능|탄소|인프라|보안/i, ['ESG·인프라', '기술 리뷰']],
  [/기술|아키텍처|시스템|개발|플랫폼|데이터/i, ['기술 리뷰', '사내 보고']],
  [/교육|워크숍|온보딩|가이드|학습/i, ['브랜드·마케팅', '제안서']],
]

/**
 * Six designs for this brief, deliberately unalike.
 *
 * The strongest rule here is the last one: no two recommendations share a
 * composition. A row of six recolourings of the same slide is exactly what makes
 * a library feel like it has one design in it.
 */
export function recommendTemplates(templates: Template[], brief: { prompt: string; tone: string; audience: string }, count = 6) {
  const text = `${brief.prompt} ${brief.audience}`
  const wanted = new Set<string>()
  for (const [pattern, uses] of briefHints) {
    if (pattern.test(text)) uses.forEach((use) => wanted.add(use))
  }
  const structural = structureTags
  const score = (template: Template) => {
    const tags = template.tags || []
    let points = 0
    if (template.kind === 'uploaded') points += 100
    // What this person pinned, and what they have actually built decks with.
    // Someone who made nine decks in one design is telling us something no
    // keyword in the brief can.
    if (template.favorite) points += 40
    points += Math.min(24, (template.usageCount || 0) * 8)
    for (const tag of tags) if (wanted.has(tag)) points += 10
    if (brief.tone === 'academic' && tags.includes('세리프')) points += 6
    if ((brief.tone === 'inspiring' || brief.tone === 'persuasive') && template.dark) points += 4
    if (brief.tone === 'friendly' && !template.dark) points += 3
    return points
  }
  const ranked = [...templates].sort((first, second) => score(second) - score(first))
  const chosen: Template[] = []
  const seenStructure = new Set<string>()
  const seenPalette = new Set<string>()
  let dark = 0
  for (const pass of [0, 1]) {
    for (const template of ranked) {
      if (chosen.length >= count || chosen.includes(template)) continue
      const structure = (template.tags || []).find((tag) => structural.includes(tag)) || template.id
      const palette = template.paletteKey?.split('-')[0] || template.id
      // First pass: no composition twice, no palette twice, and never all-dark or
      // all-light. Six recolourings of one slide is the thing to avoid.
      if (pass === 0) {
        if (seenStructure.has(structure) || seenPalette.has(palette)) continue
        if (template.dark && dark >= Math.ceil(count / 2)) continue
        if (!template.dark && chosen.length - dark >= Math.ceil(count / 2)) continue
      }
      seenStructure.add(structure)
      seenPalette.add(palette)
      if (template.dark) dark += 1
      chosen.push(template)
    }
  }
  return chosen.slice(0, count)
}

/** The filter rows: how it looks, what it is for, how it is composed. */
export function TemplateFilterChips({ groups, active, onToggle, onClear, showClear }: {
  groups: { tone: string[]; use: string[]; structure: string[] }
  active: string[]
  onToggle: (tag: string) => void
  onClear: () => void
  showClear: boolean
}) {
  const [structureOpen, setStructureOpen] = useState(false)
  const chip = (tag: string) => (
    <button key={tag} type="button" className={active.includes(tag) ? 'active' : ''} onClick={() => onToggle(tag)}>{tag}</button>
  )
  return <div className="template-chips">
    <div className="template-filter-group"><b>분위기</b>{groups.tone.map(chip)}</div>
    {groups.use.length > 0 && <div className="template-filter-group"><b>용도</b>{groups.use.map(chip)}</div>}
    {groups.structure.length > 0 && <div className="template-filter-group">
      <b>구성</b>
      {(structureOpen ? groups.structure : groups.structure.filter((tag) => active.includes(tag))).map(chip)}
      <button type="button" className="template-filter-more" onClick={() => setStructureOpen((value) => !value)}>
        {structureOpen ? '접기' : `구성 ${groups.structure.length}가지 보기`}
      </button>
    </div>}
    {showClear && <button type="button" className="clear" onClick={onClear}>초기화</button>}
  </div>
}

/** One design in a grid: its cover, its name, what it is for, and whether this
 * person keeps coming back to it. */
export function TemplateTile({ template, selected, onSelect, onFavorite, size = 420 }: {
  template: Template
  selected: boolean
  onSelect: () => void
  /** Pins the design for this person. Omitted where pinning makes no sense. */
  onFavorite?: (favorite: boolean) => void
  size?: number
}) {
  const used = template.usageCount || 0
  return (
    <div className={`template-tile-wrap ${template.favorite ? 'favorite' : ''}`}>
      <button
        type="button"
        className={`template-tile ${selected ? 'selected' : ''}`}
        onClick={onSelect}
        aria-pressed={selected}
        title={template.description || template.name}
      >
        <SlidePreview cacheKey={`tile-${template.id}-${size}`} alt={`${template.name} 표지`}
          load={() => api.templateLayoutPreview(template.id, '', size)} />
        <div>
          <strong>{template.name}</strong>
          <span>{(template.tags || []).slice(0, 3).join(' · ') || `레이아웃 ${template.layoutCount}개`}</span>
        </div>
        {selected && <em><Check size={13} /></em>}
      </button>
      {used > 0 && <span className="template-tile-used" title={`이 디자인으로 만든 덱 ${used}개`}>덱 {used}</span>}
      {onFavorite && <button type="button" className={`template-tile-star ${template.favorite ? 'on' : ''}`}
        onClick={(event) => { event.stopPropagation(); onFavorite(!template.favorite) }}
        aria-pressed={Boolean(template.favorite)}
        title={template.favorite ? '즐겨찾기 해제' : '즐겨찾기에 넣기'}><Star size={13} /></button>}
    </div>
  )
}

/** The whole library, narrowable. Used inside the create flow and on its page. */
export function TemplateBrowser({ templates, selectedId, onSelect, onFavorite, pageSize = 12 }: {
  templates: Template[]
  selectedId: string
  onSelect: (template: Template) => void
  onFavorite?: (template: Template, favorite: boolean) => void
  pageSize?: number
}) {
  const [query, setQuery] = useState('')
  const [tags, setTags] = useState<string[]>([])
  const [mineOnly, setMineOnly] = useState(false)
  const [shown, setShown] = useState(pageSize)
  const available = useMemo(() => templateTagGroups(templates), [templates])
  const personal = useMemo(
    () => templates.filter((template) => template.favorite || (template.usageCount || 0) > 0),
    [templates])
  const matching = useMemo(() => {
    const pool = mineOnly ? personal : templates
    return orderTemplates(filterTemplates(pool, query, tags))
  }, [templates, personal, mineOnly, query, tags])
  const toggle = (tag: string) => {
    setShown(pageSize)
    setTags((current) => current.includes(tag) ? current.filter((item) => item !== tag) : [...current, tag])
  }

  return <div className="template-browser">
    <div className="template-browser-bar">
      <label className="template-search">
        <Search size={15} />
        <input value={query} placeholder="디자인 이름이나 용도로 검색" aria-label="템플릿 검색"
          onChange={(event) => { setQuery(event.target.value); setShown(pageSize) }} />
        {query && <button type="button" onClick={() => setQuery('')} aria-label="검색어 지우기"><X size={13} /></button>}
      </label>
    </div>
    {personal.length > 0 && <div className="template-chips">
      <div className="template-filter-group">
        <b>내 것</b>
        <button type="button" className={mineOnly ? 'active' : ''} onClick={() => { setMineOnly((value) => !value); setShown(pageSize) }}>
          <Star size={11} /> 즐겨찾기·사용한 디자인 {personal.length}
        </button>
      </div>
    </div>}
    <TemplateFilterChips groups={available} active={tags} onToggle={toggle} onClear={() => { setTags([]); setQuery(''); setMineOnly(false) }} showClear={tags.length > 0 || Boolean(query) || mineOnly} />
    <p className="template-browser-count"><Shapes size={13} /> {matching.length}개 디자인{tags.length > 0 || query ? ` · 전체 ${templates.length}개 중` : ''}</p>
    {matching.length === 0
      ? <p className="template-browser-empty">조건에 맞는 디자인이 없습니다. 필터를 지우고 다시 찾아보세요.</p>
      : <>
        <div className="template-tiles">
          {matching.slice(0, shown).map((template) => (
            <TemplateTile key={template.id} template={template} selected={template.id === selectedId}
              onSelect={() => onSelect(template)}
              onFavorite={onFavorite ? (favorite) => onFavorite(template, favorite) : undefined} />
          ))}
        </div>
        {matching.length > shown && (
          <button type="button" className="template-browser-more" onClick={() => setShown((value) => value + pageSize)}>
            {matching.length - shown}개 더 보기
          </button>
        )}
      </>}
  </div>
}
