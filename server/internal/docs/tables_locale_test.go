package docs

import "testing"

// amountSigns strips commas outright, so a sheet that uses the comma as its
// decimal point reads a thousand times small: "€1.234,56" becomes "1.23456" and
// draws a bar a thousandth of its height, with nothing on the page to say the
// chart disagrees with the cell. Before the European signs were added such a
// cell was no figure at all and the sheet stayed a table, which was right.
func TestACommaDecimalIsNotReadAsAnAmount(t *testing.T) {
	for _, written := range []string{
		"€1.234,56", "1.234,56", "€980,00", "1,23", "£1.000,50",
	} {
		if got, ok := amountOf(written); ok {
			t.Errorf("amountOf(%q) = %v, true; a comma-decimal sheet must stay a table", written, got)
		}
	}
}

// What the change was for still works.
func TestGroupedThousandsAndPlainFiguresStillRead(t *testing.T) {
	for written, want := range map[string]float64{
		"€1,234":     1234,
		"£980":       980,
		"$1,200":     1200,
		"¥1,234,567": 1234567,
		"1,200":      1200,
		"1200":       1200,
		"68%":        68,
	} {
		got, ok := amountOf(written)
		if !ok || got != want {
			t.Errorf("amountOf(%q) = %v, %v; want %v, true", written, got, ok, want)
		}
	}
}
