package generation

import (
	"strings"
	"testing"
)

func figuresOf(pairs ...string) []promptFigure {
	figures := make([]promptFigure, 0, len(pairs))
	for _, pair := range pairs {
		label, value, _ := strings.Cut(pair, "=")
		figures = append(figures, promptFigure{Label: label, Value: value})
	}
	return figures
}

// Percentages that add up to a hundred are a split of one thing, and a split is
// the one shape a reader cannot get from a list.
func TestPartsOfOneWholeAreDrawnAsAShare(t *testing.T) {
	charts := chartsFromFigures(figuresOf("직판=46%", "대리점=33%", "온라인=21%"), koreanCopy)
	if len(charts) == 0 || charts[0].Kind != "share" {
		t.Fatalf("expected a share, got %#v", charts)
	}
	if len(charts[0].Rows) != 3 || charts[0].Rows[0] != "직판 | 46%" {
		t.Errorf("the rows are not the brief's own figures: %#v", charts[0].Rows)
	}
	if charts[0].Caption != "구성비" {
		t.Errorf("the caption is not in the deck's language: %q", charts[0].Caption)
	}
}

// The same measure at four moments is drawn with every moment named. A line
// would draw the values and drop the years, which is the one thing a reader of
// a business deck looks at first.
func TestTheSameMeasureOverTimeKeepsItsYears(t *testing.T) {
	charts := chartsFromFigures(figuresOf("매출은 2023년=820억", "2024년=910억", "2025년=1,040억"), koreanCopy)
	if len(charts) == 0 || charts[0].Kind != "columns" {
		t.Fatalf("expected columns, got %#v", charts)
	}
	if charts[0].Rows[0] != "2023년 | 820억" {
		t.Errorf("the first bar carries the subject rather than its year: %q", charts[0].Rows[0])
	}
	for index, row := range charts[0].Rows {
		if !strings.Contains(row, "년 | ") {
			t.Errorf("row %d is not a named point: %q", index, row)
		}
	}
}

// Categories that are not moments in time are magnitudes side by side.
func TestCategoriesAreDrawnAsColumns(t *testing.T) {
	charts := chartsFromFigures(figuresOf("서울=1,200건", "부산=860건", "대구=540건"), koreanCopy)
	if len(charts) == 0 || charts[0].Kind != "columns" {
		t.Fatalf("expected columns, got %#v", charts)
	}
}

// What the brief did not give, the deck does not draw.
func TestNothingIsDrawnFromFiguresThatAreNotAShape(t *testing.T) {
	for _, figures := range [][]promptFigure{
		figuresOf("예산=18억"),                         // one number is a sentence
		figuresOf("예산=18억", "회수=3년"),                // two units, no shape
		figuresOf("직판=46%", "대리점=33%"),              // a split needs three parts
		figuresOf("만족도=120%", "달성률=96%", "이익률=88%"), // a percentage over 100 is not a share
	} {
		for _, chart := range chartsFromFigures(figures, koreanCopy) {
			if chart.Kind == "share" || chart.Kind == "columns" {
				t.Errorf("%#v was drawn as %s: %#v", figures, chart.Kind, chart.Rows)
			}
		}
	}
}

// Percentages that are not a split are progress against something.
func TestAttainmentIsDrawnAsAMeter(t *testing.T) {
	charts := chartsFromFigures(figuresOf("매출 달성률=96%", "이익 달성률=88%"), koreanCopy)
	if len(charts) != 1 || charts[0].Kind != "meter" {
		t.Fatalf("expected a meter, got %#v", charts)
	}
}

// A deck is not a dashboard: at most two charts come from one brief.
func TestABriefGivesAtMostTwoCharts(t *testing.T) {
	charts := chartsFromFigures(figuresOf(
		"직판=46%", "대리점=33%", "온라인=21%",
		"2023년=820억", "2024년=910억", "2025년=1,040억"), koreanCopy)
	if len(charts) != 2 {
		t.Fatalf("expected two charts, got %d: %#v", len(charts), charts)
	}
	kinds := map[string]bool{charts[0].Kind: true, charts[1].Kind: true}
	if !kinds["share"] || !kinds["columns"] {
		t.Errorf("the two shapes in the brief are not the two charts: %#v", charts)
	}
}

// A figure's label is the words in front of it in the brief, and those words
// can be the tail of the previous sentence or the subject of this one. What
// belongs under a bar is the thing counted.
func TestABarIsNamedByWhatItCounts(t *testing.T) {
	charts := chartsFromFigures(figuresOf(
		"실적 보고. 서울=1,200건", "부산=860건", "대구=540건"), koreanCopy)
	if len(charts) == 0 {
		t.Fatal("no chart")
	}
	if charts[0].Rows[0] != "서울 | 1,200건" {
		t.Errorf("the first bar carries the sentence before it: %q", charts[0].Rows[0])
	}
	split := chartsFromFigures(figuresOf("채널별 비중은 직판=46%", "대리점=33%", "온라인=21%"), koreanCopy)
	if len(split) == 0 || split[0].Rows[0] != "직판 | 46%" {
		t.Errorf("the first part carries the subject of the sentence: %#v", split)
	}
}

// A heading says what a slide is about. The number belongs on the slide, drawn,
// and a brief's clause that ends in one gave three slides the title
// "채널 비중은 직판 46% — 기대 효과".
func TestAHeadingDoesNotEndInAMeasurement(t *testing.T) {
	cases := map[string]string{
		"채널 비중은 직판 46%":    "채널 비중은 직판",
		"지역별 처리 건수 1,200건": "지역별 처리 건수",
		"인력 재배치 12명":       "인력 재배치",
		// What is left has to still be a subject.
		"예산 4억":     "예산 4억",
		"18억":       "18억",
		"2026년 계획":  "2026년 계획",
		"고객 만족도 조사": "고객 만족도 조사",
	}
	for given, want := range cases {
		if got := headingName(given); got != want {
			t.Errorf("headingName(%q) = %q, want %q", given, got, want)
		}
	}
}
