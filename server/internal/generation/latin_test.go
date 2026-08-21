package generation

import (
	"strings"
	"testing"

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
