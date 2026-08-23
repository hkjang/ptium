package generation

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// compose validates model output against the template and repairs what it can.
// A language model will occasionally name a layout that does not exist, write
// into a slot the layout does not have, or overrun a text budget; none of that
// should fail a generation the user is waiting for, but none of it may reach
// the exported file either.
func compose(request writingRequest, written writtenDeck) (Deck, error) {
	manifest := request.Template.Manifest
	requested := request.Presentation.RequestedSlideCount
	if requested < 1 {
		requested = len(written.Slides)
	}
	var warnings []string
	// A model that returns the wrong number of slides is a nuisance, not a
	// failure. Extra slides are dropped from the end, where a deck's weakest
	// material sits; a short deck is delivered short and says so. Failing here
	// would throw away good work someone is waiting for.
	if actual := len(written.Slides); actual != requested {
		warnings = append(warnings,
			fmt.Sprintf("the model wrote %d slides for a request of %d", actual, requested))
		if actual > requested {
			written.Slides = written.Slides[:requested]
		}
	}

	result := Deck{Warnings: warnings}
	outline := make([]map[string]any, 0, requested)
	for index, slide := range written.Slides {
		layout := resolveWrittenLayout(manifest, request.Plan, slide, index, requested)
		content := deck.Content{Type: deck.ContentType, LayoutID: layout.ID, Accent: profileAccent(request.Profile, index)}

		title := strings.TrimSpace(slide.Title)
		if title == "" {
			title = firstLine(slide.Fields[pptx.SlotTitle])
		}
		if title == "" && request.Plan != nil && index < len(request.Plan.Slides) {
			title = strings.TrimSpace(request.Plan.Slides[index].Headline)
		}
		if title == "" {
			title = fmt.Sprintf("%s %d", request.Presentation.Title, index+1)
		}

		// A component claims its slot before prose, because the model is told to
		// use one or the other and a slot cannot hold both.
		claimed := map[string]bool{}
		for slot, block := range slide.Blocks {
			placeholder, ok := layout.Slot(canonicalSlot(slot))
			if !ok || !placeholder.AcceptsText() || placeholder.Slot == pptx.SlotTitle {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("slide %d: layout %q has no region for a %s and it was dropped",
						index+1, layout.Name, block.Kind))
				continue
			}
			sanitized, usable := sanitizeBlock(block, placeholder)
			if !usable {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("slide %d: the %s did not have enough room and was dropped", index+1, block.Kind))
				continue
			}
			content.SetBlock(placeholder.Slot, sanitized)
			claimed[placeholder.Slot] = true
		}

		assigned := map[string]bool{}
		for slot, raw := range slide.Fields {
			placeholder, ok := layout.Slot(canonicalSlot(slot))
			if !ok || !placeholder.AcceptsText() || claimed[placeholder.Slot] {
				// Text with nowhere to go used to disappear here without a word.
				// The other way of writing a deck says so; both should.
				if lines := parseParagraphs(raw); len(lines) > 0 {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("slide %d: layout %q has no free %s region, so %q was dropped",
							index+1, layout.Name, canonicalSlot(slot), truncate(lines[0].Text, 40)))
				}
				continue
			}
			paragraphs := fitParagraphs(parseParagraphs(raw), placeholder)
			if len(paragraphs) == 0 {
				continue
			}
			content.SetField(placeholder.Slot, paragraphs)
			assigned[placeholder.Slot] = true
		}

		if placeholder, ok := layout.Slot(pptx.SlotTitle); ok {
			content.SetField(pptx.SlotTitle, fitParagraphs([]pptx.Paragraph{{Text: title}}, placeholder))
		}
		// A slide with a body slot and nothing in it is worse than no slide, so
		// fall back to the planned key points before giving up on the slot.
		if !hasAnyBody(content) && len(content.Blocks) == 0 && request.Plan != nil && index < len(request.Plan.Slides) {
			if placeholder, ok := firstBodyPlaceholder(layout); ok {
				paragraphs := make([]pptx.Paragraph, 0, len(request.Plan.Slides[index].KeyPoints))
				for _, point := range request.Plan.Slides[index].KeyPoints {
					paragraphs = append(paragraphs, pptx.Paragraph{Text: point})
				}
				content.SetField(placeholder.Slot, fitParagraphs(paragraphs, placeholder))
			}
		}

		subtitle := ""
		if paragraphs, ok := content.Fields[pptx.SlotSubtitle]; ok && len(paragraphs) > 0 {
			subtitle = paragraphs[0].Text
		}
		result.Slides = append(result.Slides, model.Slide{
			Position:     index + 1,
			Title:        truncate(title, 200),
			Subtitle:     truncate(subtitle, 300),
			Content:      content.Encode(),
			SpeakerNotes: truncate(strings.TrimSpace(slide.Notes), 4000),
			Layout:       layout.Role,
			LayoutID:     layout.ID,
		})
		outline = append(outline, map[string]any{"position": index + 1, "title": title, "layout": layout.Name, "role": layout.Role})
	}
	result.Outline, _ = json.Marshal(outline)
	// The deck is written back out as source, so a deck a model produced is as
	// editable as one written by hand.
	result.Source = deck.Format(model.Presentation{Slides: result.Slides, Language: request.Presentation.Language}, manifest)
	return result, nil
}

// resolveWrittenLayout finds the template layout a written slide belongs to,
// falling back through the plan's intent and the slide's position.
func resolveWrittenLayout(manifest pptx.Manifest, plan *deckPlan, slide writtenSlide, index, total int) pptx.Layout {
	candidates := []string{slide.LayoutID}
	roles := []string{slide.Role}
	if plan != nil && index < len(plan.Slides) {
		candidates = append(candidates, plan.Slides[index].LayoutID)
		roles = append(roles, plan.Slides[index].Role)
	}
	for _, candidate := range candidates {
		if layout, ok := matchLayout(manifest, candidate); ok {
			return layout
		}
	}
	for _, role := range roles {
		if strings.TrimSpace(role) == "" {
			continue
		}
		if layout, ok := manifest.LayoutForRole(strings.TrimSpace(role)); ok {
			return layout
		}
	}
	if layout, ok := manifest.LayoutForRole(deck.RoleForLegacyLayout("", index, total)); ok {
		return layout
	}
	return manifest.Layouts[0]
}

// matchLayout accepts an exact id, a case-insensitive id, or a layout name, so
// a model that echoes the human-readable label still lands on the right slide.
func matchLayout(manifest pptx.Manifest, candidate string) (pptx.Layout, bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return pptx.Layout{}, false
	}
	if layout, ok := manifest.Layout(candidate); ok {
		return layout, true
	}
	lowered := strings.ToLower(candidate)
	for _, layout := range manifest.Layouts {
		if strings.ToLower(layout.ID) == lowered || strings.ToLower(layout.Name) == lowered {
			return layout, true
		}
	}
	return pptx.Layout{}, false
}

// canonicalSlot maps the aliases a model reaches for onto real slot names.
func canonicalSlot(slot string) string {
	slot = strings.TrimSpace(slot)
	switch strings.ToLower(slot) {
	case "title", "heading", "headline":
		return pptx.SlotTitle
	case "subtitle", "subhead", "subheading":
		return pptx.SlotSubtitle
	case "body", "content", "bullets", "text":
		return pptx.SlotBody
	case "body2", "content2", "right", "bullets2":
		return "body2"
	case "body3", "content3":
		return "body3"
	case "body4", "content4":
		return "body4"
	}
	return slot
}

// parseParagraphs accepts the three shapes a model realistically emits: a
// plain string, an array of strings, or an array of {text, level} objects.
func parseParagraphs(raw json.RawMessage) []pptx.Paragraph {
	if len(raw) == 0 {
		return nil
	}
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return splitLines(single)
	}
	var lines []string
	if json.Unmarshal(raw, &lines) == nil {
		result := make([]pptx.Paragraph, 0, len(lines))
		for _, line := range lines {
			result = append(result, splitLines(line)...)
		}
		return result
	}
	var structured []struct {
		Text  string `json:"text"`
		Level int    `json:"level"`
	}
	if json.Unmarshal(raw, &structured) == nil {
		result := make([]pptx.Paragraph, 0, len(structured))
		for _, entry := range structured {
			for _, paragraph := range splitLines(entry.Text) {
				paragraph.Level += entry.Level
				result = append(result, paragraph)
			}
		}
		return result
	}
	return nil
}

// splitLines normalizes one incoming string into paragraphs, stripping the
// markdown bullet characters models add out of habit.
func splitLines(value string) []pptx.Paragraph {
	var result []pptx.Paragraph
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		level := 0
		for strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "  ") {
			if strings.HasPrefix(line, "\t") {
				line = line[1:]
			} else {
				line = line[2:]
			}
			level++
		}
		trimmed := strings.TrimSpace(line)
		for _, marker := range []string{"- ", "* ", "• ", "– ", "· "} {
			if strings.HasPrefix(trimmed, marker) {
				trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, marker))
				break
			}
		}
		trimmed = strings.Trim(trimmed, "*")
		if strings.TrimSpace(trimmed) == "" {
			continue
		}
		result = append(result, pptx.Paragraph{Text: strings.TrimSpace(trimmed), Level: level})
	}
	return result
}

// fitParagraphs keeps copy inside the slot's budget. Mild overflow is left to
// the renderer's autofit; only text that would still not fit after shrinking
// is trimmed, and always at a word boundary.
// fitParagraphs and sanitizeBlock live in the pptx package, which owns the
// placeholder and component models; these keep the local call sites short.
func fitParagraphs(paragraphs []pptx.Paragraph, placeholder pptx.Placeholder) []pptx.Paragraph {
	return pptx.FitParagraphs(paragraphs, placeholder, "")
}

func sanitizeBlock(block pptx.Block, placeholder pptx.Placeholder) (pptx.Block, bool) {
	return pptx.SanitizeBlock(block, placeholder)
}

func firstLine(raw json.RawMessage) string {
	paragraphs := parseParagraphs(raw)
	if len(paragraphs) == 0 {
		return ""
	}
	return paragraphs[0].Text
}

func hasAnyBody(content deck.Content) bool {
	for slot := range content.Fields {
		if slot != pptx.SlotTitle && slot != pptx.SlotSubtitle {
			return true
		}
	}
	return false
}

func firstBodyPlaceholder(layout pptx.Layout) (pptx.Placeholder, bool) {
	for _, placeholder := range layout.BodySlots() {
		return placeholder, true
	}
	if placeholder, ok := layout.Slot(pptx.SlotSubtitle); ok {
		return placeholder, true
	}
	return pptx.Placeholder{}, false
}

func knownBlockKind(kind string) bool {
	for _, candidate := range pptx.BlockKinds() {
		if candidate == kind {
			return true
		}
	}
	return false
}
