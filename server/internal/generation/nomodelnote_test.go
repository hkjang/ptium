package generation

import (
	"context"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// A deployment with no model writes a frame, and says so.
//
// Everything else this product does tells the person what it did: an import
// says what it dropped, a short deck says why it is short, a model-written deck
// says which figures it invented and removed. The one place it wrote
// scaffolding — lines naming what belongs there, "6개월 안에 확인할 지표와 목표" —
// it said nothing at all, and the deck came back with no notes and no warnings.
func TestADeckWrittenWithoutAModelSaysSo(t *testing.T) {
	template := Template{ID: "t", Name: "T", Manifest: builtinManifest(t)}
	for _, language := range []string{"ko", "en", "ja", "zh"} {
		generator := New(testSettings{"ai.provider": "fallback"})
		deck, err := generator.Generate(context.Background(), model.Presentation{
			Title: "점검", Prompt: "사내 데이터 품질 개선 방안을 정리해줘",
			Language: language, RequestedSlideCount: 6,
		}, model.Profile{}, template)
		if err != nil {
			t.Fatalf("[%s] Generate: %v", language, err)
		}
		if len(deck.Notes) == 0 {
			t.Errorf("[%s] a deck written without a model said nothing to the person who asked for it", language)
			continue
		}
		said := strings.Join(deck.Notes, " ")
		if !mentionsTheFrame(language, said) {
			t.Errorf("[%s] the note does not say what came back: %q", language, said)
		}
	}
}

func mentionsTheFrame(language, said string) bool {
	switch language {
	case "ko":
		return strings.Contains(said, "모델이 없어") && strings.Contains(said, "뼈대")
	case "en":
		return strings.Contains(said, "no AI model connected") && strings.Contains(said, "frame")
	case "ja":
		return strings.Contains(said, "接続されていない")
	case "zh":
		return strings.Contains(said, "未连接")
	}
	return false
}

func builtinManifest(t *testing.T) pptx.Manifest {
	t.Helper()
	data, err := pptx.BuiltinTemplate("slate-classic")
	if err != nil {
		t.Fatalf("BuiltinTemplate: %v", err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatalf("AnalyzeBytes: %v", err)
	}
	return manifest
}
