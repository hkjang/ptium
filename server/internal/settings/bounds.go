package settings

import (
	"encoding/json"
	"strings"
)

// What a setting is honoured at.
//
// Every one of these values is read back with a bound already applied: a
// timeout outside 10–3600 seconds is ignored and the default used, a repair
// count above ten likewise, a flag that is not a boolean falls back to true.
// The API took any of them and answered 200, stored it, and showed it back on
// the settings screen — so an administrator could read "생성 후 자동 수정 500"
// off their own deployment while it ran three passes. A setting that will not
// be honoured is refused here instead, with the range said out loud.
//
// The readers use these same bounds, so the two cannot drift.

// Range is the closed interval a numeric setting is honoured in.
type Range struct{ Low, High int }

// Holds reports whether a value is one this deployment will act on.
func (r Range) Holds(value int) bool { return value >= r.Low && value <= r.High }

// Numbers are the numeric settings and the range each is honoured in.
var Numbers = map[string]Range{
	"ai.timeout_seconds":             {10, 3600},
	"ai.max_output_tokens":           {500, 32000},
	"generation.repair_passes":       {0, 10},
	"generation.default_slide_count": {1, 50},
	"generation.max_slides":          {1, 50},
	// An upload is bounded by what the reader will open at all, which is
	// pptx.MaxPackageBytes — 64 MiB. Asking for more is asking for something
	// that will not happen. A test keeps the two the same.
	"generation.max_template_mb": {1, 64},
}

// Words are the settings whose value must be one of a few words.
var Words = map[string][]string{
	"ai.provider":  {"fallback", "openai", "openai-compatible"},
	"ai.reasoning": {"auto", "off", "on"},
}

// Flags are the settings read as true or false, and nothing else.
var Flags = map[string]bool{
	"generation.outline_pass":       true,
	"generation.allow_user_uploads": true,
}

// IsWord reports whether value is one of the words a setting is honoured at.
func IsWord(key, value string) bool {
	for _, word := range Words[key] {
		if strings.EqualFold(strings.TrimSpace(value), word) {
			return true
		}
	}
	return false
}

// Honoured reports whether a stored value is one this deployment will act on.
// A key with no bound at all is honoured as written.
func Honoured(key string, value json.RawMessage) bool {
	if bounds, ok := Numbers[key]; ok {
		var number int
		return json.Unmarshal(value, &number) == nil && bounds.Holds(number)
	}
	if _, ok := Flags[key]; ok {
		var flag bool
		return json.Unmarshal(value, &flag) == nil
	}
	if _, ok := Words[key]; ok {
		var word string
		return json.Unmarshal(value, &word) == nil && IsWord(key, word)
	}
	return true
}
