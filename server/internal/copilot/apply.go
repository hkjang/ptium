package copilot

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// Apply performs the commands and returns the deck they produce.
//
// Nothing here rewrites anyone's words. Slides move, join, divide, repeat and
// go — every one of those is an edit with a single right answer, and the deck
// that comes back is the deck someone would have made by hand.
func Apply(slides []model.Slide, commands []Command, weakest func(position int) int) ([]model.Slide, []string, error) {
	result := append([]model.Slide(nil), slides...)
	var notes []string
	for _, command := range commands {
		var err error
		switch command.Kind {
		case KindDelete:
			result, err = removeSlides(result, command.Slides)
		case KindMove:
			result, err = moveSlide(result, command.Slides[0], command.To)
		case KindDuplicate:
			result, err = duplicateSlide(result, command.Slides[0])
		case KindMerge:
			result, notes, err = mergeSlides(result, command.Slides[0], command.Slides[1], notes)
		case KindSplit:
			result, notes, err = splitSlide(result, command.Slides[0], notes)
		case KindTrim:
			result, notes, err = trimDeck(result, command.Count, weakest, notes)
		default:
			err = fmt.Errorf("모르는 명령입니다: %s", command.Kind)
		}
		if err != nil {
			return nil, nil, err
		}
	}
	for index := range result {
		result[index].Position = index + 1
	}
	return result, notes, nil
}

func removeSlides(slides []model.Slide, positions []int) ([]model.Slide, error) {
	drop := map[int]bool{}
	for _, position := range positions {
		drop[position] = true
	}
	kept := make([]model.Slide, 0, len(slides))
	for index, slide := range slides {
		if !drop[index+1] {
			kept = append(kept, slide)
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("덱의 모든 슬라이드를 지울 수는 없습니다")
	}
	return kept, nil
}

func moveSlide(slides []model.Slide, from, to int) ([]model.Slide, error) {
	if from < 1 || from > len(slides) || to < 1 || to > len(slides) {
		return nil, fmt.Errorf("%d번이나 %d번 슬라이드가 없습니다", from, to)
	}
	moved := slides[from-1]
	rest := append([]model.Slide(nil), slides[:from-1]...)
	rest = append(rest, slides[from:]...)
	at := to - 1
	if at > len(rest) {
		at = len(rest)
	}
	result := append([]model.Slide(nil), rest[:at]...)
	result = append(result, moved)
	return append(result, rest[at:]...), nil
}

func duplicateSlide(slides []model.Slide, position int) ([]model.Slide, error) {
	if position < 1 || position > len(slides) {
		return nil, fmt.Errorf("%d번 슬라이드가 없습니다", position)
	}
	copied := slides[position-1]
	// A copy is a new slide: it keeps the words and gets its own identity, or
	// the store would take it for the same row twice.
	copied.ID = ""
	result := append([]model.Slide(nil), slides[:position]...)
	result = append(result, copied)
	return append(result, slides[position:]...), nil
}

// mergeSlides joins the second slide into the first: its points follow, its
// notes follow, and its sources follow — a merged slide that lost half its
// evidence would be worse than two slides.
func mergeSlides(slides []model.Slide, first, second int, notes []string) ([]model.Slide, []string, error) {
	if first < 1 || first > len(slides) || second < 1 || second > len(slides) || first == second {
		return nil, notes, fmt.Errorf("%d번과 %d번을 합칠 수 없습니다", first, second)
	}
	into, from := slides[first-1], slides[second-1]
	target, source := deck.Decode(into.Content), deck.Decode(from.Content)

	points := append(append([]string(nil), target.PrimaryBullets()...), source.PrimaryBullets()...)
	if slot := primarySlot(target); slot != "" {
		paragraphs := make([]pptx.Paragraph, 0, len(points))
		for _, point := range points {
			paragraphs = append(paragraphs, pptx.Paragraph{Text: point})
		}
		target.SetField(slot, paragraphs)
	}
	target.Sources = append(target.Sources, source.Sources...)
	target.Notes = strings.TrimSpace(strings.TrimSpace(target.Notes) + " " + strings.TrimSpace(source.Notes))
	into.Content = target.Encode()
	into.SpeakerNotes = strings.TrimSpace(into.SpeakerNotes + " " + from.SpeakerNotes)
	slides[first-1] = into
	notes = append(notes, fmt.Sprintf("%d번의 요점과 노트, 출처를 %d번으로 옮겼습니다", second, first))
	return removeAt(slides, second), notes, nil
}

// splitSlide divides a slide's points in half, keeping the title on both and
// marking the second so a reader knows it continues.
func splitSlide(slides []model.Slide, position int, notes []string) ([]model.Slide, []string, error) {
	if position < 1 || position > len(slides) {
		return nil, notes, fmt.Errorf("%d번 슬라이드가 없습니다", position)
	}
	original := slides[position-1]
	content := deck.Decode(original.Content)
	points := content.PrimaryBullets()
	if len(points) < 2 {
		return nil, notes, fmt.Errorf("%d번은 요점이 하나뿐이라 나눌 수 없습니다", position)
	}
	slot := primarySlot(content)
	if slot == "" {
		return nil, notes, fmt.Errorf("%d번에는 나눌 본문이 없습니다", position)
	}
	half := (len(points) + 1) / 2
	write := func(base model.Slide, take []string, title string) model.Slide {
		copied := deck.Decode(base.Content)
		paragraphs := make([]pptx.Paragraph, 0, len(take))
		for _, point := range take {
			paragraphs = append(paragraphs, pptx.Paragraph{Text: point})
		}
		copied.SetField(slot, paragraphs)
		base.Content = copied.Encode()
		base.Title = title
		return base
	}
	head := write(original, points[:half], original.Title)
	tail := original
	tail.ID = ""
	tail = write(tail, points[half:], strings.TrimSpace(original.Title+" (계속)"))
	result := append([]model.Slide(nil), slides[:position-1]...)
	result = append(result, head, tail)
	result = append(result, slides[position:]...)
	notes = append(notes, fmt.Sprintf("%d번을 %d개와 %d개의 요점으로 나눴습니다", position, half, len(points)-half))
	return result, notes, nil
}

// trimDeck brings a deck down to a length by dropping its weakest slides —
// measured, not guessed — and never the first or the last, which are the deck's
// own opening and close.
func trimDeck(slides []model.Slide, count int, weakest func(position int) int, notes []string) ([]model.Slide, []string, error) {
	if count < 1 {
		return nil, notes, fmt.Errorf("남길 장수를 알 수 없습니다")
	}
	if count >= len(slides) {
		return slides, notes, nil
	}
	if count < 2 && len(slides) > 2 {
		return nil, notes, fmt.Errorf("표지와 마무리는 남겨야 하므로 %d장까지만 줄일 수 있습니다", 2)
	}
	type ranked struct {
		position int
		score    int
	}
	order := make([]ranked, 0, len(slides))
	for position := 2; position < len(slides); position++ {
		score := 100
		if weakest != nil {
			score = weakest(position)
		}
		order = append(order, ranked{position: position, score: score})
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].score < order[j].score })
	drop := map[int]bool{}
	for _, entry := range order {
		if len(slides)-len(drop) <= count {
			break
		}
		drop[entry.position] = true
	}
	kept := make([]model.Slide, 0, count)
	dropped := make([]int, 0, len(drop))
	for index, slide := range slides {
		if drop[index+1] {
			dropped = append(dropped, index+1)
			continue
		}
		kept = append(kept, slide)
	}
	sort.Ints(dropped)
	notes = append(notes, fmt.Sprintf("측정 점수가 가장 낮은 %s번을 뺐습니다", join(dropped)))
	return kept, notes, nil
}

func removeAt(slides []model.Slide, position int) []model.Slide {
	result := append([]model.Slide(nil), slides[:position-1]...)
	return append(result, slides[position:]...)
}

// primarySlot is the region a slide keeps its points in.
func primarySlot(content deck.Content) string {
	best := ""
	for slot, paragraphs := range content.Fields {
		if slot == pptx.SlotTitle || slot == pptx.SlotSubtitle || len(paragraphs) == 0 {
			continue
		}
		if best == "" || slot < best {
			best = slot
		}
	}
	return best
}
