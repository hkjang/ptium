package generation

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// Fallback builds a complete deck without contacting any AI provider. It is
// what an air-gapped deployment runs on, so it has to produce something a
// person would actually present: a real narrative arc, layout variety and
// speaker notes — not placeholder text.
func Fallback(presentation model.Presentation, profile model.Profile, template Template) Deck {
	count := presentation.RequestedSlideCount
	if count < 1 {
		count = 8
	}
	if count > 50 {
		count = 50
	}
	phrases := localizedCopy(presentation.Language)
	topic := subjectPhrase(presentation, phrases.DefaultTopic)
	audience := strings.TrimSpace(presentation.Audience)
	if audience == "" {
		audience = phrases.DefaultAudience
	}
	tone := strings.TrimSpace(presentation.Tone)
	if tone == "" {
		tone = phrases.DefaultTone
	}
	perspective := presenterPerspective(profile, phrases)
	manifest := template.Manifest

	roles := fallbackRoles(count, manifest)
	result := Deck{}
	outline := make([]map[string]any, 0, count)
	sectionIndex := 0
	for index, role := range roles {
		layout, ok := manifest.LayoutForRole(role)
		if !ok {
			layout = manifest.Layouts[0]
		}
		content := deck.Content{Type: deck.ContentType, LayoutID: layout.ID, Accent: profileAccent(profile, index)}

		var title, subtitle, notes string
		var bullets []pptx.Paragraph
		switch role {
		case pptx.RoleTitle:
			title = strings.TrimSpace(presentation.Title)
			if title == "" {
				title = topic
			}
			subtitle = fmt.Sprintf(phrases.CoverSubtitle, audience, tone)
			notes = phrases.CoverNotes(topic, audience)
		case pptx.RoleClosing:
			title = phrases.ClosingTitle
			subtitle = phrases.ClosingSubtitle(audience)
			bullets = paragraphs(phrases.ClosingPoints(topic, perspective))
			notes = fmt.Sprintf(phrases.ClosingNotes, audience)
		case pptx.RoleSection:
			section := phrases.Sections[sectionIndex%len(phrases.Sections)]
			sectionIndex++
			title = section.Title
			bullets = paragraphs([]string{fmt.Sprintf(section.Lead, topic)})
			notes = fmt.Sprintf(phrases.SectionNotes, section.Title)
		case pptx.RoleQuote:
			title = fmt.Sprintf(phrases.QuoteText, topic)
			bullets = paragraphs([]string{perspective})
			notes = phrases.QuoteNotes
		default:
			section := phrases.Sections[sectionIndex%len(phrases.Sections)]
			sectionIndex++
			title = section.Title
			bullets = paragraphs(section.Points(topic, audience, tone, perspective))
			notes = fmt.Sprintf(phrases.SlideNotes, section.Title, audience)
		}

		fillSlots(&content, layout, title, subtitle, bullets)
		result.Slides = append(result.Slides, model.Slide{
			Position:     index + 1,
			Title:        truncate(title, 200),
			Subtitle:     truncate(subtitle, 300),
			Content:      content.Encode(),
			SpeakerNotes: truncate(notes, 4000),
			Layout:       layout.Role,
			LayoutID:     layout.ID,
		})
		outline = append(outline, map[string]any{"position": index + 1, "title": title, "layout": layout.Name, "role": layout.Role})
	}
	result.Outline, _ = json.Marshal(outline)
	return result
}

// fillSlots writes the generated copy into whatever slots the chosen layout
// actually has, splitting bullets across columns for multi-content layouts.
func fillSlots(content *deck.Content, layout pptx.Layout, title, subtitle string, bullets []pptx.Paragraph) {
	if placeholder, ok := layout.Slot(pptx.SlotTitle); ok && strings.TrimSpace(title) != "" {
		content.SetField(pptx.SlotTitle, fitParagraphs([]pptx.Paragraph{{Text: title}}, placeholder))
	}
	if placeholder, ok := layout.Slot(pptx.SlotSubtitle); ok && strings.TrimSpace(subtitle) != "" {
		content.SetField(pptx.SlotSubtitle, fitParagraphs([]pptx.Paragraph{{Text: subtitle}}, placeholder))
	}
	bodies := layout.BodySlots()
	if len(bodies) == 0 || len(bullets) == 0 {
		if len(bullets) > 0 && subtitle == "" {
			if placeholder, ok := layout.Slot(pptx.SlotSubtitle); ok {
				content.SetField(pptx.SlotSubtitle, fitParagraphs(bullets[:1], placeholder))
			}
		}
		return
	}
	if len(bodies) == 1 {
		content.SetField(bodies[0].Slot, fitParagraphs(bullets, bodies[0]))
		return
	}
	// Spread the points evenly so a two-column layout never looks lopsided.
	perColumn := (len(bullets) + len(bodies) - 1) / len(bodies)
	if perColumn < 1 {
		perColumn = 1
	}
	for index, placeholder := range bodies {
		start := index * perColumn
		if start >= len(bullets) {
			break
		}
		end := start + perColumn
		if end > len(bullets) {
			end = len(bullets)
		}
		content.SetField(placeholder.Slot, fitParagraphs(bullets[start:end], placeholder))
	}
}

// fallbackRoles lays out the narrative arc: cover, alternating content with
// periodic section breaks, an optional pull quote and a closing slide.
func fallbackRoles(count int, manifest pptx.Manifest) []string {
	roles := make([]string, 0, count)
	roles = append(roles, pptx.RoleTitle)
	if count == 1 {
		return roles
	}
	closing := count >= 3
	body := count - 1
	if closing {
		body--
	}
	hasTwoContent := hasRole(manifest, pptx.RoleTwoContent)
	hasSection := hasRole(manifest, pptx.RoleSection)
	hasQuote := hasRole(manifest, pptx.RoleQuote)
	quoteAt := -1
	if hasQuote && body >= 5 {
		quoteAt = body * 2 / 3
	}
	for index := 0; index < body; index++ {
		switch {
		case index == quoteAt:
			roles = append(roles, pptx.RoleQuote)
		case hasSection && body >= 6 && index > 0 && index%4 == 0:
			roles = append(roles, pptx.RoleSection)
		case hasTwoContent && index > 0 && index%3 == 2:
			roles = append(roles, pptx.RoleTwoContent)
		default:
			roles = append(roles, pptx.RoleContent)
		}
	}
	if closing {
		roles = append(roles, pptx.RoleClosing)
	}
	return roles
}

func hasRole(manifest pptx.Manifest, role string) bool {
	for _, layout := range manifest.Layouts {
		if layout.Role == role {
			return true
		}
	}
	return false
}

// josa appends the Korean particle that agrees with the preceding word, so
// generated copy reads as written Korean rather than a template with holes.
// The choice depends on whether the last syllable ends in a consonant.
func josa(word, withFinal, withoutFinal string) string {
	word = strings.TrimSpace(word)
	if word == "" {
		return word
	}
	runes := []rune(word)
	last := runes[len(runes)-1]
	switch {
	case last >= 0xAC00 && last <= 0xD7A3:
		// Hangul syllables encode the final consonant in the low 28 values.
		if (last-0xAC00)%28 != 0 {
			return word + withFinal
		}
		return word + withoutFinal
	case last >= '0' && last <= '9':
		// Read by Korean digit names: 0,1,3,6,7,8 end in a consonant.
		if strings.ContainsRune("013678", last) {
			return word + withFinal
		}
		return word + withoutFinal
	}
	// Latin and other scripts: assume a trailing vowel is read openly.
	if strings.ContainsRune("aeiouAEIOU", last) {
		return word + withoutFinal
	}
	return word + withFinal
}

// subjectPhrase picks a short noun-like phrase to weave into generated
// sentences. The title is preferred because a brief is usually a full request
// ("… 계획을 임원진에게 보고") that reads badly inside another sentence.
func subjectPhrase(presentation model.Presentation, fallback string) string {
	if title := strings.TrimSpace(presentation.Title); title != "" && utf8.RuneCountInString(title) <= 40 {
		return title
	}
	prompt := strings.TrimSpace(presentation.Prompt)
	if prompt == "" {
		prompt = strings.TrimSpace(presentation.Title)
	}
	if prompt == "" {
		return fallback
	}
	// Keep the leading clause, which normally names the subject.
	if index := strings.IndexAny(prompt, ".,;·\n"); index > 4 {
		prompt = prompt[:index]
	}
	words := strings.Fields(prompt)
	for len(words) > 0 && utf8.RuneCountInString(strings.Join(words, " ")) > 28 {
		words = words[:len(words)-1]
	}
	if joined := strings.TrimSpace(strings.Join(words, " ")); joined != "" {
		return joined
	}
	return truncate(prompt, 28)
}

func paragraphs(lines []string) []pptx.Paragraph {
	result := make([]pptx.Paragraph, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		result = append(result, pptx.Paragraph{Text: strings.TrimSpace(line)})
	}
	return result
}

func presenterPerspective(profile model.Profile, phrases languageCopy) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(profile.Company) != "" {
		parts = append(parts, strings.TrimSpace(profile.Company))
	}
	if strings.TrimSpace(profile.JobTitle) != "" {
		parts = append(parts, strings.TrimSpace(profile.JobTitle))
	}
	if strings.TrimSpace(profile.Bio) != "" {
		parts = append(parts, truncate(strings.TrimSpace(profile.Bio), 120))
	}
	if len(parts) == 0 {
		return phrases.DefaultPerspective
	}
	return strings.Join(parts, " · ")
}

type sectionCopy struct {
	Title  string
	Lead   string
	Points func(topic, audience, tone, perspective string) []string
}

type languageCopy struct {
	DefaultTopic       string
	DefaultAudience    string
	DefaultTone        string
	DefaultPerspective string
	CoverSubtitle      string
	CoverNotes         func(topic, audience string) string
	SectionNotes       string
	SlideNotes         string
	QuoteText          string
	QuoteNotes         string
	ClosingTitle       string
	ClosingSubtitle    func(audience string) string
	ClosingNotes       string
	ClosingPoints      func(topic, perspective string) []string
	Sections           []sectionCopy
}

func localizedCopy(language string) languageCopy {
	switch {
	case strings.HasPrefix(strings.ToLower(language), "ja"):
		return japaneseCopy
	case strings.HasPrefix(strings.ToLower(language), "zh"):
		return chineseCopy
	case strings.HasPrefix(strings.ToLower(language), "ko"), strings.TrimSpace(language) == "":
		return koreanCopy
	}
	return englishCopy
}

var koreanCopy = languageCopy{
	DefaultTopic: "제안 주제", DefaultAudience: "일반 청중", DefaultTone: "전문적", DefaultPerspective: "발표자",
	CoverSubtitle: "대상: %s · 톤: %s",
	CoverNotes: func(topic, audience string) string {
		return fmt.Sprintf("%s 왜 지금 다뤄야 하는지 한 문장으로 밝히고, %s 오늘 얻어갈 것을 예고합니다.",
			josa(topic, "을", "를"), josa(audience, "이", "가"))
	},
	SectionNotes: "%s 구간으로 넘어간다는 신호를 주고 잠시 호흡을 둡니다.",
	SlideNotes:   "%s의 핵심을 %s의 언어로 풀어 설명하고, 숫자보다 원인에 무게를 둡니다.",
	QuoteText:    "%s의 성패는 실행 속도가 아니라 실행 순서에서 갈립니다",
	QuoteNotes:   "이 문장 뒤에 2초 정도 멈춰 청중이 곱씹을 시간을 줍니다.",
	ClosingTitle: "다음 단계",
	ClosingSubtitle: func(audience string) string {
		return fmt.Sprintf("%s 함께 확정할 사항", josa(audience, "과", "와"))
	},
	ClosingNotes: "%s에게 필요한 결정을 명확히 요청하고, 담당자와 기한을 말로 확인합니다.",
	ClosingPoints: func(topic, perspective string) []string {
		return []string{
			"이번 주 안에 실행 범위와 담당자를 확정합니다",
			fmt.Sprintf("%s 기준으로 첫 성과 지표를 합의합니다", perspective),
			fmt.Sprintf("%s 논의를 격주 리뷰로 이어갑니다", topic),
		}
	},
	Sections: []sectionCopy{
		{Title: "지금 논의해야 하는 이유", Lead: "%s 논의가 지금 필요한 배경입니다",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					fmt.Sprintf("%s 더 이상 미룰 수 없는 의사결정 지점에 도달했습니다", josa(topic, "은", "는")),
					fmt.Sprintf("%s 체감하는 문제와 내부 지표가 처음으로 일치했습니다", josa(audience, "이", "가")),
					"대응을 늦출수록 선택지가 줄어드는 구조입니다",
				}
			}},
		{Title: "현재 상태 진단", Lead: "%s의 현재 위치를 사실로만 확인합니다",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"성장은 유지되지만 개선 속도가 계획을 밑돌고 있습니다",
					"문제는 특정 단계에 집중되어 있어 원인 분리가 가능합니다",
					fmt.Sprintf("%s 관점에서 관리 가능한 범위 안에 있습니다", perspective),
				}
			}},
		{Title: "핵심 인사이트", Lead: "%s에서 발견한 결정적 사실입니다",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"증상이 아니라 구조를 바꿔야 결과가 바뀝니다",
					"가장 큰 손실은 눈에 띄지 않는 중간 단계에서 발생합니다",
					fmt.Sprintf("%s에게 설명 가능한 단일 원인으로 좁혔습니다", audience),
				}
			}},
		{Title: "제안 전략", Lead: "%s에 대한 우리의 답입니다",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"가장 큰 병목 한 곳에 자원을 집중합니다",
					"작게 시작해 2주 단위로 검증하고 확장합니다",
					fmt.Sprintf("%s 톤으로 이해관계자와 진행 상황을 공유합니다", tone),
				}
			}},
		{Title: "실행 로드맵", Lead: "%s 실행 순서와 시점을 정리합니다",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"1개월: 문제 구간 계측과 기준선 확보",
					"2~3개월: 개선안 적용과 A/B 검증",
					"4개월 이후: 성공 패턴을 전체로 확장",
				}
			}},
		{Title: "필요한 자원과 조건", Lead: "%s 실행에 필요한 최소 조건입니다",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"전담 인력 확보가 일정보다 중요한 변수입니다",
					"의사결정 주기를 격주로 단축해야 합니다",
					"기존 예산 재배분으로 추가 비용 없이 시작 가능합니다",
				}
			}},
		{Title: "리스크와 대응", Lead: "%s에서 예상되는 위험과 대비책입니다",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"초기 지표가 흔들릴 수 있어 관측 기간을 미리 정합니다",
					"조직 저항은 파일럿 성과로 설득합니다",
					"중단 기준을 먼저 합의해 매몰 비용을 막습니다",
				}
			}},
		{Title: "기대 효과", Lead: "%s 성공했을 때의 모습입니다",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"핵심 지표를 분기 내 반등 구간으로 되돌립니다",
					"반복 업무를 줄여 팀의 판단 시간을 확보합니다",
					fmt.Sprintf("%s에게 설명 가능한 성과 체계를 남깁니다", audience),
				}
			}},
	},
}

var englishCopy = languageCopy{
	DefaultTopic: "the proposal", DefaultAudience: "a general audience", DefaultTone: "professional", DefaultPerspective: "the presenter",
	CoverSubtitle: "Audience: %s · Tone: %s",
	CoverNotes: func(topic, audience string) string {
		return fmt.Sprintf("Say why %s matters now, and tell %s what they will leave with.", topic, audience)
	},
	SectionNotes: "Signal the move into %s and give the room a beat to reset.",
	SlideNotes:   "Explain %s in the language of %s, and weight causes over numbers.",
	QuoteText:    "%s is decided by the order of execution, not its speed",
	QuoteNotes:   "Pause for two seconds after this line so it lands.",
	ClosingTitle: "Next steps",
	ClosingSubtitle: func(audience string) string {
		return fmt.Sprintf("What we decide with %s today", audience)
	},
	ClosingNotes: "Ask %s for the specific decision, then confirm owner and date out loud.",
	ClosingPoints: func(topic, perspective string) []string {
		return []string{
			"Confirm scope and a single owner this week",
			fmt.Sprintf("Agree the first success metric with %s", perspective),
			fmt.Sprintf("Carry %s into a fortnightly review", topic),
		}
	},
	Sections: []sectionCopy{
		{Title: "Why this matters now", Lead: "The case for taking on %s today",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					fmt.Sprintf("%s has reached a decision point we cannot defer", topic),
					fmt.Sprintf("What %s reports and what our data shows finally agree", audience),
					"Every quarter of delay removes an option we still have",
				}
			}},
		{Title: "Where we stand", Lead: "An honest read of %s today",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"Growth continues, but improvement trails the plan",
					"The loss concentrates in one stage, so the cause is separable",
					fmt.Sprintf("It stays inside what %s can control", perspective),
				}
			}},
		{Title: "The insight", Lead: "What %s actually told us",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"Changing the structure changes the result; treating symptoms does not",
					"The largest loss hides in the step nobody reports on",
					fmt.Sprintf("We narrowed it to one cause we can explain to %s", audience),
				}
			}},
		{Title: "The proposal", Lead: "Our answer to %s",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"Concentrate resources on the single largest bottleneck",
					"Start small, validate in two-week cycles, then scale",
					fmt.Sprintf("Report progress to stakeholders in a %s register", tone),
				}
			}},
		{Title: "Roadmap", Lead: "The sequence for delivering %s",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"Month 1: instrument the failing stage and set a baseline",
					"Months 2-3: ship the change and validate against control",
					"Month 4 onward: extend the winning pattern across the funnel",
				}
			}},
		{Title: "What we need", Lead: "The minimum conditions for %s",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"A dedicated owner matters more than a compressed timeline",
					"Decision cadence has to shorten to fortnightly",
					"Reallocation covers this; no incremental budget is required",
				}
			}},
		{Title: "Risks and mitigations", Lead: "What could go wrong with %s",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"Early metrics will wobble, so fix the observation window first",
					"Answer internal resistance with pilot evidence, not argument",
					"Agree the stop condition now to avoid sunk-cost drift",
				}
			}},
		{Title: "What success looks like", Lead: "The state we reach if %s works",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"The core metric returns to growth within the quarter",
					"Less repetitive work, so the team spends its time judging",
					fmt.Sprintf("A results story %s can retell without us", audience),
				}
			}},
	},
}

var japaneseCopy = languageCopy{
	DefaultTopic: "提案テーマ", DefaultAudience: "一般の聴衆", DefaultTone: "プロフェッショナル", DefaultPerspective: "発表者の視点",
	CoverSubtitle: "対象: %s · トーン: %s",
	CoverNotes: func(topic, audience string) string {
		return fmt.Sprintf("%sを今扱う理由を一文で示し、%sが持ち帰るものを先に伝えます。", topic, audience)
	},
	SectionNotes: "%sへ移ることを示し、一呼吸置きます。",
	SlideNotes:   "%sの要点を%sの言葉で説明し、数字より原因に重心を置きます。",
	QuoteText:    "%sの成否は実行の速さではなく順序で決まります",
	QuoteNotes:   "この一文の後に二秒置き、聴衆に考える時間を与えます。",
	ClosingTitle: "次のステップ",
	ClosingSubtitle: func(audience string) string {
		return fmt.Sprintf("%sと今日決めること", audience)
	},
	ClosingNotes: "%sに必要な意思決定を明確に求め、担当と期限を口頭で確認します。",
	ClosingPoints: func(topic, perspective string) []string {
		return []string{
			"今週中に範囲と担当者を確定します",
			fmt.Sprintf("%sの基準で最初の成功指標に合意します", perspective),
			fmt.Sprintf("%sの議論を隔週レビューに引き継ぎます", topic),
		}
	},
	Sections: []sectionCopy{
		{Title: "今議論すべき理由", Lead: "%sに今取り組む背景を整理します",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					fmt.Sprintf("%sは先送りできない意思決定の局面にあります", topic),
					fmt.Sprintf("%sの実感と社内指標が初めて一致しました", audience),
					"遅らせるほど選べる手が減る構造です",
				}
			}},
		{Title: "現状の診断", Lead: "%sの現在地を事実だけで確認します",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"成長は続いていますが改善速度が計画を下回っています",
					"損失は特定の段階に集中し、原因を切り分けられます",
					fmt.Sprintf("%sの管理可能な範囲に収まっています", perspective),
				}
			}},
		{Title: "重要な洞察", Lead: "%sから見えた決定的な事実です",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"症状ではなく構造を変えなければ結果は変わりません",
					"最大の損失は誰も報告しない中間工程で生じています",
					fmt.Sprintf("%sに説明できる単一の原因まで絞りました", audience),
				}
			}},
		{Title: "提案", Lead: "%sに対する私たちの答えです",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"最大のボトルネック一点に資源を集中します",
					"小さく始め、二週間単位で検証して広げます",
					fmt.Sprintf("%sなトーンで関係者に進捗を共有します", tone),
				}
			}},
		{Title: "実行計画", Lead: "%sをいつ何から進めるかを示します",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"1か月目: 問題箇所の計測と基準値の確保",
					"2〜3か月目: 施策の実装と対照群での検証",
					"4か月目以降: 成功パターンを全体へ展開",
				}
			}},
		{Title: "必要な条件", Lead: "%sの実行に必要な最小条件です",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"専任担当の確保が日程より重要な変数です",
					"意思決定の周期を隔週まで短縮する必要があります",
					"既存予算の再配分で追加費用なく開始できます",
				}
			}},
		{Title: "リスクと対応", Lead: "%sで想定されるリスクと備えです",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"初期指標は揺れるため観測期間を先に決めます",
					"組織的な抵抗にはパイロットの実績で応えます",
					"中止条件を先に合意し、埋没費用を避けます",
				}
			}},
		{Title: "期待効果", Lead: "%sが成功したときの姿です",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"主要指標を四半期内に反転させます",
					"反復作業を減らし、判断に使う時間を確保します",
					fmt.Sprintf("%sが自ら語れる成果の物語を残します", audience),
				}
			}},
	},
}

var chineseCopy = languageCopy{
	DefaultTopic: "提案主题", DefaultAudience: "一般受众", DefaultTone: "专业", DefaultPerspective: "汇报者视角",
	CoverSubtitle: "受众: %s · 语气: %s",
	CoverNotes: func(topic, audience string) string {
		return fmt.Sprintf("用一句话说明为什么现在要谈%s，并预告%s今天能带走什么。", topic, audience)
	},
	SectionNotes: "提示进入%s部分，并留出一次呼吸。",
	SlideNotes:   "用%s的语言解释%s的要点，把重心放在原因而不是数字上。",
	QuoteText:    "%s的成败取决于执行的顺序，而不是速度",
	QuoteNotes:   "说完这句停两秒，让听众消化。",
	ClosingTitle: "下一步",
	ClosingSubtitle: func(audience string) string {
		return fmt.Sprintf("今天与%s共同确定的事项", audience)
	},
	ClosingNotes: "向%s明确提出需要的决策，并口头确认负责人与时间。",
	ClosingPoints: func(topic, perspective string) []string {
		return []string{
			"本周内确定范围与唯一负责人",
			fmt.Sprintf("以%s为准共识第一个成功指标", perspective),
			fmt.Sprintf("把%s纳入双周复盘", topic),
		}
	},
	Sections: []sectionCopy{
		{Title: "为什么是现在", Lead: "梳理现在处理%s的背景",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					fmt.Sprintf("%s已到了无法再推迟的决策节点", topic),
					fmt.Sprintf("%s的直观感受与内部数据首次一致", audience),
					"越晚应对，可选方案越少",
				}
			}},
		{Title: "现状诊断", Lead: "只用事实确认%s的当前位置",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"增长仍在，但改善速度低于计划",
					"损失集中在特定环节，因此原因可被分离",
					fmt.Sprintf("仍在%s可控的范围之内", perspective),
				}
			}},
		{Title: "关键洞察", Lead: "%s揭示的决定性事实",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"改变结构才会改变结果，处理症状不会",
					"最大的损失藏在无人汇报的中间环节",
					fmt.Sprintf("已收敛为一个可以向%s解释的原因", audience),
				}
			}},
		{Title: "提案", Lead: "我们对%s的回答",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"把资源集中在最大的一个瓶颈上",
					"小步启动，以两周为周期验证后再扩大",
					fmt.Sprintf("以%s的语气向相关方同步进展", tone),
				}
			}},
		{Title: "执行路线", Lead: "%s的推进顺序",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"第1个月：为问题环节建立度量与基线",
					"第2至3个月：落地改动并做对照验证",
					"第4个月起：把有效模式推广到全流程",
				}
			}},
		{Title: "所需条件", Lead: "推进%s的最低条件",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"专职负责人比压缩工期更关键",
					"决策周期需要缩短到双周",
					"通过预算再分配即可启动，无需追加",
				}
			}},
		{Title: "风险与应对", Lead: "%s可能出现的风险与准备",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"早期指标会波动，先约定观察窗口",
					"以试点结果回应内部阻力，而非争辩",
					"先约定终止条件，避免沉没成本",
				}
			}},
		{Title: "预期成效", Lead: "%s成功后的样子",
			Points: func(topic, audience, tone, perspective string) []string {
				return []string{
					"核心指标在本季度内重回增长",
					"减少重复工作，把时间留给判断",
					fmt.Sprintf("留下一套%s能自己复述的成果体系", audience),
				}
			}},
	},
}
