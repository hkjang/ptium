# Deck source

A Ptium deck is written as text and compiled into a template. The text is the
deck: opening a deck shows its source, applying edited source redraws the slides,
and the round trip is exact.

Why a language and not a form: the interesting part of a deck is its structure —
what each slide argues, and in what form — and structure is far easier to review,
diff and correct as text than as a tree of boxes. It is also what a language model
produces most reliably.

## The whole language

```
# 클라우드 전환 로드맵          a new slide, and its title
@cover                        what kind of slide this is
> 2026년 하반기 · 임원 보고     the lead line under the title
- 첫 번째 요점                 a bullet
  - 딸린 근거                  a sub-bullet, two spaces per level
::kpi 핵심 지표                a component, with an optional caption
- 전환 대상 | 42개              label | value
- 예상 절감 | 18%
::                            the component ends
!notes 예산 질문이 나오면…      speaker notes, continuing over following lines
// a comment
```

That is all of it. A line that starts with none of those marks is prose: it
becomes the lead if the slide has none yet, and a bullet otherwise.

One escape exists. Text that would be misread where it sits is written with a
leading backslash — a title that itself begins with `#`, or a component field
containing a `|`. Nothing else needs escaping, because every line already carries
its own marker: `- - dash` is a bullet whose text is "- dash".

### Slide kinds

`@cover` `@section` `@content` `@two` `@comparison` `@quote` `@picture`
`@table` `@chart` `@closing` `@blank`, and the Korean equivalents 표지, 간지,
본문, 비교, 인용, 마무리. A kind chooses among the template's layouts by role; to
name one exactly, write `@layout <id>` with an id from
`GET /api/v1/presentations/{id}/source`. A layout may be named by its id, its
name, or the slug of either, and `@layout id=제목-및-내용` is read the same way —
a model writes the assignment form often enough that reading it literally would
move slides to the wrong layout.

Without a kind, a slide takes the role its position implies: the first is a
cover, the last of three or more is a closing, everything between is content.

### Components

`kpi` `hero` `steps` `timeline` `comparison` `columns` `bars` `line` `share`
`meter` `table` `quote` `callout` `grid`, with aliases in both languages (`지표`,
`단계`, `로드맵`, `비교`, `추이`, `비중`, `달성률`, `강조`, …).

Rows are `label | value | detail`; any part may be omitted. A value that carries
a number — `42개`, `18%`, `1,200억`, `-3.5pt` — is read as one, so bars and
meters draw to scale. Components are drawn as native PowerPoint shapes in the
template's own colours, not as images.

A table takes as many columns as its author writes, and its first row is the
header:

```
::table 연간 비용 (억원)
- 항목 | 2026 | 2027 | 2028
- 인건비 | 4.2 | 3.4 | 3.1
- 라이선스 | 1.1 | 1.4 | 1.4
::
```

A line chart takes one row per series, and a row whose values are labels rather
than bare numbers is the time axis:

```
::line 월별 처리량
- 월 | 1월, 2월, 3월, 4월
- 전환 전 | 120, 118, 121, 119
- 전환 후 | 120, 132, 148, 165
::
```

When a component fills the only body region, the slide's lead line is drawn as the
component's heading rather than being dropped.

A component that reads across the page takes the page: a comparison matrix, a
table, a grid or a chart placed in one column of a two-column layout is drawn
across both, so the other half is not left empty.

A chart whose values are not numbers — `Q3 | 1시간` — cannot be plotted, so it is
drawn as labelled figures (or a timeline, past four rows) instead of being turned
back into prose. The compiler says so in its warnings.

### Comparisons

`::comparison` covers two shapes, and which one is drawn follows the rows.

Two or three rows of `name | headline | supporting point` are the alternatives
being compared, drawn as cards. Rows of `attribute | side | side`, or a first row
that names the columns, are an attribute matrix, drawn as a table with each side
in its own accented column:

```
::comparison
- 항목 | 기존 방식 | 신규 방식
- 아키텍처 | 모놀리식 단일 구조 | 마이크로서비스 분산 구조
- 확장성 | 수직 확장으로 비용 폭증 | 수평 확장으로 유연한 대응
::
```

A header row is recognised by a generic first cell (항목, 구분, item, …) or by
cells that name sides rather than hold values (현재 · 목표, 기존 · 신규, before ·
after). Two columns under such a header are two sides with no attribute column.

### Rows written as a table

Asked for a table, a model often writes the rows as ordinary bullets, or as
markdown. Both are read as the component they plainly are: a run of two or more
bullets with the same number of `|`-separated fields becomes a comparison, a
figure row or a table, and a markdown row's surrounding pipes and its `|---|`
rule are punctuation rather than empty columns.

```
| 항목 | 결과 |
|---|---|
| 응답 시간 | 240ms |
| 오류율 | 0.2% |
```

### Grids

`::grid <definition> [caption]` draws a grid an organisation defined for itself:
a RACI chart, a risk matrix, a readiness checklist. The first row is the header,
the rest are data, and a cell whose value the definition knows is drawn as a
coloured chip.

```
::grid raci 전환 프로젝트
- 활동 | 데이터본부 | 개발팀 | 운영팀
- 요건 정의 | A | R | C
- 이관 실행 | I | R | A
::
```

Three definitions ship: `raci`, `matrix` and `checklist`. The editor's **격자** panel
edits them as a form — columns, values, colour roles — and writes a worked example
into the code for you. A definition of your own under the same name replaces the
shipped one, so the source above keeps working while the slide follows your house
rules. Over the API:

```bash
curl -X PUT -H 'Authorization: Bearer <key>' -H 'Content-Type: application/json' \
  -d '{"title":"KCB 담당 체계","zebra":true,"legend":true,
       "order":["R","A","C","I"],
       "columns":[{"label":"업무","weight":2.4,"align":"l"}],
       "values":{"R":{"label":"실행","role":"accent1","chip":true,"meaning":"직접 수행"},
                 "A":{"label":"승인","role":"negative","chip":true,"meaning":"최종 책임"}}}' \
  http://localhost:8080/api/v1/grids/raci
```

A definition never names a colour — it names a role: `accent1`…`accent6`,
`positive`, `negative`, `muted`, `ink`. Each template resolves those through its
own theme, so one definition comes out in every house's colours, and the colours
it gets are the ones that passed the palette check. `weight` is a column's share
of the width, `order` is the legend's reading order, and `zebra` shades alternate
rows.

| Request | Effect |
| --- | --- |
| `GET /api/v1/grids` | your definitions, plus the shipped ones you have not replaced |
| `POST /api/v1/grids` · `PUT /api/v1/grids/{name}` | save a definition |
| `DELETE /api/v1/grids/{name}` | remove yours; a shipped definition returns |

### Images

`::image <name or id> | <caption>` places an image that was uploaded first. It
goes into the layout's own picture region when it has one, and otherwise into the
largest free body region, centre-cropped to the frame rather than stretched — a
tight crop reads better than a squashed logo.

In the workspace, the editor's **이미지** panel uploads by drag and drop and
writes the directive into the code for you. Over the API:

```bash
curl -H 'Authorization: Bearer <key>' \
     -F 'file=@logo.png' -F 'name=로고' \
     http://localhost:8080/api/v1/assets
```

PNG, JPEG, GIF and SVG, up to 16 MiB each, named per account: a second upload
under the same name replaces it, so a logo is changed in one place. The deck holds
a reference rather than a copy, which is why the same logo on twenty slides is
stored once. A name nobody uploaded is reported as a warning with its line, not
silently skipped.

| Request | Effect |
| --- | --- |
| `POST /api/v1/assets` | upload an image (multipart `file`, optional `name`) |
| `GET /api/v1/assets` | list your images |
| `GET /api/v1/assets/{id}` | the image's bytes |
| `DELETE /api/v1/assets/{id}` | remove it |

## Compiling

Compiling binds source to the template that deck uses:

- Each slide takes a real layout, and text is written into that layout's real
  slots, trimmed to what each slot holds.
- A component claims a body region; a slot never holds both a component and prose.
- Bullets are split across the columns of a two-content layout, keeping each
  sub-bullet with its parent.
- A layout with no writable region of its own gets one derived from the space its
  artwork leaves free. See `docs/architecture.md`.

Nothing is rejected for being imperfect. A layout that does not exist, a
component with no room, text with nowhere to go — each is adjusted and reported
as a warning that names the line it came from, because a deck someone is waiting
for should arrive. Text that had to be shortened or left out is reported the same
way: a slide that quietly says less than its author wrote is worse than one that
says so.

## Inspection

Compiling also measures the slides it produced, and every compile response
carries `findings` beside its warnings. A warning says what compiling changed; a
finding says what still looks wrong once the slide is drawn:

A finding is either a defect — the slide is drawn wrong — or advisory: the slide is
drawn correctly and could still be better. The two are kept apart because
conflating them trains people to ignore both, and because nothing advisory
justifies rewriting an author's words to satisfy a measurement.

| Kind | | Meaning |
| --- | --- | --- |
| `overflow` | defect | text that must shrink past readability, does not fit at all, or is drawn taller than the room a component reserved for it |
| `outside` | defect | something drawn past the edge of its region or of the slide |
| `collision` | defect | two regions on top of each other, two lines of one component's own text overlapping, or text over the template's own picture or lettering |
| `contrast` | defect | composed text below 4.5:1 against what sits behind it |
| `orphan` | advisory | a heading whose wrap leaves one stray word or syllable on its last line |
| `density` | advisory | more than six points on a slide, or a region filled to its last line |
| `notes` | advisory | a slide that argues something with nothing written down to say |

`GET …/inspect` reports `defects` and `advisories` separately, and `clean` refers
to the defects: a deck can be drawn perfectly and still be unfinished.

A component is measured by what it draws, not by the boxes it asked for: a line of
text is as tall as the lines it wraps into. Measuring the box is how a slide whose
heading was drawn on top of its own first row once passed inspection.

The same measurements run over every shipped design in the test suite, against a
deck that uses a cover, prose, each component, a table, a chart and a grid. That
is what replaced opening a rendered file and looking at it.

The first slide is a cover only by convention. One that carries a component or a
list of points is compiled as content, whatever its position.

## Where the source comes from

Both writers produce this language, so a connected deployment and an air-gapped
one differ in how good the prose is, not in how the deck is built:

- An AI provider is asked for the slide language directly rather than for nested
  JSON. There is one construct per line, nothing to balance, and a mistake costs
  one line instead of the whole response — and what the model wrote is what the
  author reads and corrects. A provider that answers with the older JSON shape is
  still accepted.
- Without a provider, the deterministic writer reads the prompt for its subjects,
  timeframe and figures and writes the same language.

### Talking to a self-hosted model

A reasoning model behind an OpenAI-compatible endpoint answers with its thinking
and an empty message, and it thinks for longer than any sane request timeout. So
the first request asks it not to think (`chat_template_kwargs.enable_thinking`),
and a provider that rejects that field is retried without it. Three settings
control the rest:

| Setting | Default | Effect |
| --- | --- | --- |
| `ai.reasoning` | `auto` | `auto` asks for no thinking and falls back if refused; `off` always asks; `on` leaves the model's default alone |
| `ai.max_output_tokens` | 8000 | a deck's source runs to thousands of tokens; without a bound a reasoning model spends the context on thinking |
| `ai.timeout_seconds` | 300 | a self-hosted 100B-class model writes ten slides in 30–40 seconds, and much slower under load |

They are on the admin **AI 모델** page beside the model connection. A stored API key
the server can no longer decrypt — the encryption key or the database URL changed —
is reported on that page as needing re-entry instead of failing the whole page.

## API

| Request | Effect |
| --- | --- |
| `GET /api/v1/presentations/{id}/source` | the deck as text, plus its layout and component vocabulary |
| `PUT /api/v1/presentations/{id}/source` | compile and replace the deck's slides |
| `PUT …/source` with `{"dryRun": true}` | compile and report without changing anything |
| `POST …/source/preview.svg?slide=N` | render one slide of unsaved source, changing nothing |
| `GET /api/v1/presentations/{id}/inspect` | measure the stored deck as it will be drawn |

`GET` regenerates the text from the stored slides when they have been edited on
the canvas since the source was written, so the two never disagree.

`preview.svg` is what the editor calls while someone types: it compiles the text
and draws the slide through the real template without storing anything, and
reports the compiled slide count in `X-Ptium-Slide-Count`.

## Example

```
# 결제 시스템 전환
@cover
> 2026년 하반기 · 임원 보고
!notes 결론부터: 3분기 착수를 승인해 주십시오.

# 전환 대상과 우선순위
> 42개 시스템을 세 묶음으로 나눴습니다.
::kpi 규모
- 전환 대상 | 42개
- 1차 범위 | 12개
- 예상 절감 | 18%
::

# 이행 순서
::steps 3단계
- 준비 | 조직·예산 확정 (7월)
- 이행 | 1차 12개 이관 (8~10월)
- 안정화 | 운영 이관과 점검 (11월)
::

# 투자 타당성
> 회수 시점은 14개월입니다.
::comparison
- 현행 유지 | 연 4.2억 · 장애 리스크 누적
- 전환 후 | 연 3.4억 · 확장 비용 선형
::
!notes 가정은 인건비 동결과 트래픽 20% 증가입니다.
```
