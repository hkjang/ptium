package generation

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// English puts the head noun first. Keeping the tail — which is right for
// Korean — produced slide titles that read "two regions team".
func TestALatinTopicKeepsItsHead(t *testing.T) {
	cases := map[string]string{
		"A plan to make the payment platform redundant across two regions": "Plan to make the payment platform redundant",
		"migration in three phases":                                        "Migration in three phases",
		"Onboarding guide for new engineers joining the payments team":     "Onboarding guide for new engineers",
	}
	for prompt, want := range cases {
		if got := capitalized(topicPhrase(prompt)); got != want {
			t.Errorf("topicPhrase(%q) = %q, want %q", prompt, got, want)
		}
	}
}

// A brief states its figures in whatever units its language uses. Reading only
// Korean units meant an English brief's numbers became slide titles instead of
// the figures on a slide.
func TestEnglishFiguresAreRead(t *testing.T) {
	outline := outlinePrompt("A redundancy plan. Target availability 99.95%, budget 400M KRW, migration in three phases.",
		"", englishCopy)
	if !outline.hasFigures() {
		t.Fatalf("no figures read: %#v", outline)
	}
	values := make([]string, 0, len(outline.Figures))
	for _, figure := range outline.Figures {
		values = append(values, figure.Value)
	}
	joined := strings.Join(values, " ")
	for _, wanted := range []string{"99.95%", "400M KRW"} {
		if !strings.Contains(joined, wanted) {
			t.Errorf("the figures %v do not include %s", values, wanted)
		}
	}
	for _, topic := range outline.Topics {
		if strings.Contains(topic.Name, "400M") {
			t.Errorf("a figure became a topic: %q", topic.Name)
		}
	}
}

// Two topics that both read as a plan used to rotate identically, and the same
// figures, lead and notes came out on two slides.
func TestNoTwoSlidesArgueTheSameWay(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"en", "ko"} {
		prompt := "A plan to make the payment platform redundant across two regions for the executive team. " +
			"Target availability 99.95%, budget 400M KRW, migration in three phases."
		if language == "ko" {
			prompt = "결제 플랫폼을 두 리전으로 이중화하는 계획을 경영진에게 보고. 목표 가용성 99.95%, 예산 4억, 3단계 이행."
		}
		generated := Fallback(model.Presentation{Language: language, RequestedSlideCount: 9, Prompt: prompt},
			model.Profile{}, Template{ID: "t", Manifest: manifest})
		titles, leads := map[string]bool{}, map[string]bool{}
		for _, line := range strings.Split(generated.Source, "\n") {
			switch {
			case strings.HasPrefix(line, "# "):
				title := strings.TrimSpace(line[2:])
				if titles[title] {
					t.Errorf("%s: two slides are titled %q:\n%s", language, title, generated.Source)
				}
				titles[title] = true
			case strings.HasPrefix(line, "> "):
				lead := strings.TrimSpace(line[2:])
				if leads[lead] {
					t.Errorf("%s: two slides open with %q:\n%s", language, lead, generated.Source)
				}
				leads[lead] = true
			}
		}
	}
}

// A slide title lifted out of a brief arrives mid-sentence. In Latin script it
// starts the line, and a lower-case title reads as an unfinished note.
func TestLatinTitlesAreCapitalized(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	generated := Fallback(model.Presentation{Language: "en", RequestedSlideCount: 8,
		Prompt: "q3 marketing results and the plan for q4. revenue grew 12%, CAC down 8%, 3 new channels."},
		model.Profile{}, Template{ID: "t", Manifest: manifest})
	for _, line := range strings.Split(generated.Source, "\n") {
		if !strings.HasPrefix(line, "# ") {
			continue
		}
		title := strings.TrimSpace(line[2:])
		if title == "" {
			continue
		}
		if first := []rune(title)[0]; first >= 'a' && first <= 'z' {
			t.Errorf("the slide is titled %q:\n%s", title, generated.Source)
		}
	}
}

// Japanese and Chinese are written without spaces, so every word-based rule
// above was a no-op on them: a phrase was cut mid-word and a brief's figures
// became slide titles.
func TestASpacelessBriefIsReadAsPhrases(t *testing.T) {
	data, err := pptx.BuiltinTemplate("")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		language string
		prompt   string
		title    string
		figures  []string
	}{
		{"ja", "決済プラットフォームを二つのリージョンに冗長化する計画。目標可用性99.95%、予算4億、3段階で移行。",
			"決済プラットフォームを二つのリージョンに冗長化する計画", []string{"目標可用性 | 99.95%", "予算 | 4億"}},
		{"zh", "将支付平台在两个区域实现冗余的计划。目标可用性99.95%，预算4亿元，分三个阶段迁移。",
			"将支付平台在两个区域实现冗余的计划", []string{"目标可用性 | 99.95%", "预算 | 4亿元"}},
	}
	for _, test := range cases {
		generated := Fallback(model.Presentation{Language: test.language, RequestedSlideCount: 8, Prompt: test.prompt},
			model.Profile{}, Template{ID: "t", Manifest: manifest})
		if !strings.HasPrefix(generated.Source, "# "+test.title+"\n") {
			t.Errorf("%s: the deck is titled %q, want %q", test.language,
				strings.SplitN(generated.Source, "\n", 2)[0], "# "+test.title)
		}
		for _, figure := range test.figures {
			if !strings.Contains(generated.Source, "- "+figure) {
				t.Errorf("%s: the figures do not include %q:\n%s", test.language, figure, generated.Source)
			}
		}
		// A figure the brief gave is a number on a slide, never the slide's title.
		for _, line := range strings.Split(generated.Source, "\n") {
			if !strings.HasPrefix(line, "# ") {
				continue
			}
			for _, unwanted := range []string{"99.95", "4億", "4亿"} {
				if strings.Contains(line, unwanted) {
					t.Errorf("%s: a figure became the title %q", test.language, line)
				}
			}
		}
		// And a lead never opens on the particle its subject left behind.
		for _, line := range strings.Split(generated.Source, "\n") {
			if !strings.HasPrefix(line, "> ") {
				continue
			}
			// Japanese case particles and punctuation. A Chinese lead may open on
			// 从 or 把 — those are words there, not leftovers.
			leftovers := map[rune]bool{'の': true, 'を': true, 'は': true, 'が': true, 'に': true,
				'で': true, 'と': true, 'も': true, '、': true, '。': true, '，': true, '的': true}
			if first := []rune(strings.TrimSpace(line[2:]))[0]; leftovers[first] {
				t.Errorf("%s: a lead opens on a particle: %q", test.language, line)
			}
		}
	}
}

// A label is read back from the characters before the number, and stepping over
// a three-byte "、" as though it were one byte left a stray byte in front of it.
func TestASpacelessFigureLabelIsWholeCharacters(t *testing.T) {
	outline := outlinePrompt("計画。目標可用性99.95%、予算4億", "", japaneseCopy)
	for _, figure := range outline.Figures {
		if !utf8.ValidString(figure.Label) {
			t.Errorf("the label %q is not valid UTF-8", figure.Label)
		}
	}
	if len(outline.Figures) != 2 || outline.Figures[1].Label != "予算" {
		t.Fatalf("figures = %#v", outline.Figures)
	}
}
