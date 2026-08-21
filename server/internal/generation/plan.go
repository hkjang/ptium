package generation

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/model"
)

// deckPlanCopy is the wording layer: it turns the topics a prompt named into
// slides, in the language the deck is written in.
//
// Without an AI provider Ptium cannot invent facts, and it does not pretend to.
// What it can do is build the argument a professional deck would make about
// exactly these topics — the frame, the order, the questions each slide has to
// answer — so the author fills in numbers rather than deciding structure. That is
// honest scaffolding, and it is why the prompt is visible in the result.
type deckPlanCopy struct {
	Title     string
	Language  string
	CoverLead string
	// AgendaTitle and AgendaNotes head the contents page a long deck opens with.
	AgendaTitle   string
	AgendaNotes   string
	CoverNotes    string
	ClosingTitle  string
	ClosingLead   string
	ClosingPoints []string
	ClosingNotes  string
	// Section plans one slide about a topic. part is which slide of share.
	Section func(topic promptTopic, part, share int) sectionPlan
	// Followup plans a slide for a deck longer than its list of topics.
	Followup func(index int) sectionPlan
}

// frameTitleSuffix names what a slide covers when a topic spans several slides.
// "도입 방안 (2/2)" tells a reader nothing; "도입 방안 — 기대 효과" tells them what
// is on it.
var frameTitleSuffix = map[string]map[string]string{
	"ko": {frameSituation: "현황", frameSequence: "이행 순서", frameCase: "비용과 효과",
		frameOptions: "선택지 비교", frameRisk: "리스크와 대응", frameOutcome: "기대 효과"},
	"en": {frameSituation: "where it stands", frameSequence: "sequence", frameCase: "cost and return",
		frameOptions: "options", frameRisk: "risk and response", frameOutcome: "expected outcome"},
	"ja": {frameSituation: "現状", frameSequence: "実行順序", frameCase: "費用と効果",
		frameOptions: "選択肢の比較", frameRisk: "リスクと対応", frameOutcome: "期待効果"},
	"zh": {frameSituation: "现状", frameSequence: "执行顺序", frameCase: "成本与收益",
		frameOptions: "方案比较", frameRisk: "风险与应对", frameOutcome: "预期效果"},
}

// deckMonth writes the month a deck was made, in its own language. A zero time
// is a deck that has not been stored yet, and dating that would be inventing a
// fact.
func deckMonth(at time.Time, language string) string {
	if at.IsZero() {
		return ""
	}
	switch language {
	case "ko":
		return fmt.Sprintf("%d년 %d월", at.Year(), int(at.Month()))
	case "ja":
		return fmt.Sprintf("%d年%d月", at.Year(), int(at.Month()))
	case "zh":
		return fmt.Sprintf("%d年%d月", at.Year(), int(at.Month()))
	}
	return at.Format("January 2006")
}

// coverLine assembles the line under a deck's title: when, who is presenting,
// and who it is for — in that order, and only the parts that exist.
func coverLine(period, presenter, audience string) string {
	parts := make([]string, 0, 3)
	for _, part := range []string{period, presenter, audience} {
		if strings.TrimSpace(part) != "" {
			parts = append(parts, strings.TrimSpace(part))
		}
	}
	return strings.Join(parts, " · ")
}

// partTitle titles one slide of a topic that spans several.
//
// A long subject plus a suffix wraps and leaves a stray ending, and by the second
// slide about a subject the audience knows what it is — so a long name gives way
// to the aspect alone.
func partTitle(language, name, frame string, part, share int) string {
	if share <= 1 || part == 0 {
		return name
	}
	suffix, ok := frameTitleSuffix[language][frame]
	if !ok || suffix == "" {
		return fmt.Sprintf("%s (%d/%d)", name, part+1, share)
	}
	// Latin script needs more room for the same amount of meaning: twenty-four
	// characters is a full Korean title and half an English one.
	limit := 24
	if latinPhrase(name) {
		limit = 56
	}
	// The name gives way, not the aspect — dropping it entirely is what made two
	// different subjects both arrive at a slide titled "expected outcome". It
	// gives way by the same amount on every slide of the topic, measured against
	// the longest aspect, so a subject is not called three different things in
	// three consecutive titles.
	display := name
	if room := limit - longestFrameSuffix(language) - 3; utf8.RuneCountInString(name) > room {
		if short := phraseWithin(name, room); short != "" {
			display = short
		}
	}
	combined := display + " — " + suffix
	if utf8.RuneCountInString(combined) <= limit {
		return combined
	}
	return suffix
}

// longestFrameSuffix is how much room the aspects need in a language.
func longestFrameSuffix(language string) int {
	longest := 0
	for _, suffix := range frameTitleSuffix[language] {
		longest = max(longest, utf8.RuneCountInString(suffix))
	}
	return longest
}

// sectionPlan is one slide, before it becomes source.
type sectionPlan struct {
	Title        string
	Role         string
	Lead         string
	Points       []string
	Block        string
	BlockCaption string
	Items        []string
	Notes        string
}

// newDeckPlan builds the wording for a deck from its outline.
func newDeckPlan(outline promptOutline, presentation model.Presentation, phrases languageCopy, audience, presenter string) deckPlanCopy {
	joiner := ", "
	switch languageOf(presentation.Language) {
	case "ko", "ja", "zh":
		// A comma inside a Japanese or Chinese title reads as punctuation of the
		// sentence it was lifted from; the interpunct reads as a join.
		joiner = " · "
	}
	title := outline.deckTitle(presentation.Title, presentation.Prompt, joiner)
	// A cover carries when as well as who. The brief usually says ("2026년 하반기"),
	// and when it does not, the deck's own date does — real covers are dated, and
	// a line reading only "경영진 보고" looks like a draft.
	if strings.TrimSpace(outline.Period) == "" {
		outline.Period = deckMonth(presentation.CreatedAt, languageOf(presentation.Language))
	}

	switch languageOf(presentation.Language) {
	case "ko":
		return koreanPlan(outline, title, audience, presenter, phrases)
	case "ja":
		return japanesePlan(outline, title, audience, presenter)
	case "zh":
		return chinesePlan(outline, title, audience, presenter)
	}
	return englishPlan(outline, title, audience, presenter, phrases)
}

func languageOf(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return "ko"
	}
	if index := strings.IndexAny(language, "-_"); index > 0 {
		return language[:index]
	}
	return language
}

func koreanPlan(outline promptOutline, title, audience, presenter string, phrases languageCopy) deckPlanCopy {
	// The subject as a phrase, not as the brief: sentences are built from it, and
	// "…을 실무진에게 하는 자료에 대한 결정" is the request read back aloud.
	subject := outline.subjectPhrase()
	cover := coverLine(outline.Period, presenter, audience+" 보고")
	plan := deckPlanCopy{
		Title:        title,
		Language:     "ko",
		CoverLead:    cover,
		AgendaTitle:  "목차",
		AgendaNotes:  "오늘 다룰 순서를 한 번 훑고, 결론이 어디에 있는지 미리 말합니다.",
		CoverNotes:   fmt.Sprintf("%s 왜 지금 다뤄야 하는지 한 문장으로 밝히고, 오늘 결론이 무엇인지 먼저 말합니다.", josa(subject, "을", "를")),
		ClosingTitle: "다음 단계",
		ClosingLead:  fmt.Sprintf("%s 대한 결정과 실행을 분리해 요청합니다.", josa(subject, "에", "에")),
		ClosingPoints: []string{
			"오늘 요청하는 결정 한 가지",
			"결정 후 30일 안에 진행할 일",
			"다음 보고 시점과 확인할 지표",
		},
		ClosingNotes: fmt.Sprintf("%s 무엇을 승인해야 하는지 분명히 남기고 마칩니다.", josa(audience, "이", "가")),
	}
	plan.Section = func(topic promptTopic, part, share int) sectionPlan {
		name := topic.Name
		section := sectionPlan{Title: partTitle("ko", headingName(name), topic.Frame, part, share), Role: "content"}
		switch topic.Frame {
		case frameSequence:
			section.Lead = fmt.Sprintf("%s 순서대로 나눠 봅니다.", josa(name, "을", "를"))
			section.Block = "steps"
			section.Items = []string{
				"준비 | 범위 · 조직 · 예산을 확정",
				fmt.Sprintf("이행 | %s 단계별로 적용", josa(name, "을", "를")),
				"안정화 | 운영 이관과 점검 기준 확정",
			}
			section.Notes = "각 단계의 완료 조건을 한 문장으로 말하고, 병행할 수 없는 이유를 덧붙입니다."
		case frameCase:
			section.Lead = fmt.Sprintf("%s 비용과 효과를 같은 기준으로 비교합니다.", josa(name, "의", "의"))
			section.Points = []string{
				"투입: 인력 · 라이선스 · 이관 비용",
				"회수: 절감액과 회수 시점",
				"비교: 지금 하지 않을 때의 누적 비용",
			}
			section.Notes = "숫자의 출처와 가정을 먼저 말합니다. 가정이 흔들리면 결론도 흔들린다는 점을 분명히 합니다."
		case frameOptions:
			section.Lead = "선택지를 같은 축에서 견줍니다."
			section.Block = "comparison"
			section.Items = []string{
				"현행 유지 | 추가 비용 없음 · 문제는 누적",
				fmt.Sprintf("%s | 초기 투자 필요 · 구조 개선", name),
			}
			section.Notes = "권고안을 먼저 말하고 그 이유를 두 가지로 좁혀 설명합니다."
		case frameRisk:
			section.Lead = "가장 큰 위험부터 다룹니다."
			section.Points = []string{
				"발생 가능성이 가장 높은 위험과 그 조건",
				"완화 방안과 담당 · 기한",
				"감수하기로 한 잔여 위험과 그 근거",
			}
			section.Notes = "위험을 숨기지 않는 편이 신뢰를 얻습니다. 대응이 준비돼 있다는 점을 함께 말합니다."
		case frameOutcome:
			section.Lead = "무엇이 어떻게 달라지는지 지표로 말합니다."
			if outline.hasFigures() {
				section.Block = "kpi"
				section.BlockCaption = "핵심 지표"
				for _, figure := range outline.Figures {
					label := figure.Label
					if label == "" {
						label = name
					}
					section.Items = append(section.Items, fmt.Sprintf("%s | %s", capitalized(label), figure.Value))
				}
			} else {
				section.Points = []string{
					"6개월 안에 확인할 지표와 목표",
					"12개월 목표와 그때의 판단 기준",
					"측정 방법과 보고 주기",
				}
			}
			section.Notes = "지표는 세 개를 넘기지 않습니다. 측정 방법이 없는 목표는 목표가 아닙니다."
		default:
			section.Lead = fmt.Sprintf("%s 지금 어디까지 와 있는지 정리합니다.", josa(name, "이", "가"))
			section.Points = []string{
				"현재 상태와 이 논의가 다루는 범위",
				"확인된 문제와 그렇게 된 원인",
				fmt.Sprintf("%s 지금 결정이 필요한 이유", josa(name, "에", "에")),
			}
			section.Notes = fmt.Sprintf("%s 체감하는 문제부터 말하고, 원인을 한 단계 더 파고듭니다.", josa(audience, "이", "가"))
		}
		return section
	}
	plan.Followup = func(index int) sectionPlan {
		followups := []sectionPlan{
			{Title: "결정이 필요한 사항", Lead: "이 자리에서 정해야 하는 것만 남겼습니다.",
				Points: []string{"결정 사항과 선택지", "결정하지 않을 때의 비용", "결정 후 즉시 착수할 일"},
				Notes:  "결정권자에게 선택지를 두 개 이하로 제시합니다."},
			{Title: "실행 준비 상태", Lead: "시작할 수 있는 조건이 갖춰졌는지 확인합니다.",
				Points: []string{"확보된 자원과 부족한 자원", "선행 조건과 해소 계획", "첫 2주에 할 일"},
				Notes:  "준비되지 않은 항목을 먼저 말하는 편이 안전합니다."},
			{Title: "남은 질문", Lead: "아직 답하지 못한 것을 공개합니다.",
				Points: []string{"확인이 필요한 가정", "추가로 필요한 데이터", "다음 보고에서 답할 항목"},
				Notes:  "모르는 것을 남겨 두면 신뢰가 올라갑니다."},
		}
		return followups[max(index, 0)%len(followups)]
	}
	return plan
}

func englishPlan(outline promptOutline, title, audience, presenter string, phrases languageCopy) deckPlanCopy {
	subject := outline.subjectPhrase()
	cover := coverLine(outline.Period, presenter, "Prepared for "+audience)
	plan := deckPlanCopy{
		Title:        title,
		CoverLead:    cover,
		CoverNotes:   fmt.Sprintf("Say why %s matters now, and give the conclusion before the detail.", subject),
		Language:     "en",
		AgendaTitle:  "Agenda",
		AgendaNotes:  "Walk the order once, and say up front where the decision sits.",
		ClosingTitle: "Next steps",
		ClosingLead:  "Separate the decision from the work that follows it.",
		ClosingPoints: []string{
			"The one decision being asked for today",
			"What starts within 30 days of that decision",
			"When this is reviewed, and against which measure",
		},
		ClosingNotes: "Leave the room clear on what was approved.",
	}
	plan.Section = func(topic promptTopic, part, share int) sectionPlan {
		name := topic.Name
		section := sectionPlan{Title: capitalized(partTitle("en", headingName(name), topic.Frame, part, share)), Role: "content"}
		switch topic.Frame {
		case frameSequence:
			section.Lead = "The order this happens in."
			section.Block = "steps"
			section.Items = []string{
				"Prepare | Scope, owners and budget agreed",
				fmt.Sprintf("Execute | %s, stage by stage", name),
				"Stabilise | Handover to operations, with exit criteria",
			}
			section.Notes = "State the completion condition for each stage, and why they cannot run at once."
		case frameCase:
			section.Lead = "Cost and return on the same basis."
			section.Points = []string{
				"Investment: people, licensing, migration",
				"Return: savings, and when they land",
				"Comparison: the cost of doing nothing",
			}
			section.Notes = "Give the source and the assumptions first; the conclusion rests on them."
		case frameOptions:
			section.Lead = "The options, judged on the same axes."
			section.Block = "comparison"
			section.Items = []string{
				"Stay as is | No new spend · the problem compounds",
				fmt.Sprintf("%s | Upfront investment · structural fix", name),
			}
			section.Notes = "Lead with the recommendation, then two reasons for it."
		case frameRisk:
			section.Lead = "The largest risk first."
			section.Points = []string{
				"The most likely failure, and what triggers it",
				"Mitigation, with an owner and a date",
				"Residual risk being accepted, and why",
			}
			section.Notes = "Naming risk earns trust, provided the response is ready."
		case frameOutcome:
			section.Lead = "What changes, measured."
			if outline.hasFigures() {
				section.Block = "kpi"
				section.BlockCaption = "Key figures"
				for _, figure := range outline.Figures {
					label := figure.Label
					if label == "" {
						label = name
					}
					section.Items = append(section.Items, fmt.Sprintf("%s | %s", capitalized(label), figure.Value))
				}
			} else {
				section.Points = []string{
					"The measure to check at six months, and its target",
					"The twelve-month target, and how it will be judged",
					"How it is measured, and how often it is reported",
				}
			}
			section.Notes = "Three measures at most. A target without a method is not a target."
		default:
			section.Lead = fmt.Sprintf("Where %s stands today.", name)
			section.Points = []string{
				"Current state, and the scope of this discussion",
				"The problem observed, and what causes it",
				fmt.Sprintf("Why %s needs a decision now", name),
			}
			section.Notes = fmt.Sprintf("Open with the problem %s feels, then go one level into the cause.", audience)
		}
		return section
	}
	plan.Followup = func(index int) sectionPlan {
		followups := []sectionPlan{
			{Title: "Decisions required", Lead: "Only what has to be settled here.",
				Points: []string{"The decision, and the options", "The cost of not deciding", "What begins immediately after"},
				Notes:  "Offer no more than two options to a decision maker."},
			{Title: "Readiness", Lead: "Whether the conditions to start are in place.",
				Points: []string{"Resources secured, and missing", "Dependencies, and how they clear", "The first two weeks"},
				Notes:  "Say what is not ready before someone asks."},
			{Title: "Open questions", Lead: "What is not answered yet.",
				Points: []string{"Assumptions still to verify", "Data still needed", "What the next review will answer"},
				Notes:  "Admitting the gaps raises confidence in the rest."},
		}
		return followups[max(index, 0)%len(followups)]
	}
	return plan
}

// planWords is a language's wording for the same argument. Adding a language is
// filling this in; the structure of the deck does not change with it.
type planWords struct {
	coverLead     func(period, audience string) string
	coverNotes    func(subject string) string
	agendaTitle   string
	agendaNotes   string
	closingTitle  string
	closingLead   string
	closingPoints []string
	closingNotes  string
	language      string
	frames        map[string]func(name, audience string) sectionPlan
	figureCaption string
	followups     []sectionPlan
}

func buildPlan(outline promptOutline, title, audience string, words planWords) deckPlanCopy {
	plan := deckPlanCopy{
		Title:         title,
		CoverLead:     words.coverLead(outline.Period, audience),
		CoverNotes:    words.coverNotes(outline.subjectPhrase()),
		Language:      words.language,
		AgendaTitle:   words.agendaTitle,
		AgendaNotes:   words.agendaNotes,
		ClosingTitle:  words.closingTitle,
		ClosingLead:   words.closingLead,
		ClosingPoints: words.closingPoints,
		ClosingNotes:  words.closingNotes,
	}
	plan.Section = func(topic promptTopic, part, share int) sectionPlan {
		build, ok := words.frames[topic.Frame]
		if !ok {
			build = words.frames[frameSituation]
		}
		section := build(topic.Name, audience)
		section.Role = "content"
		section.Title = partTitle(words.language, headingName(topic.Name), topic.Frame, part, share)
		// Figures the prompt supplied belong on the outcome slide as figures.
		if topic.Frame == frameOutcome && outline.hasFigures() {
			section.Block, section.BlockCaption, section.Points, section.Items = "kpi", words.figureCaption, nil, nil
			for _, figure := range outline.Figures {
				label := figure.Label
				if label == "" {
					label = topic.Name
				}
				section.Items = append(section.Items, fmt.Sprintf("%s | %s", label, figure.Value))
			}
		}
		return section
	}
	plan.Followup = func(index int) sectionPlan {
		return words.followups[max(index, 0)%len(words.followups)]
	}
	return plan
}

func japanesePlan(outline promptOutline, title, audience, presenter string) deckPlanCopy {
	return buildPlan(outline, title, audience, planWords{
		coverLead: func(period, audience string) string {
			return coverLine(period, presenter, audience+"向け報告")
		},
		coverNotes: func(subject string) string {
			return subject + "を今扱う理由を一文で述べ、結論を先に示します。"
		},
		closingTitle: "次のステップ",
		closingLead:  "決めることと、その後の実行を分けて依頼します。",
		closingPoints: []string{
			"本日お願いする決定は一つ",
			"決定から30日以内に着手すること",
			"次回報告の時期と確認する指標",
		},
		closingNotes:  "何を承認したのかを明確に残して終えます。",
		language:      "ja",
		figureCaption: "主要指標",
		frames: map[string]func(name, audience string) sectionPlan{
			frameSituation: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: name + "の現在地を整理します。",
					Points: []string{"現状と、この議論が扱う範囲", "確認された問題と、その原因", name + "に今決定が必要な理由"},
					Notes:  audience + "が感じている問題から入り、原因を一段深く掘ります。"}
			},
			frameSequence: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: name + "を順序に分けて示します。", Block: "steps",
					Items: []string{"準備 | 範囲・体制・予算の確定", "実行 | " + name + "を段階的に適用", "安定化 | 運用移管と点検基準"},
					Notes: "各段階の完了条件を一文で述べ、並行できない理由を添えます。"}
			},
			frameCase: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: name + "の費用と効果を同じ基準で比べます。",
					Points: []string{"投入: 人員・ライセンス・移行費用", "回収: 削減額と回収時期", "比較: 今やらない場合の累積費用"},
					Notes:  "数値の出典と前提を先に述べます。前提が崩れれば結論も崩れます。"}
			},
			frameOptions: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: "選択肢を同じ軸で比較します。", Block: "comparison",
					Items: []string{"現状維持 | 追加費用なし · 問題は累積", name + " | 初期投資が必要 · 構造改善"},
					Notes: "推奨案を先に述べ、その理由を二つに絞ります。"}
			},
			frameRisk: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: "最大のリスクから扱います。",
					Points: []string{"発生確率が最も高いリスクとその条件", "緩和策と担当・期限", "受容する残存リスクとその根拠"},
					Notes:  "リスクを隠さない方が信頼を得ます。対応が用意されていることも述べます。"}
			},
			frameOutcome: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: "何がどう変わるかを指標で述べます。",
					Points: []string{"6か月で確認する指標と目標", "12か月の目標と判断基準", "測定方法と報告の頻度"},
					Notes:  "指標は三つまで。測定方法のない目標は目標ではありません。"}
			},
		},
		followups: []sectionPlan{
			{Title: "決定が必要な事項", Lead: "この場で決めるべきことだけを残しました。",
				Points: []string{"決定事項と選択肢", "決めない場合の費用", "決定後すぐに着手すること"},
				Notes:  "決定者には選択肢を二つ以下で示します。"},
			{Title: "実行準備の状況", Lead: "着手できる条件が揃っているかを確認します。",
				Points: []string{"確保済みの資源と不足する資源", "前提条件と解消計画", "最初の2週間の作業"},
				Notes:  "準備できていない項目を先に述べる方が安全です。"},
			{Title: "残る論点", Lead: "まだ答えていないことを開示します。",
				Points: []string{"確認が必要な前提", "追加で必要なデータ", "次回報告で答える項目"},
				Notes:  "分からないことを残しておくと信頼が上がります。"},
		},
	})
}

func chinesePlan(outline promptOutline, title, audience, presenter string) deckPlanCopy {
	return buildPlan(outline, title, audience, planWords{
		coverLead: func(period, audience string) string {
			return coverLine(period, presenter, "面向"+audience+"的汇报")
		},
		coverNotes: func(subject string) string {
			return "用一句话说明为何现在讨论" + subject + "，并先给出结论。"
		},
		closingTitle: "下一步",
		closingLead:  "把决策与后续执行分开来请求。",
		closingPoints: []string{
			"今天请求的一项决策",
			"决策后30天内启动的工作",
			"下次汇报的时间与要看的指标",
		},
		closingNotes:  "明确留下需要批准的内容。",
		language:      "zh",
		figureCaption: "关键指标",
		frames: map[string]func(name, audience string) sectionPlan{
			frameSituation: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: "梳理" + name + "目前的进展。",
					Points: []string{"现状，以及本次讨论的范围", "已确认的问题及其原因", "为什么" + name + "现在需要决策"},
					Notes:  "从" + audience + "能感受到的问题讲起，再深入一层原因。"}
			},
			frameSequence: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: "按顺序拆解" + name + "。", Block: "steps",
					Items: []string{"准备 | 确定范围、组织与预算", "执行 | 分阶段落实" + name, "稳定 | 移交运维并确定检查标准"},
					Notes: "用一句话说明每个阶段的完成条件，并说明为何无法并行。"}
			},
			frameCase: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: "在同一基准上比较" + name + "的成本与收益。",
					Points: []string{"投入：人力、许可与迁移成本", "回收：节省金额与回收时点", "对比：不做的累计成本"},
					Notes:  "先说明数据来源与假设；假设动摇，结论也会动摇。"}
			},
			frameOptions: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: "在同一维度上比较各选项。", Block: "comparison",
					Items: []string{"维持现状 | 无新增投入 · 问题累积", name + " | 需前期投入 · 结构性改善"},
					Notes: "先给推荐方案，再用两条理由支撑。"}
			},
			frameRisk: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: "从最大的风险讲起。",
					Points: []string{"最可能发生的风险及触发条件", "缓解措施、责任人与时限", "接受的剩余风险及理由"},
					Notes:  "不隐瞒风险更能获得信任，同时说明应对已就绪。"}
			},
			frameOutcome: func(name, audience string) sectionPlan {
				return sectionPlan{Lead: "用指标说明会有什么改变。",
					Points: []string{"六个月内要看的指标与目标", "十二个月目标与判断标准", "测量方法与汇报周期"},
					Notes:  "指标不超过三个。没有测量方法的目标不是目标。"}
			},
		},
		followups: []sectionPlan{
			{Title: "需要决策的事项", Lead: "只保留必须在此决定的内容。",
				Points: []string{"决策事项与选项", "不决策的代价", "决策后立即启动的工作"},
				Notes:  "给决策者的选项不超过两个。"},
			{Title: "执行准备情况", Lead: "确认启动条件是否具备。",
				Points: []string{"已具备与欠缺的资源", "前置条件与解决计划", "前两周的工作"},
				Notes:  "先讲尚未就绪的部分更稳妥。"},
			{Title: "尚未解决的问题", Lead: "公开还没有答案的部分。",
				Points: []string{"需要验证的假设", "还需要的数据", "下次汇报将回答的内容"},
				Notes:  "承认未知会提升整体可信度。"},
		},
	})
}
