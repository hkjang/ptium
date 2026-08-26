package pptx

import (
	"strconv"
	"strings"
)

// What a screen reader is told.
//
// A slide made of drawn shapes is, to anyone not looking at it, a slide made of
// nothing: PowerPoint's own accessibility check reports every chart, table and
// group as missing alternative text, and a deck that fails that check cannot be
// filed in most public-sector and financial organisations — the very places
// whose templates Ptium is built around.
//
// So every object Ptium draws says what it is and what it shows. The text is
// written from the same block the drawing came from, in the deck's language,
// which means it stays true when the numbers change.

// blockNames are what each component is called, per language.
var blockNames = map[string]map[string]string{
	"ko": {BlockKPI: "핵심 지표", BlockHero: "핵심 수치", BlockSteps: "단계 도해", BlockTimeline: "일정 도해",
		BlockComparison: "비교표", BlockColumns: "세로막대 차트", BlockBars: "가로막대 차트",
		BlockLine: "선 그래프", BlockShare: "점유 막대", BlockMeter: "달성률", BlockTable: "표",
		BlockQuote: "인용", BlockCallout: "강조 문구", BlockGrid: "격자표", BlockBullets: "요점"},
	"en": {BlockKPI: "key figures", BlockHero: "headline figure", BlockSteps: "process diagram",
		BlockTimeline: "timeline", BlockComparison: "comparison", BlockColumns: "column chart",
		BlockBars: "bar chart", BlockLine: "line chart", BlockShare: "share bar", BlockMeter: "progress meter",
		BlockTable: "table", BlockQuote: "quotation", BlockCallout: "callout", BlockGrid: "matrix",
		BlockBullets: "points"},
	"ja": {BlockKPI: "主要指標", BlockHero: "主要数値", BlockSteps: "手順図", BlockTimeline: "スケジュール図",
		BlockComparison: "比較表", BlockColumns: "縦棒グラフ", BlockBars: "横棒グラフ", BlockLine: "折れ線グラフ",
		BlockShare: "構成比バー", BlockMeter: "達成率", BlockTable: "表", BlockQuote: "引用",
		BlockCallout: "強調文", BlockGrid: "マトリクス", BlockBullets: "要点"},
	"zh": {BlockKPI: "关键指标", BlockHero: "核心数字", BlockSteps: "步骤图", BlockTimeline: "时间轴",
		BlockComparison: "对比表", BlockColumns: "柱状图", BlockBars: "条形图", BlockLine: "折线图",
		BlockShare: "占比条", BlockMeter: "完成率", BlockTable: "表格", BlockQuote: "引用",
		BlockCallout: "重点", BlockGrid: "矩阵", BlockBullets: "要点"},
}

// describeBlock writes the alternative text for a component: what it is, what
// it is headed, and what it actually says.
func describeBlock(block Block, language string) string {
	names, ok := blockNames[describeLanguage(language)]
	if !ok {
		names = blockNames["en"]
	}
	parts := make([]string, 0, 3)
	if name := names[block.Kind]; name != "" {
		parts = append(parts, name)
	}
	for _, heading := range []string{block.Heading, block.Caption} {
		if trimmed := strings.TrimSpace(heading); trimmed != "" {
			parts = append(parts, trimmed)
			break
		}
	}
	if content := describeContent(block); content != "" {
		parts = append(parts, content)
	}
	description := strings.Join(parts, ". ")
	// Alt text is read aloud. Past a couple of hundred characters it stops being
	// a description and becomes the slide read twice.
	const limit = 260
	if runes := []rune(description); len(runes) > limit {
		description = strings.TrimSpace(string(runes[:limit])) + "…"
	}
	return description
}

// describeContent is what the component shows, in the order it shows it.
func describeContent(block Block) string {
	switch block.Kind {
	case BlockQuote, BlockCallout:
		return strings.TrimSpace(block.Text)
	case BlockTable, BlockComparison, BlockGrid:
		if len(block.Rows) == 0 {
			break
		}
		entries := make([]string, 0, len(block.Rows)+1)
		if len(block.Columns) > 0 {
			entries = append(entries, strings.Join(block.Columns, " / "))
		}
		for _, row := range block.Rows {
			entries = append(entries, strings.Join(row, " / "))
		}
		return strings.Join(entries, "; ")
	case BlockLine:
		entries := make([]string, 0, len(block.Series)+1)
		if len(block.Labels) > 0 {
			entries = append(entries, strings.Join(block.Labels, ", "))
		}
		for _, series := range block.Series {
			points := make([]string, 0, len(series.Points))
			for _, point := range series.Points {
				points = append(points, formatNumber(point))
			}
			entry := strings.Join(points, ", ")
			if name := strings.TrimSpace(series.Name); name != "" {
				entry = name + " " + entry
			}
			entries = append(entries, entry)
		}
		return strings.Join(entries, "; ")
	}
	items := block.items()
	entries := make([]string, 0, len(items))
	for _, item := range items {
		entry := strings.TrimSpace(item.Label)
		if value := strings.TrimSpace(item.Display(block.Unit)); value != "" {
			entry = strings.TrimSpace(entry + " " + value)
		}
		if entry != "" {
			entries = append(entries, entry)
		}
	}
	if len(entries) == 0 && len(block.Rows) > 0 {
		for _, row := range block.Rows {
			entries = append(entries, strings.Join(row, " / "))
		}
	}
	return strings.Join(entries, "; ")
}

// describeLanguage reduces a tag such as "ko-KR" to the language alt text is
// written in.
func describeLanguage(language string) string {
	trimmed := strings.ToLower(strings.TrimSpace(language))
	if index := strings.IndexAny(trimmed, "-_"); index > 0 {
		trimmed = trimmed[:index]
	}
	if trimmed == "" {
		return "ko"
	}
	return trimmed
}

// describeTable is the alternative text for a table, which says its shape as
// well as its contents: "표 3열 4행" is what a reader needs before the cells.
func describeTable(part *TablePart, block Block, language string) string {
	if part == nil {
		return ""
	}
	// What is read aloud is what is drawn. A table caps its rows and stops at
	// the bottom of its region, and describing the rows that did not make it
	// tells someone who cannot see the slide about content nobody else has.
	drawn := block
	drawn.Columns, drawn.Rows = part.Columns, part.Rows
	description := describeBlock(drawn, language)
	shape := tableShape(len(part.Columns), len(part.Rows)+1, describeLanguage(language))
	if shape == "" {
		return description
	}
	if description == "" {
		return shape
	}
	return description + " (" + shape + ")"
}

func tableShape(columns, rows int, language string) string {
	if columns <= 0 || rows <= 0 {
		return ""
	}
	switch language {
	case "ko":
		return strconv.Itoa(columns) + "열 " + strconv.Itoa(rows) + "행"
	case "ja":
		return strconv.Itoa(columns) + "列 " + strconv.Itoa(rows) + "行"
	case "zh":
		return strconv.Itoa(columns) + "列 " + strconv.Itoa(rows) + "行"
	}
	return strconv.Itoa(columns) + " columns, " + strconv.Itoa(rows) + " rows"
}
