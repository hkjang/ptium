package deck

import (
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/pptx"
)

// SourceFromImport writes a deck someone already had as deck source.
//
// The reader hands back the argument — titles, points, notes — and this turns it
// into the language the rest of the product is built on. From there the deck is
// an ordinary Ptium deck: it compiles into any template, it can be edited as
// words, and the model can be asked to rewrite a slide of it.
//
// What a slide carried besides words is reported rather than dropped silently. A
// photograph cannot be moved into another design at another aspect ratio and be
// trusted to look right, so the import says what it left behind.
func SourceFromImport(imported pptx.ImportedDeck) (string, []string) {
	return SourceFromImportWithImages(imported, nil)
}

// SourceFromImportWithImages is SourceFromImport with somewhere to put the
// pictures: store takes one and returns the name deck source should call it by.
// A picture it declines — because it is a logo repeated on every slide, or a
// decoration too small to be the point of the slide — is simply not placed.
func SourceFromImportWithImages(imported pptx.ImportedDeck, store func(pptx.ImportedPicture) (string, bool)) (string, []string) {
	var builder strings.Builder
	pictures, placed, tables, charts := 0, 0, 0, 0
	for index, slide := range imported.Slides {
		if index > 0 {
			builder.WriteString("\n")
		}
		title := strings.TrimSpace(slide.Title)
		if title == "" {
			title = fmt.Sprintf("%d번 슬라이드", index+1)
		}
		fmt.Fprintf(&builder, "# %s\n", escapeSourceLine(title))
		if name, ok := canonicalRoleName[strings.TrimSpace(slide.Role)]; ok {
			fmt.Fprintf(&builder, "@%s\n", name)
		}
		if lead := strings.TrimSpace(slide.Lead); lead != "" {
			fmt.Fprintf(&builder, "> %s\n", escapeSourceLine(lead))
		}
		for _, bullet := range slide.Bullets {
			fmt.Fprintf(&builder, "%s- %s\n", strings.Repeat("  ", bullet.Level), escapeSourceLine(bullet.Text))
		}
		// A table comes back as a table: the same grid, drawn by the design it
		// lands in rather than the one it came from.
		for _, table := range slide.Tables {
			builder.WriteString("::table\n")
			for _, row := range table {
				cells := make([]string, 0, len(row))
				for _, cell := range row {
					cells = append(cells, escapeItemField(cell))
				}
				fmt.Fprintf(&builder, "- %s\n", strings.Join(cells, " | "))
			}
			builder.WriteString("::\n")
			tables++
		}
		// A photograph goes into the region the new design keeps for one. Where it
		// sat in the old deck is not carried: coordinates chosen for one layout
		// mean nothing in another.
		for _, picture := range slide.Pictures {
			if store == nil {
				pictures++
				continue
			}
			name, ok := store(picture)
			if !ok {
				continue
			}
			fmt.Fprintf(&builder, "::image %s\n", escapeItemField(name))
			placed++
		}
		if notes := strings.TrimSpace(slide.Notes); notes != "" {
			fmt.Fprintf(&builder, "!notes %s\n", strings.ReplaceAll(notes, "\n", " "))
		}
		charts += slide.Charts
	}
	var warnings []string
	if placed > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"그림 %d개를 이미지 라이브러리에 저장하고 슬라이드에 넣었습니다", placed))
	}
	if pictures > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"그림 %d개는 가져오지 않았습니다. 이미지 탭에서 올려 다시 넣어 주세요", pictures))
	}
	if tables > 0 {
		warnings = append(warnings, fmt.Sprintf("표 %d개를 이 덱의 디자인으로 다시 그렸습니다", tables))
	}
	if charts > 0 {
		warnings = append(warnings, fmt.Sprintf(
			"차트 %d개는 가져오지 않았습니다. 숫자를 ::bars 나 ::line 으로 적으면 다시 그려집니다", charts))
	}
	return builder.String(), warnings
}
