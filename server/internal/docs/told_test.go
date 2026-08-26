package docs

import (
	"os"
	"strings"
	"testing"
)

// What the product reads has to be what the product says it reads.
//
// A format added to Extensions and to nothing else is a feature nobody finds:
// the first thing somebody reads is the README, and the person deciding what to
// upload reads the guide. Both have been silent about a working feature before
// — .xlsx, .docx and .md were read for a year with the README naming only
// .pptx — and nothing failed, because nothing was looking.
func TestEveryFormatReadIsAFormatSaid(t *testing.T) {
	// The extension as a page writes it, in code marks. Looking for the bare
	// word instead would pass on a README that only mentions exporting a PDF,
	// which is a different feature and was the first way this check fooled
	// itself.
	said := map[string]string{
		".csv":      "`.csv`",
		".tsv":      "`.tsv`",
		".xlsx":     "`.xlsx`",
		".docx":     "`.docx`",
		".pdf":      "`.pdf`",
		".md":       "`.md`",
		".markdown": "`.md`",
		".txt":      "`.txt`",
	}
	for _, page := range []string{"../../../README.md", "../../../docs/user-guide.md"} {
		content, err := os.ReadFile(page)
		if err != nil {
			t.Fatalf("%s: %v", page, err)
		}
		lowered := strings.ToLower(string(content))
		for _, extension := range Extensions {
			named, known := said[extension]
			if !known {
				t.Errorf("%s is read and this test does not know what to call it", extension)
				continue
			}
			if !strings.Contains(lowered, named) {
				t.Errorf("the product reads %s and %s never says so", extension, page)
			}
		}
	}
}
