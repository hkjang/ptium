package store

import "strings"

// likePattern turns what somebody typed into a LIKE pattern that means what
// they typed.
//
// "%"+text+"%" hands the user's own characters to LIKE, where % stands for
// anything and _ for any one character. Searching for "%" then lists the whole
// library, "50%" matches "500만원" as readily as "50%", and a name with an
// underscore in it — metadata_demo, AI_Coding_ROI — cannot be searched for
// exactly. The wildcards are the query's, not the reader's.
//
// Every LIKE using one of these must say ESCAPE '\', which is what
// likeEscape spells.
func likePattern(text string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(text))
	return "%" + escaped + "%"
}

// likeEscape is the clause that makes the escaping above take effect.
const likeEscape = ` ESCAPE '\'`
