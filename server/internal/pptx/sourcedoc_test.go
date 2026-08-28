package pptx

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// docs/deck-source.md is where an author learns what they may write. The parser
// accepts fourteen components under some sixty names, and the guide named every
// one of them but the horizontal bar chart — which is reachable only as
// `hbars`, `barchart`, `ranking` or `가로막대`, and appeared in the guide under
// none of those. A component nobody can find is a component nobody has.
//
// Worse, the name the guide did use for it — `bars` — is another spelling of
// the vertical one, so an author reaching for a ranking got columns.
//
// Skipped when the guide is not beside the server.
func TestEveryComponentAnAuthorMayWriteIsInTheGuide(t *testing.T) {
	guide, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "deck-source.md"))
	if err != nil {
		t.Skip("the source guide is not beside the server here")
	}
	written := string(guide)

	// Every name this parser answers to, by the component it makes.
	names := map[string][]string{
		BlockBullets:    {"bullets", "list", "목록"},
		BlockKPI:        {"kpi", "kpis", "metrics", "지표"},
		BlockHero:       {"hero", "figure", "big", "숫자"},
		BlockSteps:      {"steps", "process", "단계", "절차"},
		BlockTimeline:   {"timeline", "roadmap", "일정", "로드맵"},
		BlockComparison: {"comparison", "compare", "versus", "vs", "비교"},
		BlockColumns:    {"columnChart", "columns", "column", "bar", "bars", "세로막대"},
		BlockBars:       {"barChart", "hbar", "hbars", "ranking", "가로막대"},
		BlockLine:       {"lineChart", "line", "trend", "추이"},
		BlockShare:      {"shareBar", "share", "split", "비중"},
		BlockMeter:      {"meter", "gauge", "progress", "달성률"},
		BlockTable:      {"table", "표"},
		BlockGrid:       {"grid", "격자", "matrix", "raci"},
		BlockQuote:      {"quote", "statement", "인용"},
		BlockCallout:    {"callout", "note", "highlight", "강조"},
	}

	// Each name really is one this parser answers to, so the list above cannot
	// drift away from the code it stands for.
	for kind, spellings := range names {
		for _, spelling := range spellings {
			if got := BlockKind(spelling); got != kind {
				t.Errorf("%q is written here as %s and the parser reads it as %q", spelling, kind, got)
			}
		}
	}

	var unfindable []string
	for kind, spellings := range names {
		found := false
		for _, spelling := range spellings {
			if strings.Contains(written, "`"+spelling+"`") || strings.Contains(written, "::"+spelling) {
				found = true
				break
			}
		}
		if !found {
			unfindable = append(unfindable, kind)
		}
	}
	sort.Strings(unfindable)
	if len(unfindable) > 0 {
		t.Errorf("an author reading the guide cannot find these components under any name they could write: %v", unfindable)
	}
}
