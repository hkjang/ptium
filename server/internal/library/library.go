// Package library decides when a deck should use a slide someone already made
// rather than write a new one.
//
// A company has slides that must not vary: the company introduction, the org
// chart, the security architecture, the standard disclaimer. Someone has
// already written them, agreed them and put them in the slide library — and
// then every generated deck writes its own version anyway, which is how a
// company's decks drift apart.
//
// So before a deck is compiled, each slide's title is looked up in the library,
// and a slide that is clearly the same slide is replaced by the registered one.
// The matching is deliberately strict and deterministic: a wrong substitution
// replaces what the author asked for with something else, which is worse than
// writing a new slide. No model is involved — "회사 소개" matching a slide named
// "회사 소개" is not a judgement call.
package library

import (
	"strings"
	"unicode"
)

// Entry is one slide someone registered.
type Entry struct {
	ID   string
	Name string
	// Aliases are the other names it answers to — its tags.
	Aliases []string
	// Source is the slide, in deck source.
	Source string
}

// Used records that a registered slide was put into a deck.
type Used struct {
	ID    string
	Name  string
	Title string
}

// Substitute replaces the slides of a deck whose titles name a registered
// slide, and reports which were used.
//
// The first slide is never replaced: it is the deck's cover, it carries the
// deck's own title, and a library slide in its place would rename the deck.
func Substitute(source string, entries []Entry) (string, []Used) {
	if strings.TrimSpace(source) == "" || len(entries) == 0 {
		return source, nil
	}
	chunks := splitSlides(source)
	if len(chunks) < 2 {
		return source, nil
	}
	var used []Used
	for index := 1; index < len(chunks); index++ {
		title := titleOf(chunks[index])
		if title == "" {
			continue
		}
		entry, ok := Match(title, entries)
		if !ok {
			continue
		}
		replacement := strings.TrimRight(entry.Source, "\n")
		if strings.TrimSpace(replacement) == "" {
			continue
		}
		chunks[index] = replacement + "\n"
		used = append(used, Used{ID: entry.ID, Name: entry.Name, Title: title})
	}
	if len(used) == 0 {
		return source, nil
	}
	return strings.Join(chunks, "\n"), used
}

// Match finds the registered slide a title names, if one clearly does.
func Match(title string, entries []Entry) (Entry, bool) {
	wanted := normalize(title)
	if len(wanted) < 2 {
		return Entry{}, false
	}
	best, bestScore := Entry{}, 0
	for _, entry := range entries {
		for _, name := range append([]string{entry.Name}, entry.Aliases...) {
			candidate := normalize(name)
			if len(candidate) < 2 {
				continue
			}
			score := 0
			switch {
			case candidate == wanted:
				score = 100
			case strings.Contains(wanted, candidate) && len(candidate)*2 >= len(wanted):
				// "회사 소개" registered, "회사 소개 (2026)" asked for: the same slide,
				// said at more length. Half the title has to be the name, or "계획"
				// would match every plan slide in every deck.
				score = 80
			}
			if score > bestScore {
				best, bestScore = entry, score
			}
		}
	}
	return best, bestScore > 0
}

// normalize reduces a name to what it says: letters and digits, folded, with
// spacing and punctuation dropped, so "회사 소개" and "회사소개" are one name.
func normalize(value string) string {
	var builder strings.Builder
	for _, symbol := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(symbol) || unicode.IsDigit(symbol) {
			builder.WriteRune(symbol)
		}
	}
	return builder.String()
}

// splitSlides cuts deck source into slides, keeping each slide's own text.
func splitSlides(source string) []string {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	var chunks []string
	var current strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "# ") && current.Len() > 0 {
			chunks = append(chunks, strings.TrimRight(current.String(), "\n")+"\n")
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if strings.TrimSpace(current.String()) != "" {
		chunks = append(chunks, strings.TrimRight(current.String(), "\n")+"\n")
	}
	return chunks
}

// titleOf is the title a slide's source states.
func titleOf(chunk string) string {
	for _, line := range strings.Split(chunk, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		}
	}
	return ""
}
