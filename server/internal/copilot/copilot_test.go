package copilot

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// What someone types is read into an edit with one right answer, and the
// sentence it understood is shown back before anything changes.
func TestATypedSentenceBecomesAnEdit(t *testing.T) {
	cases := []struct {
		typed string
		kind  string
		check func(Command) bool
	}{
		{"3번과 4번 합쳐줘", KindMerge, func(c Command) bool { return c.Slides[0] == 3 && c.Slides[1] == 4 }},
		{"5번 슬라이드 삭제", KindDelete, func(c Command) bool { return len(c.Slides) == 1 && c.Slides[0] == 5 }},
		{"2번과 5번 지워줘", KindDelete, func(c Command) bool { return len(c.Slides) == 2 }},
		{"2번을 두 장으로 나눠줘", KindSplit, func(c Command) bool { return c.Slides[0] == 2 }},
		{"3번 복제", KindDuplicate, func(c Command) bool { return c.Slides[0] == 3 }},
		{"6번을 2번으로 옮겨줘", KindMove, func(c Command) bool { return c.Slides[0] == 6 && c.To == 2 }},
		{"8장으로 줄여줘", KindTrim, func(c Command) bool { return c.Count == 8 }},
		{"10분 발표로 맞춰줘", KindTrim, func(c Command) bool { return c.Count == 5 }},
		{"merge 3 and 4", KindMerge, func(c Command) bool { return c.Slides[0] == 3 }},
		{"delete slide 5", KindDelete, func(c Command) bool { return c.Slides[0] == 5 }},
	}
	for _, test := range cases {
		commands, err := Parse(test.typed, 10)
		if err != nil {
			t.Errorf("%q was not understood: %v", test.typed, err)
			continue
		}
		if commands[0].Kind != test.kind || !test.check(commands[0]) {
			t.Errorf("%q became %+v", test.typed, commands[0])
		}
		if strings.TrimSpace(commands[0].Reason) == "" {
			t.Errorf("%q says nothing about what it will do", test.typed)
		}
	}
	if _, err := Parse("이 덱 어때?", 10); err == nil {
		t.Error("a sentence that names no edit should say so rather than guessing")
	}
	if _, err := Parse("99번 삭제", 10); err == nil {
		t.Error("a slide that does not exist is not an edit")
	}
}

func slideWith(position int, title string, points ...string) model.Slide {
	content := deck.Content{Type: "template", LayoutID: "content"}
	content.SetField(pptx.SlotTitle, []pptx.Paragraph{{Text: title}})
	paragraphs := make([]pptx.Paragraph, 0, len(points))
	for _, point := range points {
		paragraphs = append(paragraphs, pptx.Paragraph{Text: point})
	}
	content.SetField(pptx.SlotBody, paragraphs)
	content.Sources = []pptx.Citation{{Marker: "1", Title: title + " 출처"}}
	return model.Slide{Position: position, Title: title, Content: content.Encode(), SpeakerNotes: title + " 노트"}
}

// Merging keeps everything both slides carried: the points, the notes and the
// evidence. A merged slide that lost half its sources would be worse than two.
func TestMergingKeepsBothSlidesWhole(t *testing.T) {
	slides := []model.Slide{
		slideWith(1, "표지"), slideWith(2, "실적", "매출 1,240억", "이익률 9.8%"),
		slideWith(3, "계획", "자동화 2단계"), slideWith(4, "마무리"),
	}
	commands, err := Parse("2번과 3번 합쳐줘", len(slides))
	if err != nil {
		t.Fatal(err)
	}
	merged, notes, err := Apply(slides, commands, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 3 {
		t.Fatalf("merging four slides gave %d", len(merged))
	}
	content := deck.Decode(merged[1].Content)
	points := strings.Join(content.PrimaryBullets(), " | ")
	for _, wanted := range []string{"매출 1,240억", "이익률 9.8%", "자동화 2단계"} {
		if !strings.Contains(points, wanted) {
			t.Errorf("the merged slide lost %q: %s", wanted, points)
		}
	}
	if len(content.Sources) != 2 {
		t.Errorf("the merged slide carries %d sources, want both", len(content.Sources))
	}
	if !strings.Contains(merged[1].SpeakerNotes, "실적 노트") || !strings.Contains(merged[1].SpeakerNotes, "계획 노트") {
		t.Errorf("the merged notes read %q", merged[1].SpeakerNotes)
	}
	if len(notes) == 0 {
		t.Error("the edit said nothing about what it did")
	}
	for index, slide := range merged {
		if slide.Position != index+1 {
			t.Errorf("slide %d is numbered %d", index+1, slide.Position)
		}
	}
}

// A trim drops what measured worst, and never the deck's opening or its close.
func TestATrimDropsWhatMeasuredWorst(t *testing.T) {
	slides := []model.Slide{
		slideWith(1, "표지"), slideWith(2, "좋은 장", "요점"), slideWith(3, "나쁜 장", "요점"),
		slideWith(4, "보통 장", "요점"), slideWith(5, "마무리"),
	}
	scores := map[int]int{1: 100, 2: 96, 3: 60, 4: 80, 5: 100}
	commands, err := Parse("3장으로 줄여줘", len(slides))
	if err != nil {
		t.Fatal(err)
	}
	trimmed, notes, err := Apply(slides, commands, func(position int) int { return scores[position] })
	if err != nil {
		t.Fatal(err)
	}
	if len(trimmed) != 3 {
		t.Fatalf("the trim left %d slides", len(trimmed))
	}
	titles := []string{trimmed[0].Title, trimmed[1].Title, trimmed[2].Title}
	if titles[0] != "표지" || titles[2] != "마무리" {
		t.Errorf("the trim removed the deck's own opening or close: %v", titles)
	}
	if titles[1] != "좋은 장" {
		t.Errorf("the trim kept %q rather than the best-measured slide", titles[1])
	}
	if len(notes) == 0 || !strings.Contains(notes[0], "3") {
		t.Errorf("the trim did not say what it dropped: %v", notes)
	}
}

// Splitting divides the points and says the second slide continues the first.
func TestSplittingDividesThePoints(t *testing.T) {
	slides := []model.Slide{slideWith(1, "실적", "하나", "둘", "셋", "넷")}
	commands, err := Parse("1번을 나눠줘", len(slides))
	if err != nil {
		t.Fatal(err)
	}
	split, _, err := Apply(slides, commands, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(split) != 2 {
		t.Fatalf("the split gave %d slides", len(split))
	}
	first := deck.Decode(split[0].Content).PrimaryBullets()
	second := deck.Decode(split[1].Content).PrimaryBullets()
	if len(first) != 2 || len(second) != 2 {
		t.Errorf("the points divided %d/%d", len(first), len(second))
	}
	if !strings.Contains(split[1].Title, "계속") {
		t.Errorf("the second slide is titled %q", split[1].Title)
	}
}
