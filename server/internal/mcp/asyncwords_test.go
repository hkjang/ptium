package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// A caller told only that generation was "queued" fetches the deck at once,
// finds it empty, and reports a failure that has not happened. The tools are
// the only documentation an agent ever reads, so the two-step, asynchronous
// shape has to be in them.
func TestTheToolsSayGenerationIsAsynchronous(t *testing.T) {
	raw, err := json.Marshal(toolDefinitions())
	if err != nil {
		t.Fatalf("tools: %v", err)
	}
	said := map[string][]string{
		"ptium.generate_presentation": {"poll", "completed", "failed"},
		"ptium.get_presentation":      {"status", "no slides yet"},
		"ptium.create_presentation":   {"no slides"},
	}
	var tools []map[string]any
	if err := json.Unmarshal(raw, &tools); err != nil {
		t.Fatalf("read tools: %v", err)
	}
	for _, tool := range tools {
		wanted, ok := said[toString(tool["name"])]
		if !ok {
			continue
		}
		description := toString(tool["description"])
		for _, word := range wanted {
			if !strings.Contains(strings.ToLower(description), strings.ToLower(word)) {
				t.Errorf("%s does not say %q: %q", tool["name"], word, description)
			}
		}
	}
}

func toString(value any) string {
	said, _ := value.(string)
	return said
}
