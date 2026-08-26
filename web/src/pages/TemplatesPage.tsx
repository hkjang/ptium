import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Check, Download, Globe, LayoutTemplate, Lock, Plus, Search, Shapes, Star, Trash2, Upload, X,
} from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { TemplateFilterChips, filterTemplates, orderTemplates, templateTagGroups } from '../components/TemplateChooser'
import { SlidePreview } from '../components/SlidePreview'
import { warningText } from './editor/model/findings'
import { Badge, Button, EmptyState, ErrorState, Field, Input, LoadingState, Modal, Textarea } from '../components/UI'
import { useToast } from '../components/Toast'
import { navigate } from '../router'
import type { Template, TemplateLayout } from '../types'
import { displayError, relativeDate } from '../utils'

const roleLabels: Record<string, string> = {
  title: '표지', section: '구역', content: '본문', twoContent: '2단',
  comparison: '비교', quote: '인용', picture: '이미지', table: '표',
  chart: '차트', closing: '마무리', blank: '빈 화면',
}

// rejectionText translates the server's reason, which is written in English for
// the API, into the workspace's language.
function rejectionText(reason: string) {
  if (reason.includes('background')) return '배경색과 구분되지 않아 데이터 색으로 쓰지 않습니다.'
  if (reason.includes('grey')) return '회색에 가까워 강조 해제 색과 겹칩니다.'
  if (reason.includes('deuteranopia')) return '적록색약에서 앞 색과 구분되지 않습니다.'
  if (reason.includes('protanopia')) return '적색약에서 앞 색과 구분되지 않습니다.'
  if (reason.includes('indistinguishable')) return '앞 색과 구분되지 않습니다.'
  return '데이터 색으로 쓸 수 없습니다.'
}

export function roleLabel(role: string) {
  return roleLabels[role] || role
}

function formatBytes(value: number) {
  if (value <= 0) return '—'
  if (value < 1024 * 1024) return `${Math.round(value / 1024)} KB`
  return `${(value / (1024 * 1024)).toFixed(1)} MB`
}

export function TemplatesPage() {
  const [templates, setTemplates] = useState<Template[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [detail, setDetail] = useState<Template | null>(null)
  const [uploadOpen, setUploadOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [activeTags, setActiveTags] = useState<string[]>([])
  const [shown, setShown] = useState(12)
  const { showToast } = useToast()

  const load = useCallback(async () => {
    setLoading(true); setError('')
    try {
      setTemplates(await api.templates())
    } catch (err) { setError(displayError(err)) } finally { setLoading(false) }
  }, [])
  useEffect(() => { void load() }, [load])

  const openDetail = async (template: Template) => {
    try {
      setDetail(await api.template(template.id))
    } catch (err) { showToast(displayError(err), 'error') }
  }

  const remove = async (template: Template) => {
    if (!window.confirm(`템플릿 "${template.name}"을 삭제할까요? 이 템플릿으로 만든 프레젠테이션은 기본 디자인으로 내보내집니다.`)) return
    try {
      await api.deleteTemplate(template.id)
      showToast('템플릿을 삭제했습니다.')
      setDetail(null)
      await load()
    } catch (err) { showToast(displayError(err), 'error') }
  }

  const toggleScope = async (template: Template) => {
    try {
      const updated = await api.updateTemplate(template.id, { scope: template.scope === 'shared' ? 'private' : 'shared' })
      showToast(updated.scope === 'shared' ? '조직 전체에 공유했습니다.' : '나만 사용하도록 변경했습니다.')
      setTemplates((current) => current.map((item) => item.id === updated.id ? { ...item, scope: updated.scope } : item))
      setDetail((current) => current && current.id === updated.id ? { ...current, scope: updated.scope } : current)
    } catch (err) { showToast(displayError(err), 'error') }
  }

  const favorite = (template: Template, on: boolean) => {
    setTemplates((current) => current.map((item) => item.id === template.id ? { ...item, favorite: on } : item))
    void api.favoriteTemplate(template.id, on).catch((err) => {
      setTemplates((current) => current.map((item) => item.id === template.id ? { ...item, favorite: !on } : item))
      showToast(displayError(err), 'error')
    })
  }

  const mine = useMemo(() => templates.filter((template) => template.kind === 'uploaded'), [templates])
  // The shelf someone builds for themselves: pinned designs and the ones they
  // keep making decks with, whoever wrote them.
  const familiar = useMemo(() => templates
    .filter((template) => template.favorite || (template.usageCount || 0) > 0)
    .sort((first, second) => Number(Boolean(second.favorite)) - Number(Boolean(first.favorite))
      || (second.usageCount || 0) - (first.usageCount || 0)), [templates])
  const builtin = useMemo(() => templates.filter((template) => template.kind === 'builtin'), [templates])
  const availableTags = useMemo(() => templateTagGroups(builtin), [builtin])
  const matching = useMemo(() => orderTemplates(filterTemplates(builtin, query, activeTags)), [builtin, query, activeTags])

  return (
    <AppShell
      title="템플릿"
      eyebrow="DESIGN LIBRARY"
      actions={<Button onClick={() => setUploadOpen(true)}><Upload size={16} /> 템플릿 업로드</Button>}
    >
      <p className="page-lead">
        보유한 PowerPoint 템플릿(.pptx / .potx)을 업로드하면 마스터·레이아웃·테마 색·글꼴·로고를 그대로 유지한 채
        AI가 각 레이아웃의 자리 표시자에 맞춰 내용을 채웁니다.
      </p>

      {loading ? <LoadingState label="템플릿을 불러오는 중…" /> : error ? <ErrorState message={error} onRetry={() => void load()} /> : <>
        {familiar.length > 0 && <section className="template-section">
          <div className="section-head"><h2>자주 쓰는 디자인</h2><span>{familiar.length}개</span></div>
          <p className="section-note">별표를 눌러 둔 디자인과, 실제로 덱을 만든 디자인입니다. 새로 만들기에서도 먼저 보여 줍니다.</p>
          <div className="template-grid">{familiar.slice(0, 6).map((template) => (
            <TemplateCard key={`familiar-${template.id}`} template={template} onOpen={() => void openDetail(template)}
              onFavorite={(on) => favorite(template, on)} />
          ))}</div>
        </section>}

        <section className="template-section">
          <div className="section-head"><h2>내 템플릿</h2><span>{mine.length}개</span></div>
          {mine.length === 0
            ? <EmptyState
                icon={<LayoutTemplate size={25} />}
                title="업로드한 템플릿이 없습니다"
                description="회사 표준 템플릿을 올리면 그 디자인 그대로 발표 자료가 생성됩니다."
                action={<Button onClick={() => setUploadOpen(true)}><Plus size={15} /> 첫 템플릿 업로드</Button>}
              />
            : <div className="template-grid">{mine.map((template) => (
                <TemplateCard key={template.id} template={template} onOpen={() => void openDetail(template)}
                  onFavorite={(on) => favorite(template, on)}
                  onToggleScope={() => void toggleScope(template)} onDelete={() => void remove(template)} />
              ))}</div>}
        </section>

        <section className="template-section">
          <div className="section-head"><h2>기본 제공 디자인</h2><span>{builtin.length}개</span></div>
          {/* Forty covers is a library, not a page. It is narrowed by what people
              actually choose on — light or dark, how it is composed, what it is
              for — and drawn a screenful at a time. */}
          <div className="template-browser-bar">
            <label className="template-search">
              <Search size={15} />
              <input value={query} placeholder="디자인 이름이나 용도로 검색" aria-label="디자인 검색"
                onChange={(event) => { setQuery(event.target.value); setShown(12) }} />
              {query && <button type="button" onClick={() => setQuery('')} aria-label="검색어 지우기"><X size={13} /></button>}
            </label>
          </div>
          <TemplateFilterChips
            groups={availableTags}
            active={activeTags}
            showClear={activeTags.length > 0 || Boolean(query)}
            onClear={() => { setActiveTags([]); setQuery('') }}
            onToggle={(tag) => { setShown(12); setActiveTags((current) => current.includes(tag) ? current.filter((item) => item !== tag) : [...current, tag]) }}
          />
          {matching.length === 0
            ? <p className="template-browser-empty">조건에 맞는 디자인이 없습니다. 필터를 지우고 다시 찾아보세요.</p>
            : <>
              <div className="template-grid">{matching.slice(0, shown).map((template) => (
                <TemplateCard key={template.id} template={template} onOpen={() => void openDetail(template)}
                  onFavorite={(on) => favorite(template, on)} />
              ))}</div>
              {matching.length > shown && <button type="button" className="template-browser-more" onClick={() => setShown((value) => value + 12)}>
                {matching.length - shown}개 더 보기
              </button>}
            </>}
        </section>
      </>}

      <UploadTemplateModal open={uploadOpen} onClose={() => setUploadOpen(false)} onUploaded={async (template) => {
        setUploadOpen(false)
        showToast(`"${template.name}" 템플릿을 등록했습니다. 레이아웃 ${template.layoutCount}개를 인식했어요.`)
        await load()
        void openDetail(template)
      }} />

      <TemplateDetailModal template={detail} onClose={() => setDetail(null)} />
    </AppShell>
  )
}

function TemplateCard({ template, onOpen, onFavorite, onToggleScope, onDelete }: {
  template: Template
  onOpen: () => void
  onFavorite?: (favorite: boolean) => void
  onToggleScope?: () => void
  onDelete?: () => void
}) {
  return (
    <article className={`template-card ${template.favorite ? 'favorite' : ''}`}>
      <button className="template-card-preview" onClick={onOpen} aria-label={`${template.name} 레이아웃 보기`}>
        <SlidePreview cacheKey={`${template.id}-card`} alt={`${template.name} 표지 레이아웃`} load={() => api.templateLayoutPreview(template.id, '', 520)} />
      </button>
      <div className="template-card-body">
        <div className="template-card-title">
          <strong>{template.name}</strong>
          {template.kind === 'builtin' ? <Badge tone="info">기본</Badge> : template.scope === 'shared' ? <Badge tone="success">공유</Badge> : <Badge>개인</Badge>}
          {onFavorite && <button type="button" className={`template-card-star ${template.favorite ? 'on' : ''}`}
            onClick={() => onFavorite(!template.favorite)} aria-pressed={Boolean(template.favorite)}
            title={template.favorite ? '즐겨찾기 해제' : '즐겨찾기에 넣기'}><Star size={14} /></button>}
        </div>
        {(template.tags || []).length > 0 && <div className="template-card-tags">
          {(template.tags || []).map((tag) => <span key={tag}>{tag}</span>)}
        </div>}
        {template.description && <p>{template.description}</p>}
        <ul className="template-meta">
          <li><Shapes size={13} /> 레이아웃 {template.layoutCount}개</li>
          {template.aspectRatio && <li>{template.aspectRatio}</li>}
          <li>{formatBytes(template.sizeBytes)}</li>
          {(template.usageCount ?? 0) > 0 && <li>내 덱 {template.usageCount}개</li>}
          {template.lastUsed && <li>{relativeDate(template.lastUsed)} 사용</li>}
        </ul>
      </div>
      <div className="template-card-actions">
        <Button variant="secondary" size="small" onClick={onOpen}>레이아웃 보기</Button>
        <Button variant="ghost" size="small" onClick={() => navigate(`/create?template=${encodeURIComponent(template.id)}`)}>이 템플릿으로 만들기</Button>
        {onToggleScope && <button className="icon-button small" title={template.scope === 'shared' ? '공유 해제' : '조직에 공유'} onClick={onToggleScope}>{template.scope === 'shared' ? <Globe size={15} /> : <Lock size={15} />}</button>}
        {onDelete && <button className="icon-button small danger-hover" title="삭제" onClick={onDelete}><Trash2 size={15} /></button>}
      </div>
    </article>
  )
}

// The four things a brief most often asks to be drawn. A template that cannot
// hold them turns each one into a paragraph, and until this said so, the way to
// find out was to generate forty decks.
const componentOrder = ['steps', 'kpi', 'shareBar', 'table', 'image']
const componentNames: Record<string, string> = {
  steps: '단계', kpi: '지표', shareBar: '비중', table: '표', image: '그림',
}

// The reader of this panel never wrote the probe deck, so "line 37 (slide 7):"
// points at nothing they can look at. What is left is the sentence about their
// own design, which is the whole reason the panel is here.
function aboutTheTemplate(warning: string) {
  return warningText(warning).replace(/^\s*line\s+\d+\s*(\(slide\s*\d+\))?\s*:\s*/i, '')
}

function healthWord(report: Record<string, unknown> | null) {
  if (!report) return { tone: 'neutral' as const, text: '점검하는 중…' }
  const drawn = Object.values((report.components || {}) as Record<string, boolean>)
  const missing = drawn.filter((can) => !can).length
  const defects = Number(report.defects ?? 0)
  if (defects > 0) return { tone: 'danger' as const, text: `이 템플릿에서 ${defects}곳이 잘못 그려집니다` }
  if (missing === drawn.length) return { tone: 'danger' as const, text: '컴포넌트가 하나도 그려지지 않습니다' }
  if (missing > 0) return { tone: 'warning' as const, text: `${missing}종류가 이 디자인에는 들어가지 못합니다` }
  return { tone: 'success' as const, text: '덱이 이 템플릿에 그대로 들어갑니다' }
}

function TemplateHealthPanel({ template }: { template: Template }) {
  const [report, setReport] = useState<Record<string, unknown> | null>(null)
  const [failed, setFailed] = useState('')
  useEffect(() => {
    setReport(null); setFailed('')
    api.templateHealth(template.id).then(setReport).catch((err) => setFailed(displayError(err)))
  }, [template.id])
  if (failed) return <div className="template-health"><p className="muted-note">{failed}</p></div>
  const verdict = healthWord(report)
  const components = (report?.components || {}) as Record<string, boolean>
  const warnings = (report?.warnings || []) as string[]
  return <div className="template-health">
    <div className="template-health-head">
      <strong>이 템플릿에 덱을 넣으면</strong>
      <Badge tone={verdict.tone}>{verdict.text}</Badge>
    </div>
    {report && <>
      <div className="template-health-components">
        {componentOrder.filter((kind) => kind in components).map((kind) => [kind, components[kind]] as const)
          .map(([kind, drawn]) => <span key={kind} className={drawn ? 'drawn' : 'as-text'}>
          {componentNames[kind] || kind}{drawn ? '' : ' · 글로'}</span>)}
      </div>
      {warnings.length > 0 && <ul className="template-health-notes">
        {warnings.slice(0, 4).map((warning) => <li key={warning}>{aboutTheTemplate(warning)}</li>)}
        {warnings.length > 4 && <li className="muted-note">외 {warnings.length - 4}건</li>}
      </ul>}
    </>}
  </div>
}

function TemplateDetailModal({ template, onClose }: { template: Template | null; onClose: () => void }) {
  const [selected, setSelected] = useState<TemplateLayout | null>(null)
  useEffect(() => { setSelected(template?.layouts?.[0] || null) }, [template])
  if (!template) return null
  const layouts = template.layouts || []
  return (
    <Modal
      open
      onClose={onClose}
      title={template.name}
      description={`${layouts.length}개의 레이아웃을 인식했습니다. AI는 각 슬라이드의 목적에 맞는 레이아웃을 골라 자리 표시자 용량 안에서 내용을 작성합니다.`}
      footer={<>
        <Button variant="secondary" onClick={onClose}>닫기</Button>
        <Button onClick={() => navigate(`/create?template=${encodeURIComponent(template.id)}`)}>이 템플릿으로 만들기</Button>
      </>}
    >
      <TemplateHealthPanel template={template} />
      <div className="template-detail">
        <div className="template-detail-preview">
          {selected
            ? <SlidePreview cacheKey={`${template.id}-${selected.id}`} alt={`${selected.name} 레이아웃`} load={() => api.templateLayoutPreview(template.id, selected.id, 880)} />
            : <EmptyState title="레이아웃 정보가 없습니다" description="이 템플릿에서 사용할 수 있는 레이아웃을 찾지 못했습니다." />}
          {selected && <div className="template-detail-slots">
            <strong>{selected.name}</strong>
            <span className="badge badge-violet">{roleLabel(String(selected.role))}</span>
            <ul>{selected.placeholders.filter((placeholder) => placeholder.kind === 'text').map((placeholder) => (
              <li key={placeholder.slot}><code>{placeholder.slot}</code> 최대 {placeholder.maxChars.toLocaleString()}자 · {placeholder.maxLines}줄{placeholder.region ? ` · ${placeholder.region}` : ''}</li>
            ))}</ul>
          </div>}
          {template.palette && <div className="template-palette">
            <div className="template-palette-head">
              <strong>이 템플릿의 데이터 색</strong>
              <span>차트 계열 최대 {template.palette.seriesLimit}개</span>
            </div>
            <div className="template-palette-swatches">
              {template.palette.dataColors.map((color, index) => (
                <span key={color + index} style={{ background: `#${color}` }} title={`#${color}`} />
              ))}
            </div>
            <p>
              본문 글자 대비 {template.palette.inkContrast.toFixed(1)}:1
              {template.palette.inkContrast < 4.5 ? ' — 4.5:1 미만이라 템플릿 자체가 읽기 어렵습니다.' : ' (권장 4.5:1 이상)'}
            </p>
            {template.palette.rejected && <ul>
              {template.palette.rejected.map((entry) => (
                <li key={entry.slot}>
                  <span className="template-palette-chip" style={{ background: `#${entry.color}` }} />
                  <code>{entry.slot}</code> {rejectionText(entry.reason)}
                </li>
              ))}
            </ul>}
          </div>}
        </div>
        <ul className="template-layout-list">
          {layouts.map((layout) => (
            <li key={layout.id}>
              <button className={selected?.id === layout.id ? 'active' : ''} onClick={() => setSelected(layout)}>
                <span>{layout.name}</span>
                <small>{roleLabel(String(layout.role))}</small>
                {selected?.id === layout.id && <em><Check size={12} /></em>}
              </button>
            </li>
          ))}
        </ul>
      </div>
    </Modal>
  )
}

function UploadTemplateModal({ open, onClose, onUploaded }: { open: boolean; onClose: () => void; onUploaded: (template: Template) => void | Promise<void> }) {
  const [file, setFile] = useState<File | null>(null)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [shared, setShared] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [message, setMessage] = useState('')
  const inputRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    if (open) return
    setFile(null); setName(''); setDescription(''); setShared(false); setMessage('')
  }, [open])

  const choose = (chosen: File | null) => {
    setMessage('')
    if (!chosen) { setFile(null); return }
    if (!/\.(pptx|potx)$/i.test(chosen.name)) {
      setMessage('PowerPoint 템플릿(.pptx 또는 .potx)만 업로드할 수 있습니다.')
      setFile(null)
      return
    }
    setFile(chosen)
    if (!name.trim()) setName(chosen.name.replace(/\.(pptx|potx)$/i, ''))
  }

  const submit = async () => {
    if (!file) return
    setUploading(true); setMessage('')
    try {
      const template = await api.uploadTemplate(file, { name: name.trim() || undefined, description: description.trim() || undefined, scope: shared ? 'shared' : 'private' })
      await onUploaded(template)
    } catch (err) {
      setMessage(displayError(err))
    } finally { setUploading(false) }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="PowerPoint 템플릿 업로드"
      description="표지·본문·구역 등 레이아웃이 정의된 회사 표준 템플릿을 올려주세요. 슬라이드가 없는 서식 파일(.potx)도 사용할 수 있습니다."
      footer={<>
        <Button variant="secondary" onClick={onClose} disabled={uploading}>취소</Button>
        <Button onClick={() => void submit()} disabled={!file || uploading}>{uploading ? '분석 중…' : '업로드하고 분석'}</Button>
      </>}
    >
      <div className="upload-dropzone"
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => { event.preventDefault(); choose(event.dataTransfer.files?.[0] || null) }}
      >
        <input ref={inputRef} type="file" accept=".pptx,.potx" hidden onChange={(event) => choose(event.target.files?.[0] || null)} />
        <span className="upload-icon"><Upload size={22} /></span>
        {file ? <div><strong>{file.name}</strong><span>{formatBytes(file.size)}</span></div> : <div><strong>파일을 끌어다 놓거나 선택하세요</strong><span>.pptx · .potx</span></div>}
        <Button variant="secondary" size="small" onClick={() => inputRef.current?.click()}>파일 선택</Button>
      </div>
      <Field label="템플릿 이름"><Input maxLength={120} value={name} onChange={(event) => setName(event.target.value)} placeholder="예: 2026 사내 표준 제안서" /></Field>
      <Field label="설명" hint="어떤 상황에 쓰는 템플릿인지 적어두면 팀이 고르기 쉬워집니다.">
        <Textarea maxLength={1000} value={description} onChange={(event) => setDescription(event.target.value)} rows={3} />
      </Field>
      <label className="checkbox-row">
        <input type="checkbox" checked={shared} onChange={(event) => setShared(event.target.checked)} />
        <span>조직 전체가 사용할 수 있도록 공유합니다</span>
      </label>
      {message && <p className="field-error">{message}</p>}
      {uploading && <LoadingState compact label="레이아웃과 자리 표시자를 분석하고 있어요…" />}
      <p className="modal-note"><Download size={13} /> 업로드한 파일은 원본 그대로 보관되며, 내보낼 때 이 파일에 슬라이드만 추가하는 방식으로 동작합니다.</p>
    </Modal>
  )
}
