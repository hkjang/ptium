import { useEffect, useMemo, useState } from 'react'
import {
  ArrowLeft, ArrowRight, Check, ChevronRight,
  LayoutTemplate, LoaderCircle, MessageSquareText, Palette, Shapes, Sparkles, Star, Upload, WandSparkles,
} from 'lucide-react'
import { api } from '../api/client'
import { designChoices, resolveDesignKey } from '../branding/designs'
import { languageChoices, toneChoices, withStoredChoice } from '../branding/choices'
import { BrandMark, useBrand } from '../branding/BrandContext'
import { SlidePreview } from '../components/SlidePreview'
import { TemplateBrowser, TemplateTile, recommendTemplates } from '../components/TemplateChooser'
import { Button, Field, Input, Modal, Select, Textarea } from '../components/UI'
import { useToast } from '../components/Toast'
import { Link, navigate } from '../router'
import type { Template } from '../types'
import { displayError } from '../utils'

/**
 * audiences are the ones people actually pick, and the labels a stored setting
 * gets shown as.
 *
 * The admin default is stored as a key — "general" — and a form that puts that
 * key in a Korean text field looks broken. Anything an administrator or a person
 * typed themselves is left exactly as written.
 */
const audiences = ['경영진과 의사결정자', '실무 담당자', '고객사·파트너', '투자자', '사내 전체 구성원', '학생·교육생']
const audienceNames: Record<string, string> = {
  general: '일반 청중',
  executive: '경영진과 의사결정자',
  executives: '경영진과 의사결정자',
  practitioner: '실무 담당자',
  practitioners: '실무 담당자',
  technical: '기술 담당자',
  customer: '고객사·파트너',
  customers: '고객사·파트너',
  investor: '투자자',
  investors: '투자자',
  student: '학생·교육생',
  students: '학생·교육생',
  internal: '사내 전체 구성원',
}
function audienceLabel(value: string) {
  const key = value.trim().toLowerCase()
  return audienceNames[key] || value
}

/**
 * The length someone wrote into the brief.
 *
 * "6장짜리 자료"는 길이를 말한 것입니다. 슬라이더에 손대지 않은 사람에게는 그것이 유일하게
 * 밝힌 의도이므로, 그대로 따릅니다.
 */
export function slideCountInBrief(prompt: string) {
  const match = prompt.match(/(\d{1,2})\s*(장|페이지|쪽|슬라이드|slides?|pages?)/i)
  if (!match) return 0
  const count = Number(match[1])
  return count >= 1 && count <= 50 ? count : 0
}

export function CreatePage() {
  const [step, setStep] = useState(1)
  const [prompt, setPrompt] = useState('')
  const [title, setTitle] = useState('')
  const [audience, setAudience] = useState('경영진과 의사결정자')
  const [slideCount, setSlideCount] = useState(10)
  // Once the slider is moved, that is the answer; the brief stops overriding it.
  const [countChosen, setCountChosen] = useState(false)
  const [maxSlides, setMaxSlides] = useState(50)
  const [language, setLanguage] = useState('ko')
  const [theme, setTheme] = useState('aurora')
  const [tone, setTone] = useState('professional')
  const [templates, setTemplates] = useState<Template[]>([])
  const [templateId, setTemplateId] = useState('')
  const [generating, setGenerating] = useState(false)
  const [defaultsLoading, setDefaultsLoading] = useState(true)
  const [generationStage, setGenerationStage] = useState('생성 요청을 검증하고 있어요')
  const { showToast } = useToast()
  const { productName } = useBrand()
  const [browseOpen, setBrowseOpen] = useState(false)
  const canContinue = prompt.trim().length >= 12
  const selectedTemplate = useMemo(() => templates.find((item) => item.id === templateId), [templates, templateId])
  // Recommendations follow the brief, so they change as the brief is written.
  const recommended = useMemo(
    () => recommendTemplates(templates, { prompt, tone, audience }),
    [templates, prompt, tone, audience])
  const choose = (template: Template) => {
    setTemplateId(template.id)
    if (template.paletteKey) setTheme(template.paletteKey)
  }
  // Pinning answers at once and is put back if the server disagrees.
  const favorite = (template: Template, on: boolean) => {
    setTemplates((current) => current.map((item) => item.id === template.id ? { ...item, favorite: on } : item))
    void api.favoriteTemplate(template.id, on).catch((err) => {
      setTemplates((current) => current.map((item) => item.id === template.id ? { ...item, favorite: !on } : item))
      showToast(displayError(err), 'error')
    })
  }
  // What this person already reaches for: pinned first, then most built on.
  const familiar = useMemo(() => templates
    .filter((template) => template.favorite || (template.usageCount || 0) > 0)
    .sort((first, second) => Number(Boolean(second.favorite)) - Number(Boolean(first.favorite))
      || (second.usageCount || 0) - (first.usageCount || 0))
    .slice(0, 4), [templates])
  // A length written into the brief is followed until someone says otherwise.
  const briefCount = useMemo(() => slideCountInBrief(prompt), [prompt])
  useEffect(() => {
    if (!countChosen && briefCount > 0 && briefCount <= maxSlides) setSlideCount(briefCount)
  }, [briefCount, countChosen, maxSlides])

  const examples = useMemo(() => {
    const pitchSlides = Math.min(10, maxSlides)
    return [
      { text: `신규 모바일 서비스의 투자 유치를 위한 ${pitchSlides}장짜리 피치덱`, slideCount: pitchSlides },
      { text: '2026년 상반기 마케팅 성과와 다음 분기 전략 보고서' },
      { text: '생성형 AI를 처음 접하는 실무자를 위한 30분 교육 자료' },
    ]
  }, [maxSlides])

  useEffect(() => {
    let active = true
    const requested = new URLSearchParams(window.location.search).get('template') || ''
    Promise.all([api.publicSettings(), api.profile().catch(() => null), api.templates().catch(() => [] as Template[])]).then(([settings, profile, available]) => {
      if (!active) return
      const configuredMaximum = Math.max(1, Math.min(50, Number(settings['generation.max_slides']) || 50))
      const configuredDefault = Math.max(1, Math.min(configuredMaximum, Number(settings['generation.default_slide_count']) || 10))
      setMaxSlides(configuredMaximum)
      setSlideCount(configuredDefault)
      // A theme stored by an older version — "aurora", "graphite" — is not a
      // design key, and looking for a template with that palette finds nothing.
      // The screens read it the way the server does, and so does this.
      const stored = String(profile?.defaultTheme || settings['generation.default_theme'] || '')
      const configuredTheme = stored ? resolveDesignKey(stored, designChoices(available)) : ''
      if (configuredTheme) setTheme(configuredTheme)

      // A customer's own template outranks the built-in palette, because the
      // whole point of uploading one is that generated decks look like theirs.
      setTemplates(available)
      const preferred = available.find((item) => item.id === requested)
        ?? available.find((item) => item.kind === 'uploaded')
        ?? available.find((item) => item.paletteKey === configuredTheme)
        ?? available[0]
      if (preferred) {
        setTemplateId(preferred.id)
        if (preferred.paletteKey) setTheme(preferred.paletteKey)
      }
      if (requested && !available.some((item) => item.id === requested)) {
        showToast('요청한 템플릿을 찾을 수 없어 다른 디자인을 선택했습니다.', 'error')
      }
      const configuredLanguage = profile?.language || settings['generation.default_lang']
      const configuredTone = profile?.defaultTone || settings['generation.default_tone']
      const configuredAudience = profile?.defaultAudience || settings['generation.default_audience']
      if (configuredLanguage) setLanguage(String(configuredLanguage))
      if (configuredTone) setTone(String(configuredTone))
      if (configuredAudience) setAudience(audienceLabel(String(configuredAudience)))
    }).catch(() => { /* Built-in defaults keep creation available. */ }).finally(() => { if (active) setDefaultsLoading(false) })
    return () => { active = false }
  }, [])

  // A design may be handed in directly: the "generate now" button on the brief
  // picks the top recommendation, and state has not settled by the time it runs.
  const generate = async (design?: Template) => {
    if (defaultsLoading) return
    setGenerating(true); setStep(3); setGenerationStage('생성 요청을 대기열에 등록하고 있어요')
    try {
      const presentation = await api.generatePresentation({
        title: title.trim() || prompt.trim().slice(0, 42), prompt: prompt.trim(), audience,
        slide_count: slideCount, language, theme: design?.paletteKey || theme, tone,
        templateId: design?.id || templateId || undefined,
      })
      setGenerationStage('생성 작업을 시작했어요')
      window.setTimeout(() => navigate(`/presentations/${presentation.id}/editor`), 250)
    } catch (err) {
      setGenerating(false); setStep(2); showToast(displayError(err), 'error')
    }
  }

  if (generating && step === 3) return <GenerationScreen stage={generationStage} templateName={selectedTemplate?.name || ''} prompt={prompt} />

  return (
    <main className="create-page">
      <header className="create-header">
        <Link to="/dashboard" className="brand"><BrandMark /><span>{productName}</span></Link>
        <div className="create-steps" aria-label="생성 단계"><span className={step >= 1 ? 'active' : ''}><i>{step > 1 ? <Check size={13} /> : '1'}</i><b>내용</b></span><em /><span className={step >= 2 ? 'active' : ''}><i>2</i><b>스타일</b></span><em /><span><i>3</i><b>완성</b></span></div>
        <Link to="/dashboard" className="create-exit">나가기</Link>
      </header>
      {step === 1 ? <section className="create-content brief-step">
        <div className="create-title"><span className="step-icon"><MessageSquareText size={22} /></span><span className="eyebrow">STEP 01 · BRIEF</span><h1>어떤 프레젠테이션이<br />필요한가요?</h1><p>완벽하게 쓰지 않아도 괜찮아요. {productName}이 아이디어를 구조화합니다.</p></div>
        <div className="prompt-composer">
          <Textarea autoFocus disabled={defaultsLoading} value={prompt} onChange={(event) => setPrompt(event.target.value)} placeholder="예: 친환경 패키징 솔루션의 시장 기회와 제품 경쟁력을 설명하는 투자자용 피치덱을 만들어줘. 핵심 지표와 3개년 성장 전략을 포함해줘." maxLength={2000} />
          <div className="composer-tools"><div /><span>{prompt.length.toLocaleString()} / 2,000</span></div>
        </div>
        <div className="prompt-examples"><span>이런 식으로 시작해 보세요</span>{examples.map((example) => <button key={example.text} disabled={defaultsLoading} onClick={() => { setPrompt(example.text); if (example.slideCount) setSlideCount(example.slideCount) }}><Sparkles size={14} /> {example.text}<ChevronRight size={14} /></button>)}</div>
        <div className="create-footer">
          <span>{defaultsLoading ? '개인·조직 기본값을 불러오는 중…'
            : !canContinue && prompt.length > 0 ? '조금 더 구체적으로 설명해 주세요.'
            : canContinue ? `${briefCount > 0 ? briefCount : slideCount}장으로 만듭니다. 다음 단계에서 바꿀 수 있어요.` : ''}</span>
          {/* Choosing a design is optional. Someone who just wants the deck gets
              the recommended one and can change it in the editor afterwards. */}
          <button className="button button-secondary button-large" disabled={defaultsLoading || !canContinue || generating}
            onClick={() => { if (recommended[0]) choose(recommended[0]); void generate(recommended[0]) }}
            title={recommended[0] ? `${recommended[0].name} 디자인으로 바로 만듭니다` : undefined}>
            <WandSparkles size={17} /> 추천 디자인으로 바로 생성
          </button>
          <Button size="large" disabled={defaultsLoading || !canContinue} onClick={() => setStep(2)}>디자인 고르기 <ArrowRight size={17} /></Button>
        </div>
      </section> : <section className="create-content style-step">
        <button className="back-link" onClick={() => setStep(1)}><ArrowLeft size={16} /> 내용 수정하기</button>
        <div className="style-layout">
          <div className="style-form">
            <div className="create-title compact"><span className="step-icon"><Palette size={22} /></span><span className="eyebrow">STEP 02 · STYLE</span><h1>발표에 어울리는<br />분위기를 선택하세요.</h1><p>나중에 편집기에서도 언제든 바꿀 수 있어요.</p></div>
            <div className="form-card">
              <Field label="프레젠테이션 제목" hint="비워두면 입력한 주제의 앞부분을 제목으로 사용합니다."><Input maxLength={200} value={title} onChange={(event) => setTitle(event.target.value)} placeholder="주제에서 제목 만들기" /></Field>
              <div className="form-grid two"><Field label="청중" hint="누구에게 말하는지에 따라 문장의 높이와 근거의 종류가 달라집니다."><Input maxLength={300} value={audience} onChange={(event) => setAudience(event.target.value)} list="audience-options" placeholder="예: 경영진과 의사결정자" /><datalist id="audience-options">{audiences.map((item) => <option key={item} value={item} />)}</datalist><div className="chip-row">{audiences.slice(0, 4).map((item) => <button type="button" key={item} className={audience === item ? 'active' : ''} onClick={() => setAudience(item)}>{item}</button>)}</div></Field><Field label="발표 톤"><Select value={tone} onChange={(event) => setTone(event.target.value)}>{withStoredChoice(toneChoices, tone).map((choice) => <option key={choice.id} value={choice.id}>{choice.label}</option>)}</Select></Field></div>
              <div className="form-grid two"><Field label="슬라이드 수" hint={!countChosen && briefCount > 0 ? `브리프에 적은 ${briefCount}장을 그대로 씁니다.` : undefined}><div className="range-field"><input type="range" min="1" max={maxSlides} value={slideCount} onChange={(event) => { setCountChosen(true); setSlideCount(Number(event.target.value)) }} /><strong>{slideCount}장</strong></div></Field><Field label="언어"><Select value={language} onChange={(event) => setLanguage(event.target.value)}>{withStoredChoice(languageChoices, language).map((choice) => <option key={choice.id} value={choice.id}>{choice.label}</option>)}</Select></Field></div>
            </div>
          </div>
          <TemplatePicker
            templates={templates}
            // A design already on the shelf above does not need suggesting again.
            recommended={recommended.filter((template) => !familiar.some((item) => item.id === template.id))}
            familiar={familiar}
            selectedId={templateId}
            loading={defaultsLoading}
            onSelect={choose}
            onFavorite={favorite}
            onBrowse={() => setBrowseOpen(true)}
          />
        </div>
        <div className="create-footer style-footer">
          <button className="button button-secondary button-large" onClick={() => setStep(1)}><ArrowLeft size={17} /> 이전</button>
          <div className="create-footer-summary">{selectedTemplate
            ? <span><LayoutTemplate size={14} /> {selectedTemplate.name} · 레이아웃 {selectedTemplate.layoutCount}개 사용</span>
            : <span>사용 가능한 템플릿을 찾지 못했습니다. 기본 디자인으로 생성됩니다.</span>}</div>
          <Button size="large" disabled={defaultsLoading || generating} onClick={() => void generate()}><WandSparkles size={18} /> 슬라이드 초안 생성</Button>
        </div>
      </section>}
      <Modal
        open={browseOpen}
        onClose={() => setBrowseOpen(false)}
        title="모든 디자인"
        description="밝기·구성·용도로 좁혀서 고르세요. 고르면 바로 적용됩니다."
        wide
        footer={<Button variant="secondary" onClick={() => setBrowseOpen(false)}>닫기</Button>}
      >
        <TemplateBrowser templates={templates} selectedId={templateId} onFavorite={favorite}
          onSelect={(template) => { choose(template); setBrowseOpen(false) }} />
      </Modal>
    </main>
  )
}

/**
 * The design step: the chosen design, large; a few that suit the brief; and the
 * whole library one click away. Scrolling forty covers to start writing is not a
 * choice anyone wanted to make.
 */
function TemplatePicker({ templates, recommended, familiar, selectedId, loading, onSelect, onFavorite, onBrowse }: {
  templates: Template[]
  recommended: Template[]
  /** Pinned or already built on: the shortest path for someone with a habit. */
  familiar: Template[]
  selectedId: string
  loading: boolean
  onSelect: (template: Template) => void
  onFavorite: (template: Template, favorite: boolean) => void
  onBrowse: () => void
}) {
  const selected = templates.find((template) => template.id === selectedId)
  return (
    <div className="template-picker">
      <div className="theme-picker-head">
        <div><span className="eyebrow">TEMPLATE</span><h2>디자인</h2></div>
        <LayoutTemplate size={20} />
      </div>
      {loading ? <div className="template-picker-loading"><LoaderCircle className="spin" size={18} /> 템플릿을 불러오는 중…</div>
        : templates.length === 0
          ? <div className="template-picker-empty">
              <p>사용할 수 있는 템플릿이 없습니다.</p>
              <Link to="/templates" className="button button-secondary button-small"><Upload size={14} /> 템플릿 업로드</Link>
            </div>
          : <>
            {selected && <div className="template-chosen">
              <SlidePreview cacheKey={`chosen-${selected.id}`} alt={`${selected.name} 표지`}
                load={() => api.templateLayoutPreview(selected.id, '', 640)} />
              <div>
                <strong>{selected.name}</strong>
                <span><Shapes size={12} /> 레이아웃 {selected.layoutCount}개{selected.aspectRatio ? ` · ${selected.aspectRatio}` : ''}</span>
                {(selected.tags || []).length > 0 && <em>{(selected.tags || []).join(' · ')}</em>}
              </div>
            </div>}
            {familiar.length > 0 && <div className="template-picker-group">
              <span className="template-picker-group-label"><Star size={11} /> 자주 쓰는 디자인</span>
              <div className="template-suggestions">{familiar.map((template) => (
                <TemplateTile key={template.id} template={template} size={300}
                  selected={template.id === selectedId} onSelect={() => onSelect(template)}
                  onFavorite={(favorite) => onFavorite(template, favorite)} />
              ))}</div>
            </div>}
            <div className="template-picker-group">
              <span className="template-picker-group-label">이 주제에 어울리는 디자인</span>
              <div className="template-suggestions">{recommended.map((template) => (
                <TemplateTile key={template.id} template={template} size={300}
                  selected={template.id === selectedId} onSelect={() => onSelect(template)}
                  onFavorite={(favorite) => onFavorite(template, favorite)} />
              ))}</div>
            </div>
            <button type="button" className="template-picker-browse" onClick={onBrowse}>
              <LayoutTemplate size={14} /> 모든 디자인 보기 ({templates.length})
            </button>
            <Link to="/templates" className="template-picker-manage"><Upload size={13} /> 회사 템플릿 업로드 및 관리</Link>
          </>}
    </div>
  )
}

function GenerationScreen({ stage, templateName, prompt }: { stage: string; templateName: string; prompt: string }) {
  const { productName } = useBrand()
  return <main className="generation-page">
    <div className="generation-brand"><BrandMark /><span>{productName}</span></div>
    <section className="generation-content"><div className="generation-visual"><div className="generating-slide theme-aurora"><span>{templateName || 'PTIUM ENGINE'}</span><div><i /><i /><i /></div><strong>{prompt.slice(0, 55)}{prompt.length > 55 ? '…' : ''}</strong><em /></div><div className="generation-spark spark-one"><Sparkles size={15} /></div><div className="generation-spark spark-two"><Sparkles size={11} /></div><span className="orbit orbit-one" /><span className="orbit orbit-two" /></div><span className="eyebrow">CREATING YOUR STORY</span><h1>아이디어를 생성 대기열에<br />등록하고 있어요.</h1><p>{stage}</p><div className="generation-tasks"><span className="done"><Check size={13} /> 입력 검증</span><span className="active"><LoaderCircle className="spin" size={13} /> 작업 등록</span><span><span /> 편집기 연결</span></div><small>등록 후 편집기에서 서버의 실제 생성 상태를 확인합니다.</small></section>
  </main>
}
