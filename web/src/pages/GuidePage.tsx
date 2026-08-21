import { useEffect, useMemo, useState } from 'react'
import {
  BookOpenCheck, Download, Image as ImageIcon, Keyboard, LayoutTemplate, MessageSquareText,
  MonitorPlay, MousePointerClick, Sparkles, TriangleAlert,
} from 'lucide-react'
import { AppShell } from '../components/AppShell'
import { ShortcutTable, editorShortcuts, presentationShortcuts } from '../components/Shortcuts'
import { useBrand } from '../branding/BrandContext'
import { Link } from '../router'

/**
 * The usage guide.
 *
 * Everything here is a thing the product already does and nobody could have
 * guessed: which words make a good brief, what the canvas will and will not let
 * go of, the six characters the source language is made of, and what the
 * measurement warnings actually mean. It is one page rather than a help centre
 * because a person looking something up mid-deck will read one page and will not
 * navigate a tree.
 */

interface Section { id: string; title: string; icon: typeof Sparkles }

const sections: Section[] = [
  { id: 'quickstart', title: '5분 만에 첫 덱', icon: Sparkles },
  { id: 'brief', title: '브리프 잘 쓰기', icon: MessageSquareText },
  { id: 'templates', title: '템플릿 고르기', icon: LayoutTemplate },
  { id: 'editing', title: '편집하기', icon: MousePointerClick },
  { id: 'images', title: '이미지 넣기', icon: ImageIcon },
  { id: 'source', title: '코드로 쓰기', icon: BookOpenCheck },
  { id: 'present', title: '발표와 내보내기', icon: MonitorPlay },
  { id: 'shortcuts', title: '단축키', icon: Keyboard },
  { id: 'trouble', title: '잘 안 될 때', icon: TriangleAlert },
]

export function GuidePage() {
  const { productName } = useBrand()
  const [active, setActive] = useState('quickstart')

  // The contents follow the reader, so a long page still says where they are.
  useEffect(() => {
    const headings = sections.map((section) => document.getElementById(section.id)).filter(Boolean) as HTMLElement[]
    const observer = new IntersectionObserver((entries) => {
      const visible = entries.filter((entry) => entry.isIntersecting)
        .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)[0]
      if (visible) setActive(visible.target.id)
    }, { rootMargin: '-96px 0px -60% 0px' })
    headings.forEach((heading) => observer.observe(heading))
    return () => observer.disconnect()
  }, [])

  const sourceExample = useMemo(() => [
    '# 클라우드 전환 로드맵',
    '@cover',
    '> 2026년 하반기 · 임원 보고',
    '',
    '# 왜 지금인가',
    '- 온프레미스 계약이 9월에 끝납니다',
    '- 재계약가는 전년 대비 22% 인상됩니다',
    '::kpi 핵심 지표',
    '- 전환 대상 | 42개',
    '- 예상 절감 | 18%',
    '::',
    '!notes 예산 질문이 나오면 3장의 표를 보여 줍니다.',
  ].join('\n'), [])

  return <AppShell title="사용 가이드" eyebrow="USER GUIDE">
    <div className="guide-layout">
      <nav className="guide-toc" aria-label="목차">
        {sections.map((section) => (
          <a key={section.id} href={`#${section.id}`} className={active === section.id ? 'active' : ''}>
            <section.icon size={15} />{section.title}
          </a>
        ))}
        <Link to="/create" className="button button-primary button-small guide-toc-cta"><Sparkles size={14} /> 바로 만들어 보기</Link>
      </nav>

      <article className="guide-body">
        <section id="quickstart" className="guide-section">
          <h2><Sparkles size={19} /> 5분 만에 첫 덱</h2>
          <p className="guide-lead">{productName}은 <b>쓴 글</b>을 <b>가지고 있는 템플릿</b>에 맞춰 그립니다.
            그래서 순서는 언제나 같습니다 — 무엇을 말할지 적고, 어느 디자인에 담을지 고르고, 고칩니다.</p>
          <ol className="guide-steps">
            <li><b>새로 만들기</b>에서 무엇을 발표할지 두세 문장으로 적습니다.</li>
            <li><b>추천 디자인으로 바로 생성</b>을 누르거나, <b>디자인 고르기</b>에서 청중·톤·장수를 조정합니다.</li>
            <li>편집기가 열리고 생성이 끝나면 슬라이드가 채워집니다. 여기서부터는 전부 고칠 수 있습니다.</li>
            <li>왼쪽 목록에서 장을 고르고, 가운데 캔버스에서 <b>더블클릭</b>해 글을 고칩니다.</li>
            <li>다 됐으면 <b>발표</b>로 보여 주거나 <b>내보내기</b>로 PowerPoint 파일을 받습니다.</li>
          </ol>
          <p className="guide-note">저장 버튼은 없습니다. 고치는 즉시 자동 저장되고, 버전 이력에서 예전 상태로 되돌릴 수 있습니다.</p>
        </section>

        <section id="brief" className="guide-section">
          <h2><MessageSquareText size={19} /> 브리프 잘 쓰기</h2>
          <p>생성 품질의 대부분은 첫 화면에 적는 몇 문장에서 결정됩니다. 네 가지만 있으면 충분합니다.</p>
          <ul className="guide-checklist">
            <li><b>주제</b> — 무엇에 대한 발표인지. "AI 도입"보다 "사내 개발팀의 AI 코딩 도구 도입 성과".</li>
            <li><b>청중</b> — 경영진인지 실무자인지 고객인지. 같은 내용도 청중에 따라 다른 덱이 됩니다.</li>
            <li><b>목적</b> — 승인을 받으려는 것인지, 공유하려는 것인지, 가르치려는 것인지.</li>
            <li><b>숫자</b> — 아는 수치는 그대로 적습니다. "개발 속도 32% 개선, 12개월 ROI"처럼 적으면
              지표 컴포넌트로 그려집니다. 적지 않은 숫자를 지어내지는 않습니다.</li>
          </ul>
          <div className="guide-compare">
            <div className="guide-compare-bad"><span>아쉬운 브리프</span><p>AI 도입 발표자료 만들어줘</p></div>
            <div className="guide-compare-good"><span>좋은 브리프</span><p>사내 개발팀의 AI 코딩 도구 도입 성과를 경영진에게 보고하는 8장짜리 덱.
              개발 속도 32% 개선, 12개월 ROI, 내년 전사 확대 방안을 포함해줘.</p></div>
          </div>
          <p className="guide-note">장수는 브리프에 "8장으로"라고 써도 되고, 스타일 단계의 슬라이드 수로 지정해도 됩니다.
            둘 다 있으면 슬라이드 수 설정을 따릅니다.</p>
        </section>

        <section id="templates" className="guide-section">
          <h2><LayoutTemplate size={19} /> 템플릿 고르기</h2>
          <p>기본 제공 디자인은 50종입니다. 밝기(밝은/어두운), 구성(여백형·2단·그림형), 성격(정중한·역동적인)
            태그로 좁혀서 고르세요. 브리프를 적으면 어울리는 몇 개를 먼저 보여 줍니다.</p>
          <ul className="guide-checklist">
            <li><b>회사 템플릿</b> — 가지고 있는 .pptx를 <Link to="/templates">템플릿</Link>에서 올리면
              그 파일의 레이아웃·글꼴·색을 그대로 써서 생성합니다. 슬라이드 마스터의 자리 표시자가
              많을수록 잘 맞습니다.</li>
            <li><b>메타포 계열</b> — Orbit(성장)·Arc(여정)·Diagonal(전환)·Dots(데이터)·Layers(구조)·Wash(비전)는
              표지에서 도형으로 이야기를 한 번 말해 줍니다. 주제와 맞을 때만 고르세요.</li>
            <li><b>자주 쓰는 디자인</b> — 별표를 눌러 둔 디자인과 실제로 덱을 만든 디자인은 템플릿 화면과
              새로 만들기에서 맨 앞에 모아 보여 주고, 추천에도 먼저 반영됩니다.</li>
            <li>디자인은 나중에 편집기에서 바꿔도 글이 그대로 옮겨집니다.</li>
          </ul>
        </section>

        <section id="editing" className="guide-section">
          <h2><MousePointerClick size={19} /> 편집하기</h2>
          <p>캔버스에는 성격이 다른 두 가지가 있습니다. 구분이 되면 편집기 전체가 쉬워집니다.</p>
          <div className="guide-two">
            <div className="guide-card">
              <strong>템플릿 영역</strong>
              <p>제목·본문처럼 <b>템플릿이 정해 준 자리</b>. AI가 쓴 글은 여기에 들어갑니다.
                더블클릭해 고치고, 끌어서 옮기고, 오른쪽 클릭으로 <b>원래 자리로</b> 되돌릴 수 있습니다.
                내보낸 PPTX에서도 진짜 자리 표시자로 남습니다.</p>
            </div>
            <div className="guide-card">
              <strong>덧붙인 개체</strong>
              <p>글상자·도형·표·이미지처럼 <b>내가 얹은 것</b>. 자유롭게 놓이고, 그룹·정렬·순서 바꾸기가 됩니다.</p>
            </div>
          </div>
          <h3 className="guide-subhead">저장한 슬라이드</h3>
          <p>회사 소개·팀·연락처·면책조항처럼 <b>매번 다시 만드는 슬라이드</b>는 오른쪽 <b>슬라이드</b> 탭에서
            라이브러리에 저장해 두고 어느 덱에나 넣습니다.</p>
          <ul className="guide-checklist">
            <li>그림이 아니라 <b>글로 저장</b>되므로, 넣는 순간 <b>그 덱의 디자인</b>으로 다시 그려집니다.
              어두운 템플릿에 넣으면 어두운 슬라이드가 됩니다.</li>
            <li>미리보기도 지금 덱의 디자인으로 그려서 보여 줍니다 — 넣기 전에 어떻게 될지 보입니다.</li>
            <li>즐겨찾기·태그·검색은 이미지 라이브러리와 같습니다. 자주 넣는 것이 앞에 옵니다.</li>
            <li>넣고 나면 그냥 보통 슬라이드입니다. 원본을 지워도 이미 넣은 슬라이드는 그대로 남습니다.</li>
          </ul>
          <ul className="guide-checklist">
            <li><b>AI로 다시 쓰기</b> — 영역을 오른쪽 클릭하면 그 한 장만 다시 씁니다. 마음에 안 들면 되돌리기.</li>
            <li><b>측정 결과</b> — 편집기 위의 배지는 글자가 상자를 넘쳤는지, 겹쳤는지를 실제로 그려 보고 셉니다.
              "결함"은 잘못 그려진 것, "다듬을 곳"은 그래도 되지만 더 좋아질 수 있는 것입니다.</li>
            <li><b>슬라이드 순서</b>는 왼쪽 목록에서 끌거나 <kbd>Alt</kbd>+<kbd>↑</kbd>/<kbd>↓</kbd>로 바꿉니다.</li>
          </ul>
        </section>

        <section id="images" className="guide-section">
          <h2><ImageIcon size={19} /> 이미지 넣기</h2>
          <ul className="guide-checklist">
            <li><b>붙여넣기</b> — 캡처한 이미지를 캔버스에서 <kbd>Ctrl</kbd>+<kbd>V</kbd>. 올리고 배치까지 한 번에 됩니다.</li>
            <li><b>끌어다 놓기</b> — 이미지 파일을 캔버스에 놓으면 놓은 자리에 들어갑니다.</li>
            <li><b>영역 채우기</b> — 그림 자리가 있는 레이아웃이면 그 영역을 오른쪽 클릭 → 이미지.</li>
            <li><b>코드에서</b> — 올린 이미지는 이름이 있고, 소스에 <code>::image 로고</code>로 부릅니다.</li>
          </ul>
          <p className="guide-note">PNG · JPEG · GIF · SVG, 한 장당 16MB까지. 같은 이름으로 다시 올리면 교체되므로,
            로고를 바꾸면 그 이름을 쓰던 덱들이 함께 새 로고가 됩니다. 같은 파일을 다시 올리면 새로 만들지 않고
            이미 있는 이미지를 씁니다.</p>
          <h3 className="guide-subhead">내 이미지 (라이브러리)</h3>
          <p>올린 이미지는 계정에 남아 모든 덱에서 다시 씁니다. <Link to="/images">내 이미지</Link>에서
            자기만의 체계를 만들 수 있습니다.</p>
          <ul className="guide-checklist">
            <li><b>즐겨찾기</b> — 별표를 누르면 어느 정렬에서도 맨 앞에 옵니다. 로고처럼 매번 쓰는 것에.</li>
            <li><b>태그</b> — 로고·제품컷·배경처럼 쓰임으로 묶습니다. 태그를 누르면 그것만 보입니다.</li>
            <li><b>덱 N개</b> — 그 이미지를 실제로 쓰는 덱의 수입니다. 세어 둔 값이 아니라 저장할 때마다
              다시 세므로, 덱에서 빼면 바로 줄어듭니다. 지워도 되는 이미지가 여기서 보입니다.</li>
            <li><b>최근 사용 · 자주 사용</b>으로 정렬해 "늘 쓰는 그 이미지"를 먼저 찾습니다.</li>
            <li>이름은 두 번 눌러 바로 고칩니다. 코드에서 부르던 이름도 함께 바뀝니다.</li>
          </ul>
        </section>

        <section id="source" className="guide-section">
          <h2><BookOpenCheck size={19} /> 코드로 쓰기</h2>
          <p>덱은 원래 텍스트입니다. 편집기의 <b>코드</b> 탭에서 덱 전체를 글로 보고 고칠 수 있습니다.
            표와 지표를 한꺼번에 손볼 때는 이쪽이 훨씬 빠릅니다.</p>
          <pre className="guide-code">{sourceExample}</pre>
          <dl className="guide-syntax">
            <div><dt><code>#</code></dt><dd>새 슬라이드와 제목</dd></div>
            <div><dt><code>@표지</code></dt><dd>슬라이드 종류 (표지·간지·본문·비교·인용·마무리…)</dd></div>
            <div><dt><code>&gt;</code></dt><dd>제목 아래 리드 문장</dd></div>
            <div><dt><code>-</code></dt><dd>요점. 두 칸 들여쓰면 하위 요점</dd></div>
            <div><dt><code>::지표 … ::</code></dt><dd>컴포넌트. 행은 <code>이름 | 값 | 설명</code></dd></div>
            <div><dt><code>!notes</code></dt><dd>발표자 노트</dd></div>
          </dl>
          <p className="guide-note">전체 문법은 <a href="https://github.com/hkjang/ptium/blob/main/docs/deck-source.md" target="_blank" rel="noreferrer">deck-source 문서</a>에 있습니다.
            코드를 적용하면 슬라이드가 다시 그려지고, 캔버스에서 옮겨 둔 위치는 유지됩니다.</p>
        </section>

        <section id="present" className="guide-section">
          <h2><MonitorPlay size={19} /> 발표와 내보내기</h2>
          <ul className="guide-checklist">
            <li><b>발표</b>(<kbd>F5</kbd>) — 청중 화면에는 슬라이드만 나옵니다.</li>
            <li><b>발표자 보기</b>(<kbd>P</kbd>) — 두 번째 창에 다음 장·노트·경과 시간이 나옵니다.
              프로젝터에 발표 창을 두고 노트북에 발표자 창을 두세요. 어느 쪽에서 넘겨도 함께 움직입니다.</li>
            <li><b>내보내기</b> — PPTX로 받습니다. 도형·표·차트가 진짜 개체이고 자리 표시자가 그대로
              남아 PowerPoint에서 계속 편집됩니다. PDF가 필요하면 받은 파일을 PowerPoint나
              LibreOffice에서 PDF로 저장하세요 — 글꼴이 설치된 그 컴퓨터에서 만드는 편이 정확합니다.</li>
          </ul>
        </section>

        <section id="shortcuts" className="guide-section">
          <h2><Keyboard size={19} /> 단축키</h2>
          <p>편집기와 발표 화면 어디서나 <kbd>?</kbd>를 누르면 이 표가 뜹니다.</p>
          <h3 className="guide-subhead">편집기</h3>
          <ShortcutTable groups={editorShortcuts} />
          <h3 className="guide-subhead">발표 중</h3>
          <ShortcutTable groups={presentationShortcuts} />
        </section>

        <section id="trouble" className="guide-section">
          <h2><TriangleAlert size={19} /> 잘 안 될 때</h2>
          <dl className="guide-faq">
            <div><dt>생성이 실패했어요</dt><dd>편집기 위쪽에 실패 이유가 그대로 나옵니다. <b>다시 시도</b>를 누르면
              같은 브리프로 다시 생성합니다. AI 연결 자체가 안 되어 있으면 관리자에게 서비스 설정의 AI 항목을 확인해 달라고 하세요.</dd></div>
            <div><dt>글자가 넘쳐요</dt><dd>측정 배지의 "결함"을 누르면 그 슬라이드로 갑니다. 요점을 줄이거나,
              담을 수 있는 레이아웃으로 바꾸거나, AI로 다시 쓰기를 쓰세요.</dd></div>
            <div><dt>내 템플릿을 올렸는데 어색해요</dt><dd>레이아웃이 적은 템플릿은 모든 슬라이드가 같은 모양이 됩니다.
              제목만 있는 마스터보다 제목+본문, 2단, 그림 자리가 있는 마스터가 있는 파일이 훨씬 잘 맞습니다.</dd></div>
            <div><dt>이미지가 안 보여요</dt><dd>이미지를 볼 수 없다는 표시가 나오면 그 이미지가 저장소에서 사라진 것입니다
              (저장 위치를 옮겼거나 복원이 이미지를 빠뜨린 경우). 다시 올리면 같은 이름으로 교체됩니다.</dd></div>
            <div><dt>로그인이 자꾸 풀려요</dt><dd>세션 기간은 관리자가 설정합니다. 여러 창을 띄워 두면 한 곳에서
              로그아웃한 것이 전부에 적용됩니다.</dd></div>
          </dl>
        </section>
      </article>
    </div>
  </AppShell>
}
