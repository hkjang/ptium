package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/generation"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/store"
)

// Bringing in a deck someone already has.
//
// Ptium's premise is that a company's own template is the design. The other half
// of that premise is the decks already written: last quarter's report, the pitch
// that worked, the introduction everyone copies. Reading one in turns it into
// deck source — text — and from there it is an ordinary Ptium deck: it compiles
// into any template, it is edited as words, and the model can rewrite a slide of
// it.
//
// The words come across; the artwork does not. A photograph placed for a 4:3
// layout cannot be moved into a 16:9 design and be trusted to look right, so the
// import reports what it left behind instead of pretending.

// importPresentation reads an uploaded .pptx into a new deck.
func (s *Server) importPresentation(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	limit := s.maximumTemplateBytes(request.Context())
	data, meta, ok := s.readTemplateUpload(writer, request, limit)
	if !ok {
		return
	}
	pkg, err := pptx.Open(data)
	if err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "not_a_presentation",
			"This file is not a PowerPoint presentation Ptium can read", nil)
		return
	}
	imported := pptx.ReadDeck(pkg)
	if len(imported.Slides) == 0 {
		writeError(writer, request, http.StatusUnprocessableEntity, "empty_presentation",
			"This file has no slides Ptium could read", nil)
		return
	}
	// A picture on most of the slides is the company's logo, and a picture too
	// small to be looked at is a decoration; neither is what the slide is about,
	// and putting either into a picture region would be worse than leaving it
	// out. What is left is stored once — an identical file uploaded twice is the
	// same image — and placed in the new design's own picture region.
	slides := len(imported.Slides)
	seen := map[string]int{}
	for _, slide := range imported.Slides {
		for _, picture := range slide.Pictures {
			seen[pictureKey(picture)]++
		}
	}
	saved := map[string]string{}
	source, warnings := deck.SourceFromImportWithImages(imported, func(picture pptx.ImportedPicture) (string, bool) {
		key := pictureKey(picture)
		if slides > 2 && seen[key]*2 > slides {
			return "", false
		}
		if picture.Area > 0 && picture.Area < 30 {
			return "", false
		}
		if name, ok := saved[key]; ok {
			return name, true
		}
		asset, err := s.store.CreateAsset(request.Context(), user.ID, store.AssetInput{
			Name: importedPictureName(meta.Filename, picture.Name), Data: picture.Data,
		})
		if err != nil {
			return "", false
		}
		saved[key] = asset.Name
		return asset.Name, true
	})

	// The design is chosen, not inherited: someone importing a deck is usually
	// doing it to put it in a different template. Without a choice, their default
	// design stands in, which is the same rule the create screen follows.
	title := strings.TrimSpace(meta.Name)
	if title == "" {
		title = strings.TrimSpace(imported.Title)
	}
	if title == "" {
		title = strings.TrimSuffix(strings.TrimSpace(meta.Filename), ".pptx")
	}
	if title == "" {
		title = "가져온 프레젠테이션"
	}
	profile, _ := s.store.GetProfile(request.Context(), user.ID)
	language := strings.TrimSpace(request.FormValue("language"))
	if language == "" {
		language = "ko"
	}
	presentation := model.Presentation{
		OwnerID: user.ID, Title: title, Language: language,
		Prompt: fmt.Sprintf("%s에서 가져온 덱", firstNonEmpty(meta.Filename, title)),
	}
	if templateID := strings.TrimSpace(request.FormValue("templateId")); templateID != "" {
		presentation.TemplateID = &templateID
	}
	_, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	compiled := generation.CompileSourceWith(source, presentation, profile,
		generation.Template{ID: templateIDOf(presentation), Manifest: manifest},
		s.resolveImage(request, user.ID), s.gridResolver(request, user.ID))
	if len(compiled.Slides) == 0 {
		writeError(writer, request, http.StatusUnprocessableEntity, "empty_presentation",
			"This file has no slides Ptium could read", nil)
		return
	}
	if maximum := s.maximumSlides(request.Context()); len(compiled.Slides) > maximum {
		compiled.Slides = compiled.Slides[:maximum]
		warnings = append(warnings, fmt.Sprintf(
			"이 배포는 한 덱에 %d장까지 허용하므로 그 뒤는 가져오지 않았습니다", maximum))
	}
	warnings = append(warnings, compiled.Warnings...)
	if warnings == nil {
		// An empty list, not a null: a client should not have to handle both to
		// find out that nothing went wrong.
		warnings = []string{}
	}

	created, err := s.store.CreatePresentation(request.Context(), user.ID, store.PresentationInput{
		Title: presentation.Title, Prompt: presentation.Prompt, Language: language,
		SlideCount: len(compiled.Slides), TemplateID: presentation.TemplateID,
	})
	if err != nil {
		s.internalError(writer, request, "presentation_create_failed", err)
		return
	}
	if err := s.store.ReplaceSlidesFromSource(request.Context(), created.ID, user.ID,
		source, compiled.Outline, compiled.Slides, nil); err != nil {
		if errors.Is(err, store.ErrGenerationLimit) {
			writeError(writer, request, http.StatusUnprocessableEntity, "too_many_slides",
				"The file has more slides than this deployment allows", nil)
			return
		}
		s.internalError(writer, request, "presentation_import_failed", err)
		return
	}
	stored, err := s.store.GetPresentation(request.Context(), created.ID, user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "presentation.import", "presentation", created.ID,
		map[string]any{"filename": meta.Filename, "slides": len(compiled.Slides)})
	writeData(writer, request, http.StatusCreated, map[string]any{
		"presentation": stored, "warnings": warnings, "slides": len(compiled.Slides),
	})
}

// pictureKey identifies the same picture wherever it appears.
func pictureKey(picture pptx.ImportedPicture) string {
	digest := sha256.Sum256(picture.Data)
	return hex.EncodeToString(digest[:])
}

// importedPictureName is what the image library will call it: the file it came
// from, and the picture's own name inside that file.
func importedPictureName(filename, name string) string {
	base := strings.TrimSuffix(strings.TrimSpace(filename), ".pptx")
	if base == "" {
		base = "가져온 파일"
	}
	if strings.TrimSpace(name) == "" {
		name = "image.png"
	}
	return base + " · " + name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
