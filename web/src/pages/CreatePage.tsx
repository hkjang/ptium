import { useEffect, useMemo, useState } from 'react'
import {
  ArrowLeft, ArrowRight, Check, ChevronRight,
  LayoutTemplate, LoaderCircle, MessageSquareText, Palette, Shapes, Sparkles, Upload, WandSparkles,
} from 'lucide-react'
import { api } from '../api/client'
import { BrandMark, useBrand } from '../branding/BrandContext'
import { SlidePreview } from '../components/SlidePreview'
import { Button, Field, Input, Select, Textarea } from '../components/UI'
import { useToast } from '../components/Toast'
import { Link, navigate } from '../router'
import type { Template } from '../types'
import { displayError } from '../utils'

const themes = [
  { id: 'aurora', name: 'Aurora', description: '세련되고 선명한', colors: ['#17162d', '#8d72ff', '#f1c6ff'] },
  { id: 'paper', name: 'Editorial', description: '차분하고 지적인', colors: ['#f4efe6', '#22201e', '#ce654a'] },
  { id: 'mint', name: 'Fresh', description: '밝고 친근한', colors: ['#dff8ed', '#153a35', '#57c59d'] },
  { id: 'graphite', name: 'Graphite', description: '전문적이고 절제된', colors: ['#22262d', '#f5f5f3', '#9da6b2'] },
]

export function CreatePage() {
  const [step, setStep] = useState(1)
  const [prompt, setPrompt] = useState('')
  const [title, setTitle] = useState('')
  const [audience, setAudience] = useState('경영진과 의사결정자')
  const [slideCount, setSlideCount] = useState(10)
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
  const canContinue = prompt.trim().length >= 12
  const selectedTheme = useMemo(() => themes.find((item) => item.id === theme) ?? themes[0], [theme])
  const selectedTemplate = useMemo(() => templates.find((item) => item.id === templateId), [templates, templateId])
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
      const configuredTheme = String(profile?.defaultTheme || settings['generation.default_theme'] || '')
      if (themes.some((item) => item.id === configuredTheme)) setTheme(configuredTheme)

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
      if (configuredAudience) setAudience(String(configuredAudience))
    }).catch(() => { /* Built-in defaults keep creation available. */ }).finally(() => { if (active) setDefaultsLoading(false) })
    return () => { active = false }
  }, [])

  const generate = async () => {
    if (defaultsLoading) return
    setGenerating(true); setStep(3); setGenerationStage('생성 요청을 대기열에 등록하고 있어요')
    try {
      const presentation = await api.generatePresentation({
        title: title.trim() || prompt.trim().slice(0, 42), prompt: prompt.trim(), audience,
        slide_count: slideCount, language, theme, tone, templateId: templateId || undefined,
      })
      setGenerationStage('생성 작업을 시작했어요')
      window.setTimeout(() => navigate(`/presentations/${presentation.id}/editor`), 250)
    } catch (err) {
      setGenerating(false); setStep(2); showToast(displayError(err), 'error')
    }
  }

  if (generating && step === 3) return <GenerationScreen stage={generationStage} theme={selectedTheme} prompt={prompt} />

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
        <div className="create-footer"><span>{defaultsLoading ? '개인·조직 기본값을 불러오는 중…' : !canContinue && prompt.length > 0 ? '조금 더 구체적으로 설명해 주세요.' : ''}</span><Button size="large" disabled={defaultsLoading || !canContinue} onClick={() => setStep(2)}>스타일 선택하기 <ArrowRight size={17} /></Button></div>
      </section> : <section className="create-content style-step">
        <button className="back-link" onClick={() => setStep(1)}><ArrowLeft size={16} /> 내용 수정하기</button>
        <div className="style-layout">
          <div className="style-form">
            <div className="create-title compact"><span className="step-icon"><Palette size={22} /></span><span className="eyebrow">STEP 02 · STYLE</span><h1>발표에 어울리는<br />분위기를 선택하세요.</h1><p>나중에 편집기에서도 언제든 바꿀 수 있어요.</p></div>
            <div className="form-card">
              <Field label="프레젠테이션 제목" hint="비워두면 입력한 주제의 앞부분을 제목으로 사용합니다."><Input maxLength={200} value={title} onChange={(event) => setTitle(event.target.value)} placeholder="주제에서 제목 만들기" /></Field>
              <div className="form-grid two"><Field label="청중"><Input maxLength={300} value={audience} onChange={(event) => setAudience(event.target.value)} /></Field><Field label="발표 톤"><Select value={tone} onChange={(event) => setTone(event.target.value)}><option value="professional">전문적</option><option value="persuasive">설득력 있는</option><option value="friendly">친근한</option><option value="inspiring">영감을 주는</option><option value="academic">학술적인</option></Select></Field></div>
              <div className="form-grid two"><Field label="슬라이드 수"><div className="range-field"><input type="range" min="1" max={maxSlides} value={slideCount} onChange={(event) => setSlideCount(Number(event.target.value))} /><strong>{slideCount}장</strong></div></Field><Field label="언어"><Select value={language} onChange={(event) => setLanguage(event.target.value)}><option value="ko">한국어</option><option value="en">English</option><option value="ja">日本語</option><option value="zh">中文</option></Select></Field></div>
            </div>
          </div>
          <TemplatePicker
            templates={templates}
            selectedId={templateId}
            loading={defaultsLoading}
            onSelect={(template) => { setTemplateId(template.id); if (template.paletteKey) setTheme(template.paletteKey) }}
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
    </main>
  )
}

function TemplatePicker({ templates, selectedId, loading, onSelect }: {
  templates: Template[]
  selectedId: string
  loading: boolean
  onSelect: (template: Template) => void
}) {
  const mine = templates.filter((template) => template.kind === 'uploaded')
  const builtin = templates.filter((template) => template.kind === 'builtin')
  const groups = [
    { label: '내 템플릿', items: mine },
    { label: '기본 제공 디자인', items: builtin },
  ].filter((group) => group.items.length > 0)

  return (
    <div className="template-picker">
      <div className="theme-picker-head">
        <div><span className="eyebrow">TEMPLATE</span><h2>디자인 템플릿</h2></div>
        <LayoutTemplate size={20} />
      </div>
      {loading ? <div className="template-picker-loading"><LoaderCircle className="spin" size={18} /> 템플릿을 불러오는 중…</div> : groups.length === 0
        ? <div className="template-picker-empty">
            <p>사용할 수 있는 템플릿이 없습니다.</p>
            <Link to="/templates" className="button button-secondary button-small"><Upload size={14} /> 템플릿 업로드</Link>
          </div>
        : <>
          {groups.map((group) => (
            <div key={group.label} className="template-picker-group">
              <span className="template-picker-group-label">{group.label}</span>
              <div className="template-picker-options">{group.items.map((template) => (
                <button
                  key={template.id}
                  className={`template-option ${selectedId === template.id ? 'selected' : ''}`}
                  onClick={() => onSelect(template)}
                  aria-pressed={selectedId === template.id}
                >
                  <SlidePreview cacheKey={`pick-${template.id}`} alt={`${template.name} 표지`} load={() => api.templateLayoutPreview(template.id, '', 420)} />
                  <div>
                    <strong>{template.name}</strong>
                    <span><Shapes size={12} /> 레이아웃 {template.layoutCount}개{template.aspectRatio ? ` · ${template.aspectRatio}` : ''}</span>
                  </div>
                  {selectedId === template.id && <em><Check size={13} /></em>}
                </button>
              ))}</div>
            </div>
          ))}
          <Link to="/templates" className="template-picker-manage"><Upload size={13} /> 회사 템플릿 업로드 및 관리</Link>
        </>}
    </div>
  )
}

function GenerationScreen({ stage, theme, prompt }: { stage: string; theme: typeof themes[number]; prompt: string }) {
  const { productName } = useBrand()
  return <main className="generation-page">
    <div className="generation-brand"><BrandMark /><span>{productName}</span></div>
    <section className="generation-content"><div className="generation-visual"><div className={`generating-slide theme-${theme.id}`}><span>PTIUM ENGINE</span><div><i /><i /><i /></div><strong>{prompt.slice(0, 55)}{prompt.length > 55 ? '…' : ''}</strong><em /></div><div className="generation-spark spark-one"><Sparkles size={15} /></div><div className="generation-spark spark-two"><Sparkles size={11} /></div><span className="orbit orbit-one" /><span className="orbit orbit-two" /></div><span className="eyebrow">CREATING YOUR STORY</span><h1>아이디어를 생성 대기열에<br />등록하고 있어요.</h1><p>{stage}</p><div className="generation-tasks"><span className="done"><Check size={13} /> 입력 검증</span><span className="active"><LoaderCircle className="spin" size={13} /> 작업 등록</span><span><span /> 편집기 연결</span></div><small>등록 후 편집기에서 서버의 실제 생성 상태를 확인합니다.</small></section>
  </main>
}
