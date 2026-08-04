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

### Slide kinds

`@cover` `@section` `@content` `@two` `@comparison` `@quote` `@picture`
`@table` `@chart` `@closing` `@blank`, and the Korean equivalents 표지, 간지,
본문, 비교, 인용, 마무리. A kind chooses among the template's layouts by role; to
name one exactly, write `@layout <id>` with an id from
`GET /api/v1/presentations/{id}/source`.

Without a kind, a slide takes the role its position implies: the first is a
cover, the last of three or more is a closing, everything between is content.

### Components

`kpi` `hero` `steps` `timeline` `comparison` `columns` `bars` `line` `share`
`meter` `table` `quote` `callout`, with aliases in both languages (`지표`,
`단계`, `로드맵`, `비교`, `추이`, `비중`, `달성률`, `강조`, …).

Rows are `label | value | detail`; any part may be omitted. A value that carries
a number — `42개`, `18%`, `1,200억`, `-3.5pt` — is read as one, so bars and
meters draw to scale. Components are drawn as native PowerPoint shapes in the
template's own colours, not as images.

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
as a warning, because a deck someone is waiting for should arrive.

## API

| Request | Effect |
| --- | --- |
| `GET /api/v1/presentations/{id}/source` | the deck as text, plus its layout and component vocabulary |
| `PUT /api/v1/presentations/{id}/source` | compile and replace the deck's slides |
| `PUT …/source` with `{"dryRun": true}` | compile and report without changing anything |

`GET` regenerates the text from the stored slides when they have been edited on
the canvas since the source was written, so the two never disagree.

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
