import { useCallback, useEffect, useState } from 'react'
import { Plus, Table2, Trash2, Type } from 'lucide-react'
import { api, type GridSpec } from '../api/client'
import { Button, Field, Input, Select } from './UI'
import { displayError } from '../utils'

/** The colour roles a definition may name. It never names a colour: each template
 * resolves a role through its own theme, so one definition works everywhere. */
const roles = [
  { value: 'accent1', label: '강조 1' }, { value: 'accent2', label: '강조 2' },
  { value: 'accent3', label: '강조 3' }, { value: 'accent4', label: '강조 4' },
  { value: 'accent5', label: '강조 5' }, { value: 'accent6', label: '강조 6' },
  { value: 'positive', label: '긍정' }, { value: 'negative', label: '부정' },
  { value: 'muted', label: '흐림' }, { value: 'ink', label: '본문색' },
]

interface ValueRow { key: string; label: string; role: string; chip: boolean; meaning: string }

function toRows(spec: GridSpec): ValueRow[] {
  const keys = spec.order && spec.order.length > 0
    ? spec.order.filter((key) => spec.values?.[key])
    : Object.keys(spec.values || {}).sort()
  const extra = Object.keys(spec.values || {}).filter((key) => !keys.includes(key))
  return [...keys, ...extra].map((key) => ({
    key,
    label: spec.values?.[key]?.label || key,
    role: spec.values?.[key]?.role || 'ink',
    chip: spec.values?.[key]?.chip ?? true,
    meaning: spec.values?.[key]?.meaning || '',
  }))
}

/**
 * Grid definitions, edited as a form rather than as JSON. A definition is data —
 * columns, values, colour roles — so the form is the honest interface to it, and
 * the source only ever refers to it by name.
 */
export function GridLibrary({ onInsert, notify }: {
  onInsert?: (name: string) => void
  notify: (message: string, tone?: 'success' | 'error') => void
}) {
  const [specs, setSpecs] = useState<GridSpec[]>([])
  const [editing, setEditing] = useState<string>('')
  const [draft, setDraft] = useState<GridSpec | null>(null)
  const [rows, setRows] = useState<ValueRow[]>([])
  const [busy, setBusy] = useState(false)

  const reload = useCallback(async () => {
    try {
      setSpecs(await api.grids())
    } catch (err) { notify(displayError(err), 'error') }
  }, [notify])

  useEffect(() => { void reload() }, [reload])

  const open = (spec: GridSpec) => {
    setEditing(spec.name)
    setDraft({ ...spec, columns: spec.columns ? [...spec.columns] : [] })
    setRows(toRows(spec))
  }

  const save = async () => {
    if (!draft) return
    setBusy(true)
    try {
      const values: GridSpec['values'] = {}
      const order: string[] = []
      for (const row of rows) {
        const key = row.key.trim()
        if (!key) continue
        values[key] = { label: row.label.trim() || key, role: row.role, chip: row.chip, meaning: row.meaning.trim() }
        order.push(key)
      }
      await api.saveGrid({ ...draft, values, order })
      notify(`${draft.name} 정의를 저장했습니다.`)
      setDraft(null); setEditing('')
      await reload()
    } catch (err) { notify(displayError(err), 'error') } finally { setBusy(false) }
  }

  const remove = async (name: string) => {
    setBusy(true)
    try {
      await api.deleteGrid(name)
      notify(`${name} 정의를 삭제했습니다. 기본 정의가 있으면 되돌아옵니다.`)
      if (editing === name) { setDraft(null); setEditing('') }
      await reload()
    } catch (err) { notify(displayError(err), 'error') } finally { setBusy(false) }
  }

  if (draft) {
    return (
      <div className="grid-editor">
        <div className="grid-editor-head">
          <strong>{draft.name}</strong>
          <div>
            <Button variant="ghost" onClick={() => { setDraft(null); setEditing('') }}>취소</Button>
            <Button onClick={() => void save()} disabled={busy}>{busy ? '저장 중…' : '저장'}</Button>
          </div>
        </div>
        <Field label="제목" hint="소스에서 캡션을 적지 않으면 이 제목이 격자 위에 붙습니다.">
          <Input value={draft.title || ''} onChange={(event) => setDraft({ ...draft, title: event.target.value })} />
        </Field>
        <div className="grid-editor-toggles">
          <label><input type="checkbox" checked={Boolean(draft.zebra)} onChange={(event) => setDraft({ ...draft, zebra: event.target.checked })} /> 줄 음영</label>
          <label><input type="checkbox" checked={Boolean(draft.legend)} onChange={(event) => setDraft({ ...draft, legend: event.target.checked })} /> 범례</label>
        </div>

        <div className="grid-editor-section">
          <span>첫 열 (행 이름)</span>
          <div className="grid-editor-row">
            <Input placeholder="열 이름" value={draft.columns?.[0]?.label || ''}
              onChange={(event) => setDraft({ ...draft, columns: [{ ...(draft.columns?.[0] || {}), label: event.target.value }] })} />
            <Input type="number" step="0.1" min="0.5" max="6" placeholder="폭 비율"
              value={draft.columns?.[0]?.weight ?? 2}
              onChange={(event) => setDraft({ ...draft, columns: [{ ...(draft.columns?.[0] || {}), weight: Number(event.target.value) || undefined }] })} />
          </div>
          <small>나머지 열은 소스의 첫 행이 정합니다. 폭 비율은 다른 열에 대한 상대값입니다.</small>
        </div>

        <div className="grid-editor-section">
          <span>값 ({rows.length})</span>
          <ul className="grid-value-rows">
            {rows.map((row, index) => (
              <li key={index}>
                <Input placeholder="R" value={row.key} aria-label="소스에 쓰는 값"
                  onChange={(event) => setRows(rows.map((item, at) => at === index ? { ...item, key: event.target.value } : item))} />
                <Input placeholder="실행" value={row.label} aria-label="슬라이드에 보일 글자"
                  onChange={(event) => setRows(rows.map((item, at) => at === index ? { ...item, label: event.target.value } : item))} />
                <Select value={row.role} aria-label="색 역할"
                  onChange={(event) => setRows(rows.map((item, at) => at === index ? { ...item, role: event.target.value } : item))}>
                  {roles.map((role) => <option key={role.value} value={role.value}>{role.label}</option>)}
                </Select>
                <Input placeholder="범례 설명" value={row.meaning} aria-label="범례 설명"
                  onChange={(event) => setRows(rows.map((item, at) => at === index ? { ...item, meaning: event.target.value } : item))} />
                <label title="칩으로 그리기"><input type="checkbox" checked={row.chip}
                  onChange={(event) => setRows(rows.map((item, at) => at === index ? { ...item, chip: event.target.checked } : item))} /> 칩</label>
                <button type="button" className="danger" aria-label="값 삭제" onClick={() => setRows(rows.filter((_, at) => at !== index))}><Trash2 size={13} /></button>
              </li>
            ))}
          </ul>
          <Button variant="ghost" onClick={() => setRows([...rows, { key: '', label: '', role: 'accent1', chip: true, meaning: '' }])}>
            <Plus size={14} /> 값 추가
          </Button>
          <small>값의 순서가 범례 순서입니다. R·A·C·I는 알파벳순이 아니므로 여기서 정합니다.</small>
        </div>
      </div>
    )
  }

  return (
    <div className="grid-library">
      <ul className="grid-list">
        {specs.map((spec) => (
          <li key={spec.name}>
            <div className="grid-list-head">
              <Table2 size={14} />
              <div>
                <strong>{spec.name}</strong>
                <small>{spec.title || '제목 없음'} · 값 {Object.keys(spec.values || {}).length}개</small>
              </div>
            </div>
            <div className="grid-list-actions">
              {onInsert && <button type="button" onClick={() => onInsert(spec.name)}><Type size={13} /> 코드에 넣기</button>}
              <button type="button" onClick={() => open(spec)}>편집</button>
              <button type="button" className="danger" onClick={() => void remove(spec.name)} disabled={busy} aria-label="삭제"><Trash2 size={13} /></button>
            </div>
          </li>
        ))}
      </ul>
      <Button variant="ghost" onClick={() => {
        const name = window.prompt('새 격자 이름 (영문 소문자·숫자·하이픈)')?.trim().toLowerCase()
        if (!name) return
        open({ name, title: '', columns: [{ label: '항목', weight: 2.2, align: 'l' }], values: {}, zebra: true, legend: true })
      }}><Plus size={14} /> 격자 정의 추가</Button>
    </div>
  )
}
