package figures

import "testing"

// Of is the lenient read: a written value carries a figure, and the figure ends
// where its unit begins.
func TestOfReadsWrittenValues(t *testing.T) {
	cases := map[string]float64{
		"42개": 42, "18%": 18, "1,200억": 1200, "-3.5pt": -3.5, "12개월": 12, "0.5배": 0.5,
		"₩1,200": 1200, "1,200원": 1200, "$1,200.50": 1200.5, "(340)": -340, "(1,200원)": -1200,
		// A figure an export separated with a space, fixed or not. Reading the
		// space as the end of the figure made this 1.
		"1 200": 1200, "1\u00a0200": 1200, "1\u202f200": 1200,
		// A bracket that opens and never closes is not the accounting minus, so
		// what is inside it is read as it stands.
		"(1,200": 1200,
		// A note in brackets after a figure leaves the figure where it was.
		"340(잠정)": 340, " (340) ": -340,
	}
	for value, want := range cases {
		got, ok := Of(value)
		if !ok || got != want {
			t.Errorf("Of(%q) = %v, %v; want %v", value, got, ok, want)
		}
	}
	for _, value := range []string{"", "높음", "미정", "₩", "()", "(미집계)"} {
		if got, ok := Of(value); ok {
			t.Errorf("Of(%q) = %v, true; should find no figure", value, got)
		}
	}
}

// Alone is the strict read the importer classifies a column by: a figure and
// the marks a sheet puts on one, and nothing besides.
func TestAloneReadsAFigureAndNothingBesides(t *testing.T) {
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
		{"1 200", 1200, true},
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
		// A date is not one figure, whichever way it is written.
		{"2025. 1. 21", 0, false},
	} {
		got, ok := Alone(test.written)
		if ok != test.ok || (ok && got != test.want) {
			t.Errorf("Alone(%q) = %v, %v; want %v, %v", test.written, got, ok, test.want, test.ok)
		}
	}
}

// The invariant the two readings exist to keep. A column the importer calls
// figures is one the deck goes on to draw, and it draws it with Of: if the two
// ever read the same cell differently, the chart is heights nobody wrote.
func TestAloneAndOfReadTheSameFigure(t *testing.T) {
	for _, value := range []string{
		"1200", "1,200", "₩1,200", "1,200원", "$1,200.50", "(340)", "(₩340)", "-₩1,200",
		"68%", "1 200", "1\u00a0200", "1\u202f200", " 980 ", "0.5", "-0.5", "+12",
		"1월", "3개", "미집계", "", "(1,200", "2025. 1. 21",
	} {
		strict, alone := Alone(value)
		if !alone {
			continue
		}
		lenient, ok := Of(value)
		if !ok || lenient != strict {
			t.Errorf("Alone(%q) = %v but Of(%q) = %v, %v", value, strict, value, lenient, ok)
		}
	}
}
