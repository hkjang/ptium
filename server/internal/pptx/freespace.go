package pptx

import (
	"math"
	"strings"
)

// Many real templates carry no text placeholders at all. A deck exported from
// Google Slides or built by a designer often puts the whole design into pictures
// and shapes, leaving one plain layout with placeholders and a set of beautiful
// branded layouts with none. Ptium used to be unable to write onto those, so
// every slide landed on the one plain layout and the customer's design went
// unused — which is exactly what "the theme is not applied" looks like.
//
// The fix is to find where a layout has room, and to write there. The area not
// covered by the layout's own artwork is the space its designer left for content.

const (
	// freeSpaceGrid is the resolution of the occupancy map. 64x36 keeps a 16:9
	// slide's cells square and is fine enough to find a usable column.
	freeSpaceGridX = 64
	freeSpaceGridY = 36
	// backgroundCoverage is the share of the slide above which a piece of artwork
	// counts as a backdrop rather than an obstacle: text belongs on top of a
	// full-bleed photograph, not beside it.
	backgroundCoverage = 0.82
	// minimumFreeWidth and minimumFreeHeight are the smallest region worth
	// writing into, as a share of the slide.
	minimumFreeWidth  = 0.34
	minimumFreeHeight = 0.22
)

// synthesizeSlots gives a layout writable regions when it has none of its own.
//
// The result is marked synthetic: the renderer draws a real text box for it
// rather than filling a placeholder, since there is no placeholder to inherit
// from. Typography comes from the template's own theme and text styles, so the
// slide still reads as the customer's design.
func synthesizeSlots(layout Layout, theme Theme, slideWidth, slideHeight, titleSize, bodySize int) []Placeholder {
	if len(layout.TextSlots()) > 0 {
		return nil
	}
	frame, ok := freeRegion(layout, slideWidth, slideHeight)
	if !ok {
		return nil
	}
	// A cover or divider gets one large statement; a content slide gets a title
	// with a body beneath it.
	statement := layoutIsStatement(layout)
	// The master's own placeholder sizes describe its own placeholders, not a
	// region derived from free space, and templates exported from other tools
	// routinely record a 14pt title. Scale from the slide instead, which is what
	// a designer does, and let the theme supply the typeface and the colour.
	titleSize, bodySize = composedTypeScale(slideHeight, statement, titleSize, bodySize)
	ink := readableInkOver(layout, theme, frame)

	// The title band is sized by the text it holds, not by a share of the region:
	// a tall free area would otherwise push the body halfway down the slide.
	titleLine := titleSize * EMUPerPoint * 122 / 100 / 100
	titleHeight := min(int(float64(frame.Height)*0.34), titleLine*2)
	if statement {
		titleHeight = min(int(float64(frame.Height)*0.52), titleLine*3)
	}
	if titleHeight < titleLine {
		titleHeight = min(titleLine, frame.Height)
	}
	gap := EMUPerInch / 8

	title := Placeholder{
		Slot: SlotTitle, Kind: "text", Type: "title", Synthetic: true,
		Name: "Ptium title", X: frame.X, Y: frame.Y, Width: frame.Width, Height: titleHeight,
		FontSize: titleSize, Bold: true, Color: ink, Font: theme.MajorLatin,
	}
	result := []Placeholder{title}

	bodyY := frame.Y + titleHeight + gap
	bodyHeight := frame.Y + frame.Height - bodyY
	if bodyHeight >= EMUPerInch/2 {
		slot := SlotBody
		size := bodySize
		if statement {
			// A divider's second line is a lead-in, not a bullet list.
			slot = SlotSubtitle
			size = bodySize * 6 / 5
		}
		result = append(result, Placeholder{
			Slot: slot, Kind: "text", Type: "body", Synthetic: true,
			Name: "Ptium body", X: frame.X, Y: bodyY, Width: frame.Width, Height: bodyHeight,
			FontSize: size, Color: ink, Font: theme.MinorLatin,
		})
	}
	for index := range result {
		result[index].MaxChars, result[index].MaxLines, result[index].LineEm = capacity(result[index])
		result[index].Region = region(result[index], slideWidth, slideHeight)
	}
	return result
}

// composedTypeScale sizes type for a composed region from the slide itself.
//
// The proportions are the ones a print-trained designer uses: a cover line at
// about a fourteenth of the slide height, a slide title at a twentieth, body
// copy at a thirtieth. A template that states larger sizes than these is
// honoured, since that is its own voice; a smaller one is not, because it is
// describing a placeholder that does not exist here.
func composedTypeScale(slideHeight int, statement bool, masterTitle, masterBody int) (titleSize, bodySize int) {
	points := float64(slideHeight) / float64(EMUPerPoint)
	titleSize = int(points * 0.050 * 100)
	if statement {
		titleSize = int(points * 0.072 * 100)
	}
	bodySize = int(points * 0.033 * 100)
	if masterTitle > titleSize {
		titleSize = masterTitle
	}
	if masterBody > bodySize {
		bodySize = masterBody
	}
	return clampFontSize(titleSize, 1800, 6600), clampFontSize(bodySize, 1100, 2800)
}

func clampFontSize(size, low, high int) int {
	if size < low {
		return low
	}
	if size > high {
		return high
	}
	return size
}

// layoutIsStatement reports whether a layout reads as a cover or divider, where
// one large line carries the slide.
func layoutIsStatement(layout Layout) bool {
	lowered := strings.ToLower(layout.Name)
	for _, marker := range []string{"title", "cover", "메인", "표지", "간지", "divider", "section", "섹션", "closing", "thank", "감사"} {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return layout.Type == "title" || layout.Type == "secHead"
}

// freeRegion finds the largest rectangle the layout's artwork leaves empty.
func freeRegion(layout Layout, slideWidth, slideHeight int) (Frame, bool) {
	if slideWidth <= 0 || slideHeight <= 0 {
		return Frame{}, false
	}
	slideArea := float64(slideWidth) * float64(slideHeight)
	occupied := make([]bool, freeSpaceGridX*freeSpaceGridY)
	for _, piece := range layout.Artwork {
		if piece.Width <= 0 || piece.Height <= 0 {
			continue
		}
		// A backdrop is not an obstacle.
		if float64(piece.Width)*float64(piece.Height)/slideArea >= backgroundCoverage {
			continue
		}
		// Nor is a hairline rule or a thin brand bar worth avoiding vertically.
		left := piece.X * freeSpaceGridX / slideWidth
		right := (piece.X + piece.Width) * freeSpaceGridX / slideWidth
		top := piece.Y * freeSpaceGridY / slideHeight
		bottom := (piece.Y + piece.Height) * freeSpaceGridY / slideHeight
		for y := max(top, 0); y <= min(bottom, freeSpaceGridY-1); y++ {
			for x := max(left, 0); x <= min(right, freeSpaceGridX-1); x++ {
				occupied[y*freeSpaceGridX+x] = true
			}
		}
	}
	cellX, cellY, cellWidth, cellHeight, ok := largestEmptyRectangle(occupied)
	if !ok {
		return Frame{}, false
	}
	frame := Frame{
		X:      cellX * slideWidth / freeSpaceGridX,
		Y:      cellY * slideHeight / freeSpaceGridY,
		Width:  cellWidth * slideWidth / freeSpaceGridX,
		Height: cellHeight * slideHeight / freeSpaceGridY,
	}
	// Keep clear of both the artwork and the slide edge.
	margin := slideWidth / 24
	frame = insetFrame(frame, margin, slideWidth, slideHeight)
	if float64(frame.Width) < float64(slideWidth)*minimumFreeWidth ||
		float64(frame.Height) < float64(slideHeight)*minimumFreeHeight {
		return Frame{}, false
	}
	return frame, true
}

func insetFrame(frame Frame, margin, slideWidth, slideHeight int) Frame {
	frame.X += margin
	frame.Y += margin
	frame.Width -= margin * 2
	frame.Height -= margin * 2
	if frame.X < margin {
		frame.X = margin
	}
	if frame.Y < margin {
		frame.Y = margin
	}
	if frame.X+frame.Width > slideWidth-margin {
		frame.Width = slideWidth - margin - frame.X
	}
	if frame.Y+frame.Height > slideHeight-margin {
		frame.Height = slideHeight - margin - frame.Y
	}
	return frame
}

// largestEmptyRectangle finds the biggest all-false rectangle in the occupancy
// grid, by the standard largest-rectangle-in-histogram sweep over each row.
func largestEmptyRectangle(occupied []bool) (x, y, width, height int, ok bool) {
	heights := make([]int, freeSpaceGridX)
	best := 0
	for row := 0; row < freeSpaceGridY; row++ {
		for column := 0; column < freeSpaceGridX; column++ {
			if occupied[row*freeSpaceGridX+column] {
				heights[column] = 0
			} else {
				heights[column]++
			}
		}
		// Widest rectangle whose bottom edge is this row.
		stack := make([]int, 0, freeSpaceGridX+1)
		for column := 0; column <= freeSpaceGridX; column++ {
			current := 0
			if column < freeSpaceGridX {
				current = heights[column]
			}
			for len(stack) > 0 && heights[stack[len(stack)-1]] >= current {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				left := 0
				if len(stack) > 0 {
					left = stack[len(stack)-1] + 1
				}
				area := heights[top] * (column - left)
				if area > best {
					best = area
					x, y = left, row-heights[top]+1
					width, height = column-left, heights[top]
				}
			}
			stack = append(stack, column)
		}
	}
	return x, y, width, height, best > 0
}

// readableInkOver picks a text colour that can be read against whatever the
// layout paints behind the given frame.
func readableInkOver(layout Layout, theme Theme, frame Frame) string {
	behind := layout.Fill.Fill
	if behind == "" {
		behind = layout.Background
	}
	if len(layout.Fill.Gradient) > 0 {
		behind = layout.Fill.Gradient[0].Color
	}
	// The nearest piece of artwork that covers the frame decides, since it is
	// what the text actually sits on.
	for _, piece := range layout.Artwork {
		if piece.Kind == "text" {
			continue
		}
		if !covers(piece, frame) {
			continue
		}
		switch {
		case piece.Average != "":
			behind = piece.Average
		case piece.Fill != "":
			behind = piece.Fill
		case len(piece.Gradient) > 0:
			behind = piece.Gradient[0].Color
		}
	}
	if behind == "" {
		behind = theme.Color("lt1")
	}
	dark := theme.Color("dk1")
	light := theme.Color("lt1")
	ink := readableInk(behind, light, dark)
	if contrastRatio(ink, behind) < 4.5 {
		ink = readableInk(behind, "FFFFFF", "000000")
	}
	return ink
}

func covers(piece Artwork, frame Frame) bool {
	// Two thirds of the frame is enough for the piece to be what shows through.
	overlapX := math.Min(float64(piece.X+piece.Width), float64(frame.X+frame.Width)) - math.Max(float64(piece.X), float64(frame.X))
	overlapY := math.Min(float64(piece.Y+piece.Height), float64(frame.Y+frame.Height)) - math.Max(float64(piece.Y), float64(frame.Y))
	if overlapX <= 0 || overlapY <= 0 {
		return false
	}
	frameArea := float64(frame.Width) * float64(frame.Height)
	return frameArea > 0 && overlapX*overlapY/frameArea >= 0.66
}
