package docs

import (
	"strings"
	"testing"
)

// A column of money is a column of figures, but a sheet almost never writes it
// bare: the Currency format puts a sign on every row of it, and the accounting
// convention puts a negative one in brackets. Read as text, a sheet of sales by
// region came out as a table of the same numbers instead of the chart anybody
// would have drawn from it.

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
			// one such row was enough to turn the whole chart back into a table.
			name:       "a negative amount written in brackets",
			csv:        "지역,매출\n서울,\"1,200\"\n부산,\"(340)\"\n",
			want:       []string{"::columns 매출", "- 부산 | (340)"},
			wantAbsent: []string{"::table 지역"},
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
		{"(1,200)", -1200, true},
		{"(₩340)", -340, true},
		{"68%", 68, true},
		{"1\u00a0200", 1200, true},
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
