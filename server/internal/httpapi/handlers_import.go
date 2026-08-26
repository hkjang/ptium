package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/docs"
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
	data, meta, ok := s.readImportUpload(writer, request, limit)
	if !ok {
		return
	}
	pkg, err := pptx.Open(data)
	if err != nil {
		// Not a presentation. It may still be the material for one: the report,
		// the spreadsheet or the notes the deck would have been written from.
		s.importDocument(writer, request, user, meta, data)
		return
	}
	// A deck Ptium exported carries the text it was written from. Reading that
	// back gives the author their components, their citations and their notes
	// exactly as they wrote them; reading the drawing instead turns a process
	// diagram into "1 · 준비 · 범위 확정 · 2 · 이행" and calls it points.
	if source, ok := pptx.DeckSource(pkg); ok {
		title := strings.TrimSpace(request.FormValue("name"))
		if title == "" {
			title = deck.TitleFromSource(source)
		}
		if title == "" {
			title = meta.Filename
		}
		s.storeImportedSource(writer, request, user, meta, title,
			fmt.Sprintf("%s에서 가져온 덱", firstNonEmpty(meta.Filename, title)), source,
			[]string{"Ptium이 만든 파일이라 원본 그대로 가져왔습니다"})
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
	s.storeImportedSource(writer, request, user, meta, title,
		fmt.Sprintf("%s에서 가져온 덱", firstNonEmpty(meta.Filename, title)), source, warnings)
}

// storeImportedSource compiles imported deck source against the chosen template
// and stores it as a new presentation. Both halves of importing — a deck from a
// .pptx and a deck from a document — end here, because from this point they are
// the same thing: deck source looking for a design.
func (s *Server) storeImportedSource(writer http.ResponseWriter, request *http.Request,
	user model.User, meta templateMetadata, title, prompt, source string, warnings []string) {
	profile, _ := s.store.GetProfile(request.Context(), user.ID)
	language := strings.TrimSpace(request.FormValue("language"))
	if language == "" {
		language = "ko"
	}
	presentation := model.Presentation{OwnerID: user.ID, Title: title, Language: language, Prompt: prompt}
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
	// Two kinds of thing to say, and they are not for the same reader. What the
	// import did with the file — "표 1개를 다시 그렸습니다" — is for the person who
	// uploaded it. What the compiler adjusted — 'layout "마무리" has no free body
	// region' — is for whoever is debugging a template, and putting it in front of
	// someone who just wanted their deck back is noise in a language they did not
	// choose.
	if warnings == nil {
		warnings = []string{}
	}
	technical := compiled.Warnings
	if technical == nil {
		technical = []string{}
	}

	// A deck carried in from a file is a deck like any other, and the fields a
	// deck is required to have are filled from the deployment's defaults. Left
	// empty — as they were — the row failed validation on every later edit, and
	// an imported deck could be looked at and never changed.
	input := s.defaultPresentationInput(request.Context())
	input.Title, input.Prompt, input.Language = presentation.Title, presentation.Prompt, language
	input.SlideCount, input.TemplateID = len(compiled.Slides), presentation.TemplateID
	created, err := s.store.CreatePresentation(request.Context(), user.ID, input)
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
		"presentation": stored, "warnings": warnings, "notes": technical,
		"slides": len(compiled.Slides),
	})
}

// importDocument reads a file that is not a presentation — a spreadsheet, a
// report, a page of notes — into a deck.
//
// The material for a deck usually exists before the deck does. Asking someone
// to retype it into a brief is asking for the work twice, and what they retype
// loses the one thing a company asks about first: where each figure came from.
// So every slide a document produces cites the file and the place in it.
func (s *Server) importDocument(writer http.ResponseWriter, request *http.Request,
	user model.User, meta templateMetadata, data []byte) {
	filename := strings.TrimSpace(meta.Filename)
	if !docs.Reads(filename) {
		writeError(writer, request, http.StatusUnprocessableEntity, "unsupported_document",
			"Ptium reads .pptx presentations and .xlsx, .csv, .docx, .pdf and .md documents", map[string]any{
				"filename": filename, "reads": docs.Extensions,
			})
		return
	}
	document, err := docs.Read(filename, data)
	if err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "unreadable_document", err.Error(),
			map[string]any{"filename": filename})
		return
	}
	title := firstNonEmpty(meta.Name, document.Title, filename, "가져온 문서")
	warnings := append([]string{}, document.Warnings...)
	warnings = append(warnings, fmt.Sprintf("%s의 내용을 슬라이드로 옮기고 각 장에 출처를 달았습니다", filename))
	s.storeImportedSource(writer, request, user, meta, title,
		fmt.Sprintf("%s에서 가져온 자료", filename), document.Source, warnings)
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

// rewritePresentation asks the model to improve a deck that already exists.
//
// It is the same queue generation uses, because it is the same shape of work: a
// round trip to a model that takes a minute or ten. The worker tells the two
// apart by whether the deck already has text — a deck with slides is being
// rewritten, and one without is being written.
//
// The deck's current version is kept: version history is what makes this safe to
// try, and "다시 써 줘" that cannot be taken back is not an offer anyone accepts.
func (s *Server) rewritePresentation(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	if len(presentation.Slides) == 0 || strings.TrimSpace(presentation.Source) == "" {
		writeError(writer, request, http.StatusConflict, "nothing_to_rewrite",
			"이 덱에는 다시 쓸 내용이 없습니다. 먼저 슬라이드를 만들거나 가져오세요", nil)
		return
	}
	// What the author asked for, if they said. "다시 써 줘" with nothing after it
	// is still a request, and the deck is rewritten the way it always was.
	//
	// The limit is generous on purpose: 2,000 Korean characters are 12,000 bytes
	// once a JSON encoder escapes them, and a body cut short by a limit meant to
	// be generous decodes to nothing at all. Which is the other half — a body
	// that cannot be read is said so, rather than read as an empty request.
	var asked struct {
		Instruction string `json:"instruction"`
	}
	if request.Body != nil && request.ContentLength != 0 {
		if err := json.NewDecoder(io.LimitReader(request.Body, 64<<10)).Decode(&asked); err != nil && !errors.Is(err, io.EOF) {
			writeError(writer, request, http.StatusBadRequest, "invalid_json",
				"이 요청의 본문을 읽지 못했습니다", nil)
			return
		}
	}
	if utf8.RuneCountInString(asked.Instruction) > 2000 {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error",
			"instruction must not exceed 2000 characters", nil)
		return
	}
	if !s.aiProviderConfigured(request.Context()) {
		writeError(writer, request, http.StatusConflict, "ai_provider_required",
			"덱 다듬기는 AI 제공자가 설정되어 있어야 합니다. 관리자에게 서비스 설정의 AI 항목을 요청하세요", nil)
		return
	}
	queued, err := s.store.QueueGenerationWith(request.Context(), presentation.ID, user.ID, false,
		s.maximumSlides(request.Context()), asked.Instruction)
	if err != nil {
		s.handleStoreError(writer, request, err, "generation_queue_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "presentation.rewrite", "presentation", presentation.ID,
		map[string]any{"slides": len(presentation.Slides)})
	s.worker.Notify()
	writeData(writer, request, http.StatusAccepted, queued)
}

// aiProviderConfigured reports whether this deployment has a model to ask.
func (s *Server) aiProviderConfigured(ctx context.Context) bool {
	provider, key := "fallback", ""
	_ = s.settings.Get(ctx, "ai.provider", &provider)
	_ = s.settings.Get(ctx, "ai.api_key", &key)
	return !strings.EqualFold(strings.TrimSpace(provider), "fallback") && strings.TrimSpace(key) != ""
}
