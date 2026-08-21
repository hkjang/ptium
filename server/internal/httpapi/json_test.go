package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
)

func TestDecodeJSONRejectsTrailingContent(t *testing.T) {
	for _, body := range []string{`{"name":"ok"} {}`, `{"name":"ok"} trailing`} {
		request := httptest.NewRequest("POST", "/", strings.NewReader(body))
		response := httptest.NewRecorder()
		var target struct {
			Name string `json:"name"`
		}
		if decodeJSON(response, request, &target) {
			t.Fatalf("decodeJSON accepted trailing content %q", body)
		}
		if response.Code != 400 {
			t.Fatalf("status = %d, want 400", response.Code)
		}
	}
}

func TestDecodeJSONAcceptsOneValue(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"ok"}`))
	response := httptest.NewRecorder()
	var target struct {
		Name string `json:"name"`
	}
	if !decodeJSON(response, request, &target) || target.Name != "ok" {
		t.Fatalf("valid JSON was rejected: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCanvasLayerSurvivesASourceApply(t *testing.T) {
	object := `{"type":"template","fields":{"title":[{"text":"A"}]},` +
		`"elements":[{"id":"e1","kind":"text","x":10,"y":10,"width":20,"height":10,"text":"손으로 둔 상자"}],` +
		`"frames":{"body":{"x":5,"y":40,"width":60,"height":30}}}`
	stored := []model.Slide{
		{Title: "첫 장", Content: json.RawMessage(object)},
		{Title: "둘째 장", Content: json.RawMessage(`{"type":"template"}`)},
	}
	// Editing the words in the code view keeps the slide count, so objects stay
	// with the slide they were placed on.
	compiled := []model.Slide{
		{Title: "첫 장, 고쳐 씀", Content: json.RawMessage(`{"type":"template","fields":{"title":[{"text":"B"}]}}`)},
		{Title: "둘째 장", Content: json.RawMessage(`{"type":"template"}`)},
	}
	carried := deck.Decode(carryCanvasLayerAcross(stored, compiled)[0].Content)
	if len(carried.Elements) != 1 || carried.Elements[0].ID != "e1" {
		t.Fatalf("the placed object was lost: %+v", carried.Elements)
	}
	if frame, ok := carried.Frames["body"]; !ok || frame.Y != 40 {
		t.Fatalf("the moved region was lost: %+v", carried.Frames)
	}
	if carried.Fields["title"][0].Text != "B" {
		t.Fatalf("the words are the ones the source produced: %+v", carried.Fields)
	}

	// A deck that gained a slide keeps objects only where the title still
	// identifies the slide they were on.
	grown := []model.Slide{
		{Title: "새 장", Content: json.RawMessage(`{"type":"template"}`)},
		{Title: "첫 장", Content: json.RawMessage(`{"type":"template"}`)},
		{Title: "둘째 장", Content: json.RawMessage(`{"type":"template"}`)},
	}
	result := carryCanvasLayerAcross(stored, grown)
	if len(deck.Decode(result[0].Content).Elements) != 0 {
		t.Fatal("a new slide must not inherit another slide's objects")
	}
	if len(deck.Decode(result[1].Content).Elements) != 1 {
		t.Fatal("the slide that kept its title keeps its objects")
	}
}
