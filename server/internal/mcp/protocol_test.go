package mcp

import (
	"encoding/json"
	"testing"
)

func TestParseRequestPreservesSupportedIDs(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{name: "string", id: `"abc"`},
		{name: "integer", id: `42`},
		{name: "decimal", id: `4.2`},
		{name: "null", id: `null`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, validationError := parseRequest(json.RawMessage(`{"jsonrpc":"2.0","id":` + test.id + `,"method":"ping"}`))
			if validationError != nil {
				t.Fatalf("parseRequest() error = %#v", validationError)
			}
			if !request.HasID || string(request.ID) != test.id {
				t.Fatalf("id = %q, hasID = %v", request.ID, request.HasID)
			}
		})
	}
}

func TestParseRequestRejectsStructuredAndBooleanIDs(t *testing.T) {
	for _, id := range []string{`true`, `{}`, `[]`} {
		_, validationError := parseRequest(json.RawMessage(`{"jsonrpc":"2.0","id":` + id + `,"method":"ping"}`))
		if validationError == nil || validationError.code != -32600 {
			t.Fatalf("id %s validation error = %#v", id, validationError)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	for _, offset := range []int{0, 1, 50, 1_000_000} {
		encoded := encodeCursor(offset)
		decoded, err := decodeCursor(encoded)
		if err != nil || decoded != offset {
			t.Fatalf("cursor %d => %q => %d, %v", offset, encoded, decoded, err)
		}
	}
	for _, cursor := range []string{"bad", encodeCursor(1_000_001)} {
		if _, err := decodeCursor(cursor); err == nil {
			t.Fatalf("decodeCursor(%q) error = nil", cursor)
		}
	}
}
