package docs

import (
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// A column of money is a column of figures, but a sheet almost never writes it
// bare: the Currency format puts a sign on every row of it. Read as text, a
// sheet of sales by region came out as a table of the same numbers instead of
// the chart anybody would have drawn from it.
//
// The accounting bracket is the one thing a sign coming off does not cover. A
// bar is drawn by its magnitude, so a refund would take the height of a month
// that sold as much, and a sheet holding one is left the table it was.

func TestASheetOfMoneyIsDrawnAsAChart(t *testing.T) {
	for _, test := range []struct {
		name       string
		csv        string
		want       []string
		wantAbsent []string
	}{
		{
			// What the Currency format writes in Korea, and the shape of every
			// sales sheet somebody brings to a deck.
			name: "an amount with a currency sign in front of it",
			csv:  "지역,매출\n서울,\"₩1,200\"\n부산,\"₩980\"\n",
			want: []string{"::columns 매출", "- 서울 | ₩1,200", "- 부산 | ₩980"},
			// The table it used to be, headed by the first cell of the sheet.
			wantAbsent: []string{"::table 지역"},
		},
		{
			// The same format written the other way round, which is what a sheet
			// spelling the currency out writes.
			name:       "an amount with the currency spelt after it",
			csv:        "지역,매출\n서울,\"1,200원\"\n부산,\"980원\"\n",
			want:       []string{"::columns 매출", "- 서울 | 1,200원"},
			wantAbsent: []string{"::table 지역"},
		},
		{
			// A refund. Brackets are how an accounting sheet writes a minus, and
			// nothing downstream can draw one: a bar is laid out by its
			// magnitude, so "(340)" would stand exactly as high as a month that
			// sold 340, with the minus surviving only in a value label the chart
			// drops past six bars. The sheet keeps its brackets as a table.
			name:       "a refund in brackets is left a table",
			csv:        "지역,매출\n서울,\"1,200\"\n부산,\"(340)\"\n",
			want:       []string{"::table 지역", "- 부산 | (340)"},
			wantAbsent: []string{"::columns 매출"},
		},
		{
			name:       "an amount in dollars",
			csv:        "지역,매출\n서울,\"$1,200.50\"\n부산,\"$980\"\n",
			want:       []string{"::columns 매출", "- 서울 | $1,200.50"},
			wantAbsent: []string{"::table 지역"},
		},
		{
			// A unit that is a word is not a sign on a figure: "1월" is a month,
			// and a column of months is the labels of a table, not its figures.
			name:       "a column of months is still a table",
			csv:        "구분,기간\n설계,1월\n개발,2월\n",
			want:       []string{"::table 구분", "- 설계 | 1월"},
			wantAbsent: []string{"::columns 기간"},
		},
		{
			// A thousand separated by the fixed space an export writes. The
			// deck's parser ends a figure at a space and would draw this row
			// as 1, so the sheet is better left the table it has always been
			// than sent on as a chart of heights nobody wrote.
			name:       "a thousand separated by a fixed space is still a table",
			csv:        "지역,매출\n서울,1\u00a0200\n부산,980\n",
			want:       []string{"::table 지역", "- 서울 | 1\u00a0200"},
			wantAbsent: []string{"::columns 매출"},
		},
		{
			// One row of prose in the column and it is not a column of figures,
			// however the rest of it is written.
			name:       "a column with a phrase in it is still a table",
			csv:        "지역,매출\n서울,\"₩1,200\"\n부산,미집계\n",
			want:       []string{"::table 지역", "- 부산 | 미집계"},
			wantAbsent: []string{"::columns 매출"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := Read("매출.csv", []byte(test.csv))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			for _, line := range test.want {
				if !strings.Contains(document.Source, line) {
					t.Errorf("the deck is missing %q:\n%s", line, document.Source)
				}
			}
			for _, line := range test.wantAbsent {
				if strings.Contains(document.Source, line) {
					t.Errorf("the deck still says %q:\n%s", line, document.Source)
				}
			}
		})
	}
}

func TestAnAmountIsReadWithItsSignTakenOff(t *testing.T) {
	for _, test := range []struct {
		written string
		want    float64
		ok      bool
	}{
		{"1200", 1200, true},
		{"1,200", 1200, true},
		{"₩1,200", 1200, true},
		{"￦1,200", 1200, true},
		{"$1,200.50", 1200.5, true},
		{"€1.5", 1.5, true},
		{"1,200원", 1200, true},
		{"-₩1,200", -1200, true},
		{"68%", 68, true},
		// The accounting bracket is a minus no bar can be drawn at, so a column
		// holding one is not a column this sends on to a chart.
		{"(1,200)", 0, false},
		{"(₩340)", 0, false},
		// The fixed space an export writes between a figure and its unit is a
		// space all the same, and the deck's parser ends a figure at one. Read
		// here as 1,200 the sheet goes on as a chart the deck then draws as 1,
		// so it is no figure at all and the sheet stays the table it was.
		{"1\u00a0200", 0, false},
		// Nothing but a sign is not an amount, and neither is a word with a
		// figure in it: a month is not the number one.
		{"₩", 0, false},
		{"1월", 0, false},
		{"3개", 0, false},
		{"()", 0, false},
		{"(미집계)", 0, false},
		{"", 0, false},
		{"미집계", 0, false},
		// A bracket that opens and never closes is not the accounting minus.
		{"(1,200", 0, false},
	} {
		got, ok := amountOf(test.written)
		if ok != test.ok || (ok && got != test.want) {
			t.Errorf("amountOf(%q) = %v, %v; want %v, %v", test.written, got, ok, test.want, test.ok)
		}
	}
}

// Calling a column figures and drawing it are two readings of the same cell,
// and they are made by two different parsers on purpose — this one decides
// whether a sheet is a chart at all, the deck's one sizes the bars and has a
// line chart's rows to keep whole. Two readings only stay honest if the sheet
// this one sends on as a chart is one the other draws at the figures the sheet
// shows. So every amount this calls a figure is followed all the way to the bar
// it becomes.
func TestTheChartReadsTheFiguresTheSheetWasSentOnFor(t *testing.T) {
	for _, test := range []struct {
		name string
		csv  string
		want map[string]float64
	}{
		{
			name: "amounts written with their currency",
			csv:  "지역,매출\n서울,\"₩1,200\"\n부산,\"₩980\"\n",
			want: map[string]float64{"서울": 1200, "부산": 980},
		},
		{
			name: "an amount with the currency spelt after it",
			csv:  "지역,매출\n서울,\"1,200원\"\n부산,\"980원\"\n",
			want: map[string]float64{"서울": 1200, "부산": 980},
		},
		{
			name: "amounts in dollars, one of them with a decimal",
			csv:  "지역,매출\n서울,\"$1,200.50\"\n부산,\"$980\"\n",
			want: map[string]float64{"서울": 1200.5, "부산": 980},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			document, err := Read("매출.csv", []byte(test.csv))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !strings.Contains(document.Source, "::columns") {
				t.Fatalf("the sheet was not sent on as a chart:\n%s", document.Source)
			}
			for label, want := range test.want {
				item, ok := chartItem(deck.ParseSource(document.Source), label)
				if !ok {
					t.Fatalf("the chart has no %q:\n%s", label, document.Source)
				}
				if item.Number == nil {
					t.Fatalf("the chart draws %q (%q) with no figure at all", label, item.Value)
				}
				if *item.Number != want {
					t.Errorf("the chart draws %q (%q) as %v; the sheet says %v",
						label, item.Value, *item.Number, want)
				}
			}
		})
	}
}

// chartItem finds a labelled bar in the first chart of a parsed deck.
func chartItem(source deck.Source, label string) (pptx.Item, bool) {
	for _, slide := range source.Slides {
		for _, block := range slide.Blocks {
			if block.Kind != pptx.BlockColumns {
				continue
			}
			for _, item := range block.Items {
				if item.Label == label {
					return item, true
				}
			}
		}
	}
	return pptx.Item{}, false
}
