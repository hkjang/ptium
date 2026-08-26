package settings

import "testing"

// A bound that says one thing and is read as another is how a setting gets
// stored, shown back, and quietly not honoured.
func TestEverySettingWithABoundSaysIt(t *testing.T) {
	t.Parallel()
	for key, bounds := range Numbers {
		if bounds.Low > bounds.High {
			t.Errorf("%s is honoured between %d and %d", key, bounds.Low, bounds.High)
		}
		if bounds.Holds(bounds.Low-1) || bounds.Holds(bounds.High+1) {
			t.Errorf("%s holds a value outside its own range", key)
		}
		if !bounds.Holds(bounds.Low) || !bounds.Holds(bounds.High) {
			t.Errorf("%s does not hold its own ends", key)
		}
	}
	// An upload above what the reader will open at all cannot be honoured.
	if got := Numbers["generation.max_template_mb"].High; got != 64 {
		t.Errorf("templates are accepted to %d MiB; the reader opens 64 MiB", got)
	}
	for key, words := range Words {
		if len(words) == 0 {
			t.Errorf("%s is honoured at no value at all", key)
		}
		for _, word := range words {
			if !IsWord(key, word) || !IsWord(key, " "+word+" ") {
				t.Errorf("%s does not accept its own value %q", key, word)
			}
		}
		if IsWord(key, "something-nobody-implements") {
			t.Errorf("%s accepts a value it does not act on", key)
		}
	}
}
