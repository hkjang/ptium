package pptx

import (
	"strings"
	"unicode/utf8"
)

// FitParagraphs trims paragraphs to what a slot can hold, so an exported slide
// never overflows the box its template drew. Language matters: a Korean line
// holds far fewer characters than an English one of the same width.
func FitParagraphs(paragraphs []Paragraph, placeholder Placeholder, language string) []Paragraph {
	if len(paragraphs) == 0 {
		return nil
	}
	maxLines := placeholder.MaxLines
	if maxLines <= 0 {
		maxLines = 1
	}
	switch placeholder.Slot {
	case SlotTitle, SlotSubtitle:
		// A title is one statement; join stray lines rather than dropping them.
		text := paragraphs[0].Text
		for _, extra := range paragraphs[1:] {
			text += " " + extra.Text
		}
		return []Paragraph{{Text: trimToWidth(text, budgetChars(placeholder.MaxChars*2, language))}}
	}
	// Two lines of wrapped text per bullet is the most a slide should carry.
	budget := maxLines
	if budget > 14 {
		budget = 14
	}
	result := make([]Paragraph, 0, len(paragraphs))
	used := 0
	for _, paragraph := range paragraphs {
		if used >= budget {
			break
		}
		text := trimToWidth(paragraph.Text, budgetChars(placeholder.MaxChars, language))
		used += LineCount(text, placeholder, paragraph.Level)
		result = append(result, Paragraph{Text: text, Level: paragraph.Level})
	}
	return result
}

// trimToWidth shortens text at a word boundary and marks the cut so an editor
// can see something was dropped.
// budgetChars scales a character budget measured for a reference language to the
// language actually being written.
func budgetChars(limit int, language string) int {
	if limit <= 0 || language == "" {
		return limit
	}
	scaled := float64(limit) * referenceAdvance / LanguageAdvance(language)
	if scaled < 8 {
		scaled = 8
	}
	return int(scaled)
}

func trimToWidth(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	cut := string(runes[:limit])
	if index := strings.LastIndexAny(cut, " ,·"); index > limit/2 {
		cut = cut[:index]
	}
	return strings.TrimSpace(cut) + "…"
}

// SanitizeBlock validates a component before it is drawn. Everything a slide
// renderer would trip over is corrected here rather than at draw time: an
// unknown kind, a component with too few entries to mean anything, labels long
// enough to collide, or a chart with more series than the template's palette
// can tell apart.
func SanitizeBlock(block Block, placeholder Placeholder) (Block, bool) {
	kind := strings.TrimSpace(block.Kind)
	if !knownBlockKind(kind) {
		return Block{}, false
	}
	block.Kind = kind
	block.Heading = truncate(strings.TrimSpace(block.Heading), 80)
	block.Caption = truncate(strings.TrimSpace(block.Caption), 160)
	block.Text = truncate(strings.TrimSpace(block.Text), 300)
	block.Attribute = truncate(strings.TrimSpace(block.Attribute), 80)
	block.Unit = truncate(strings.TrimSpace(block.Unit), 8)

	items := make([]Item, 0, len(block.Items))
	for _, item := range block.Items {
		item.Label = truncate(strings.TrimSpace(item.Label), 60)
		item.Value = truncate(strings.TrimSpace(item.Value), 24)
		item.Delta = truncate(strings.TrimSpace(item.Delta), 24)
		item.Detail = truncate(strings.TrimSpace(item.Detail), 120)
		item.Trend = strings.ToLower(strings.TrimSpace(item.Trend))
		bullets := make([]string, 0, len(item.Bullets))
		for _, bullet := range item.Bullets {
			if trimmed := strings.TrimSpace(bullet); trimmed != "" {
				bullets = append(bullets, truncate(trimmed, 120))
			}
			if len(bullets) == 4 {
				break
			}
		}
		item.Bullets = bullets
		if item.Label == "" && item.Value == "" && item.Number == nil && len(item.Bullets) == 0 {
			continue
		}
		items = append(items, item)
	}
	block.Items = items

	series := make([]Series, 0, len(block.Series))
	for _, candidate := range block.Series {
		if len(candidate.Points) < 2 {
			continue
		}
		if len(candidate.Points) > 12 {
			candidate.Points = candidate.Points[:12]
		}
		candidate.Name = truncate(strings.TrimSpace(candidate.Name), 40)
		series = append(series, candidate)
		if len(series) == 3 {
			break
		}
	}
	block.Series = series

	labels := make([]string, 0, len(block.Labels))
	for _, label := range block.Labels {
		labels = append(labels, truncate(strings.TrimSpace(label), 24))
	}
	block.Labels = labels
	if block.Emphasis < 0 || block.Emphasis > len(block.Items) {
		block.Emphasis = 0
	}

	// A component that would draw nothing is worse than prose, so the caller is
	// told to fall back rather than emitting an empty frame.
	switch kind {
	case BlockBullets:
		return Block{}, false
	case BlockKPI, BlockMeter, BlockColumns, BlockBars:
		if len(block.Items) == 0 {
			return Block{}, false
		}
	case BlockHero:
		if len(block.Items) == 0 || block.Items[0].Display(block.Unit) == "" {
			return Block{}, false
		}
	case BlockSteps, BlockTimeline, BlockComparison, BlockShare:
		if len(block.Items) < 2 {
			return Block{}, false
		}
	case BlockLine:
		if len(block.Series) == 0 {
			return Block{}, false
		}
	case BlockTable:
		if len(block.Columns) == 0 || len(block.Rows) == 0 {
			return Block{}, false
		}
	case BlockQuote, BlockCallout:
		if block.Text == "" && block.Caption == "" && len(block.Items) == 0 {
			return Block{}, false
		}
	}
	// A component needs room; a slot too short for this kind stays prose.
	if placeholder.MaxLines < BlockMinimumLines(kind) {
		return Block{}, false
	}
	return block, true
}

func knownBlockKind(kind string) bool {
	for _, candidate := range BlockKinds() {
		if candidate == kind {
			return true
		}
	}
	return false
}

func truncate(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
