import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Check, Download, Globe, LayoutTemplate, Lock, Plus, Shapes, Trash2, Upload,
} from 'lucide-react'
import { api } from '../api/client'
import { AppShell } from '../components/AppShell'
import { SlidePreview } from '../components/SlidePreview'
import { Badge, Button, EmptyState, ErrorState, Field, Input, LoadingState, Modal, Textarea } from '../components/UI'
import { useToast } from '../components/Toast'
import { navigate } from '../router'
import type { Template, TemplateLayout } from '../types'
import { displayError } from '../utils'

const roleLabels: Record<string, string> = {
  title: '표지', section: '구역', content: '본문', twoContent: '2단',
  comparison: '비교', quote: '인용', picture: '이미지', table: '표',
  chart: '차트', closing: '마무리', blank: '빈 화면',
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

  const mine = useMemo(() => templates.filter((template) => template.kind === 'uploaded'), [templates])
  const builtin = useMemo(() => templates.filter((template) => template.kind === 'builtin'), [templates])

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
                <TemplateCard key={template.id} template={template} onOpen={() => void openDetail(template)} onToggleScope={() => void toggleScope(template)} onDelete={() => void remove(template)} />
              ))}</div>}
        </section>

        <section className="template-section">
          <div className="section-head"><h2>기본 제공 디자인</h2><span>{builtin.length}개</span></div>
          <div className="template-grid">{builtin.map((template) => (
            <TemplateCard key={template.id} template={template} onOpen={() => void openDetail(template)} />
          ))}</div>
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

function TemplateCard({ template, onOpen, onToggleScope, onDelete }: {
  template: Template
  onOpen: () => void
  onToggleScope?: () => void
  onDelete?: () => void
}) {
  return (
    <article className="template-card">
      <button className="template-card-preview" onClick={onOpen} aria-label={`${template.name} 레이아웃 보기`}>
        <SlidePreview cacheKey={`${template.id}-card`} alt={`${template.name} 표지 레이아웃`} load={() => api.templateLayoutPreview(template.id, '', 520)} />
      </button>
      <div className="template-card-body">
        <div className="template-card-title">
          <strong>{template.name}</strong>
          {template.kind === 'builtin' ? <Badge tone="info">기본</Badge> : template.scope === 'shared' ? <Badge tone="success">공유</Badge> : <Badge>개인</Badge>}
        </div>
        {template.description && <p>{template.description}</p>}
        <ul className="template-meta">
          <li><Shapes size={13} /> 레이아웃 {template.layoutCount}개</li>
          {template.aspectRatio && <li>{template.aspectRatio}</li>}
          <li>{formatBytes(template.sizeBytes)}</li>
          {(template.usageCount ?? 0) > 0 && <li>사용 {template.usageCount}회</li>}
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
