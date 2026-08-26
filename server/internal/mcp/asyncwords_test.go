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

// An agent's first job is finding the right deck. An account holds thousands,
// and a tool that can only page through them cannot find one: the list tool
// says it takes a search, and passes it on.
func TestTheListToolCanBeAskedForOneDeck(t *testing.T) {
	handler := newTestHandler(t, &fakeOperations{})
	response := postRPC(t, handler, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`, nil)
	listed, _ := json.Marshal(decodeObject(t, response)["result"])
	if !strings.Contains(string(listed), `"q"`) {
		t.Errorf("the list tool does not offer a search: %s", listed)
	}

	// And what is asked for reaches the operation rather than being dropped.
	operations := &fakeOperations{}
	handler = newTestHandler(t, operations)
	postRPC(t, handler,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ptium.list_presentations","arguments":{"q":"클라우드"}}}`, nil)
	if operations.searched != "클라우드" {
		t.Errorf("the search reached the store as %q", operations.searched)
	}
}

// What a caller is told about how long to wait.
//
// The tools used to say a deck takes "seconds to a few minutes". Since this
// product stopped taking slow generations away from the workers writing them, a
// deployment may allow an hour per model call and ten repair passes on top — so
// a caller that follows a clock reports a failure that has not happened, over a
// deck that is going perfectly well.
func TestTheToolsDoNotPromiseATimeThisProductCannotKeep(t *testing.T) {
	t.Parallel()
	var generate, read string
	for _, tool := range toolDefinitions() {
		switch tool.Name {
		case "ptium.generate_presentation":
			generate = tool.Description
		case "ptium.get_presentation":
			read = tool.Description
		}
	}
	if generate == "" || read == "" {
		t.Fatal("the tools a caller generates and polls with are not both offered")
	}
	for _, promise := range []string{"a few minutes", "a minute or three", "seconds to a few minutes"} {
		if strings.Contains(strings.ToLower(generate), promise) {
			t.Errorf("generate promises %q", promise)
		}
	}
	for _, said := range []string{"poll", "queued", "generating"} {
		if !strings.Contains(strings.ToLower(generate), said) {
			t.Errorf("generate does not say %q, so a caller has to guess", said)
		}
	}
	// And the tool it polls with still names every state it can meet.
	for _, state := range []string{"draft", "queued", "generating", "completed", "failed"} {
		if !strings.Contains(read, state) {
			t.Errorf("get_presentation does not name the state %q", state)
		}
	}
}
