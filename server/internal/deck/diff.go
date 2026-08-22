// Package deck ... (this file) says what changed between two versions of a deck.
package deck

import (
	"strings"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// SlideChange is what happened to one slide between two versions.
//
// The unit is the slide, because that is the unit a person thinks in: "the cost
// slide changed" is the answer to "what did I change", and a character-level
// diff of stored JSON is not. Within a changed slide the lines that came and
// went are listed as they are written in the deck's own language, which is the
// same text the editor shows.
type SlideChange struct {
	// Kind is added, removed, changed or moved.
	Kind string `json:"kind"`
	// Position is where the slide sits now, or where it sat if it is gone.
	Position int `json:"position"`
	// From is where it sat before, for a slide that moved.
	From  int    `json:"from,omitempty"`
	Title string `json:"title"`
	// Added and Removed are the lines of deck source that appeared and went.
	Added   []string `json:"added,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

// Compare reports what changed between two versions of a deck.
//
// The hard part is deciding which slide is which. Three readings, in order:
// the same id is the same slide, because the editor keeps a slide's id across
// an edit; failing that the same title is the same slide, because a rewritten
// cost slide is still the cost slide — and applying deck source rebuilds every
// slide with a new id, so without this every deck would read as wholly
// replaced; and what is left over is matched in the order it appears, which is
// the honest reading of "the third slide changed".
func Compare(before, after []model.Slide, manifest pptx.Manifest) []SlideChange {
	pairs := matchSlides(before, after)
	seen := map[int]bool{}
	var changes []SlideChange
	for index, slide := range after {
		position := index + 1
		source, matched := pairs[index]
		if !matched {
			changes = append(changes, SlideChange{Kind: "added", Position: position,
				Title: slideTitle(slide), Added: slideLines(slide, manifest)})
			continue
		}
		seen[source] = true
		previous := before[source]
		added, removed := lineDifference(slideLines(previous, manifest), slideLines(slide, manifest))
		switch {
		case len(added) == 0 && len(removed) == 0:
			if previous.Position != 0 && previous.Position != position {
				changes = append(changes, SlideChange{Kind: "moved", Position: position,
					From: previous.Position, Title: slideTitle(slide)})
			}
		default:
			change := SlideChange{Kind: "changed", Position: position,
				Title: slideTitle(slide), Added: added, Removed: removed}
			if previous.Position != 0 && previous.Position != position {
				change.From = previous.Position
			}
			changes = append(changes, change)
		}
	}
	for index, slide := range before {
		if seen[index] {
			continue
		}
		position := slide.Position
		if position == 0 {
			position = index + 1
		}
		changes = append(changes, SlideChange{Kind: "removed", Position: position,
			Title: slideTitle(slide), Removed: slideLines(slide, manifest)})
	}
	return changes
}

// matchSlides pairs each slide of the new version with the slide of the old one
// it came from, by id, then by title, then by what is left in order.
func matchSlides(before, after []model.Slide) map[int]int {
	pairs := map[int]int{}
	taken := map[int]bool{}
	byID := map[string]int{}
	for index, slide := range before {
		if slide.ID != "" {
			byID[slide.ID] = index
		}
	}
	for index, slide := range after {
		if slide.ID == "" {
			continue
		}
		if source, ok := byID[slide.ID]; ok && !taken[source] {
			pairs[index], taken[source] = source, true
		}
	}
	byTitle := map[string][]int{}
	for index, slide := range before {
		if taken[index] {
			continue
		}
		byTitle[slideTitle(slide)] = append(byTitle[slideTitle(slide)], index)
	}
	for index, slide := range after {
		if _, done := pairs[index]; done {
			continue
		}
		title := slideTitle(slide)
		if title == "" {
			continue
		}
		for _, source := range byTitle[title] {
			if !taken[source] {
				pairs[index], taken[source] = source, true
				break
			}
		}
	}
	spare := make([]int, 0, len(before))
	for index := range before {
		if !taken[index] {
			spare = append(spare, index)
		}
	}
	next := 0
	for index := range after {
		if _, done := pairs[index]; done {
			continue
		}
		if next >= len(spare) {
			break
		}
		pairs[index], taken[spare[next]] = spare[next], true
		next++
	}
	return pairs
}

// slideLines is one slide written in the deck's own language, line by line.
func slideLines(slide model.Slide, manifest pptx.Manifest) []string {
	written := Format(model.Presentation{Slides: []model.Slide{slide}}, manifest)
	lines := make([]string, 0, 8)
	for _, line := range strings.Split(written, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

func slideTitle(slide model.Slide) string {
	if title := strings.TrimSpace(slide.Title); title != "" {
		return title
	}
	return ""
}

// lineDifference is what one version has that the other does not.
//
// Not a shortest-edit script: a person reading "what changed" wants the
// sentences that came and went, and an edit script spends its precision on
// where they moved to.
func lineDifference(before, after []string) (added, removed []string) {
	counts := map[string]int{}
	for _, line := range before {
		counts[line]++
	}
	for _, line := range after {
		if counts[line] > 0 {
			counts[line]--
			continue
		}
		added = append(added, line)
	}
	remaining := map[string]int{}
	for _, line := range after {
		remaining[line]++
	}
	for _, line := range before {
		if remaining[line] > 0 {
			remaining[line]--
			continue
		}
		removed = append(removed, line)
	}
	return added, removed
}
