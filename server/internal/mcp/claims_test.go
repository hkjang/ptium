package mcp

import (
	"strings"
	"testing"
)

// A tool's description is what an agent acts on, so it must describe the answer
// the tool actually gives.
//
// The one that was wrong: list_templates said "with their layouts", and the
// answer carries layoutCount and no layouts at all. An agent told otherwise
// looks for something that is not there — and what creating against a
// particular design actually needs is the id, which the description did not
// mention.
func TestTheTemplateToolDescribesWhatItReturns(t *testing.T) {
	description := ""
	for _, tool := range toolDefinitions() {
		if tool.Name == "ptium.list_templates" {
			description = tool.Description
		}
	}
	if description == "" {
		t.Fatal("ptium.list_templates is not offered at all")
	}
	if strings.Contains(description, "with their layouts") {
		t.Errorf("the description promises the layouts themselves: %q", description)
	}
	for _, want := range []string{"templateId", "layoutCount"} {
		if !strings.Contains(description, want) {
			t.Errorf("the description does not mention %s: %q", want, description)
		}
	}
}

// The deck list is ordered by last change, not by when a deck was made — a deck
// made last week and edited today sits above one made an hour ago. "newest
// first" reads as creation order, which is not what the list gives.
func TestTheListToolDescribesTheOrderItGives(t *testing.T) {
	description := ""
	for _, tool := range toolDefinitions() {
		if tool.Name == "ptium.list_presentations" {
			description = tool.Description
		}
	}
	if description == "" {
		t.Fatal("ptium.list_presentations is not offered at all")
	}
	if strings.Contains(description, "newest first") {
		t.Errorf("the description says newest first and the list is ordered by last change: %q", description)
	}
	if !strings.Contains(description, "most recently changed") {
		t.Errorf("the description does not say what the order is: %q", description)
	}
}
