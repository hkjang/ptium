package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hkjang/ptium/server/internal/generation"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/docs"
	"github.com/hkjang/ptium/server/internal/export"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/settings"
	"github.com/hkjang/ptium/server/internal/store"
)

// templateUploadCeiling bounds an upload before the configured limit is read,
// so a hostile request cannot make the server buffer an arbitrary body.
const templateUploadCeiling = pptx.MaxPackageBytes

func (s *Server) listTemplates(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	limit, offset := pagination(request)
	// A service picking a design, and a person scrolling a gallery, both narrow
	// before they choose.
	filter := store.TemplateFilter{
		Kind:   request.URL.Query().Get("kind"),
		Search: searchTerm(request),
	}
	items, total, err := s.store.ListTemplatesFiltered(request.Context(), user.ID, filter, limit, offset)
	if err != nil {
		s.internalError(writer, request, "templates_read_failed", err)
		return
	}
	// The manifest is large; a listing only needs the summary fields — but what a
	// template looks like is read out of it first, so a gallery can be narrowed
	// without downloading forty manifests.
	for index := range items {
		describeTemplate(&items[index])
		items[index].Manifest = nil
	}
	writeList(writer, request, items, total, limit, offset)
}

func (s *Server) getTemplate(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	template, err := s.store.GetTemplate(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "template_read_failed")
		return
	}
	// What a template's own palette can carry is a property of the customer's
	// design, and they are the only ones who can change it — so it is reported
	// rather than silently worked around.
	describeTemplate(&template)
	manifest, manifestErr := decodeManifest(template)
	if manifestErr != nil || len(manifest.Layouts) == 0 {
		writeData(writer, request, http.StatusOK, template)
		return
	}
	writeData(writer, request, http.StatusOK, struct {
		model.Template
		Palette pptx.ThemeAudit `json:"palette"`
	}{Template: template, Palette: pptx.AuditTheme(manifest)})
}

func (s *Server) createTemplate(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	if !s.templateUploadsAllowed(request.Context()) && !user.IsAdmin {
		writeError(writer, request, http.StatusForbidden, "template_uploads_disabled", "Template uploads are disabled by the administrator", nil)
		return
	}
	// Hold the upload before reading it, not after: the bytes are in memory from
	// the moment the body is read, and it is the number of them at once that
	// decides whether the process survives.
	release, ok := s.holdTemplateRead(writer, request)
	if !ok {
		return
	}
	defer release()
	limit := s.maximumTemplateBytes(request.Context())
	data, meta, ok := s.readTemplateUpload(writer, request, limit)
	if !ok {
		return
	}
	created, err := s.store.CreateTemplate(request.Context(), user.ID, store.TemplateInput{
		Name: meta.Name, Description: meta.Description, Filename: meta.Filename, Scope: meta.Scope, Data: data,
	})
	if err != nil {
		if errors.Is(err, store.ErrTemplateInvalid) {
			message := err.Error()
			if hint := templateUploadHint(data); hint != "" {
				message = hint
			}
			writeError(writer, request, http.StatusUnprocessableEntity, "template_invalid", message, nil)
			return
		}
		s.internalError(writer, request, "template_create_failed", err)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "template.create", "template", created.ID,
		map[string]any{"filename": meta.Filename, "sizeBytes": len(data), "layouts": created.LayoutCount})
	writeData(writer, request, http.StatusCreated, created)
}

// templateUploadHint names what a rejected upload actually is, when the first
// bytes say so plainly. "not a usable PowerPoint file" is true and useless: the
// two files this was written for are a deck saved in the 97-2003 format and one
// wrapped by document security, and in both cases the person holding the file
// can fix it in a minute if somebody tells them which it is.
func templateUploadHint(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}):
		return "예전 PowerPoint 형식(.ppt)이거나 암호가 걸린 파일입니다. " +
			"PowerPoint에서 [다른 이름으로 저장] → PowerPoint 프레젠테이션(.pptx)으로 저장한 뒤 올려 주세요."
	case bytes.HasPrefix(data, []byte("SCDS")):
		return "문서보안(DRM)으로 잠긴 파일입니다. 보안을 해제한 .pptx 파일을 올려 주세요."
	case len(data) >= 4 && !bytes.HasPrefix(data, []byte("PK")):
		return "PowerPoint 파일이 아닙니다. 문서보안(DRM)으로 감싸였거나 다른 형식일 수 있습니다. " +
			"PowerPoint에서 열리는 .pptx 파일을 올려 주세요."
	// An Office package that is not a presentation says what it is in the names
	// of its own parts, and saying "the package does not contain a PowerPoint
	// presentation" to somebody who renamed a Word file is a dead end. Ptium
	// reads Word and Excel at the import door: the answer is which door.
	case bytes.Contains(data, []byte("word/document.xml")):
		return "PowerPoint 파일이 아니라 Word 문서입니다. 확장자를 .docx 로 되돌려 " +
			"[기존 자료 가져오기]에 올리면 제목이 슬라이드가 됩니다."
	case bytes.Contains(data, []byte("xl/workbook.xml")):
		return "PowerPoint 파일이 아니라 Excel 문서입니다. 확장자를 .xlsx 로 되돌려 " +
			"[기존 자료 가져오기]에 올리면 시트마다 한 장이 됩니다."
	// Anything else that reached this function is a package that would not open.
	// Which of the two it is — damaged, or simply not a presentation — the bytes
	// do not say, so neither does this; what it does say is what to try, and it
	// says it in the language the person is reading.
	case bytes.HasPrefix(data, []byte("PK")):
		return "PowerPoint 파일로 열리지 않습니다. 파일이 손상됐거나 PowerPoint 파일이 아닐 수 있습니다. " +
			"PowerPoint에서 열리는지 확인한 뒤 다시 올려 주세요."
	}
	return ""
}

type templateMetadata struct {
	Name        string
	Description string
	Filename    string
	Scope       string
}

// readTemplateUpload accepts either a multipart form (what the browser sends)
// or a raw request body with metadata in the query string (what a script or an
// MCP client finds convenient).
func (s *Server) readTemplateUpload(writer http.ResponseWriter, request *http.Request, limit int64) ([]byte, templateMetadata, bool) {
	return s.readUpload(writer, request, limit, "Only .pptx and .potx templates are supported",
		func(name string) bool {
			return strings.HasSuffix(name, ".pptx") || strings.HasSuffix(name, ".potx")
		})
}

// readImportUpload reads what someone brings in to make a deck from: a
// presentation, or the material one would be written from.
func (s *Server) readImportUpload(writer http.ResponseWriter, request *http.Request, limit int64) ([]byte, templateMetadata, bool) {
	return s.readUpload(writer, request, limit,
		"Ptium reads .pptx presentations and .xlsx, .csv, .docx, .pdf and .md documents",
		func(name string) bool {
			return strings.HasSuffix(name, ".pptx") || strings.HasSuffix(name, ".potx") || docs.Reads(name)
		})
}

func (s *Server) readUpload(writer http.ResponseWriter, request *http.Request, limit int64,
	refusal string, accepted func(string) bool) ([]byte, templateMetadata, bool) {
	meta := templateMetadata{
		Name:        strings.TrimSpace(request.URL.Query().Get("name")),
		Description: strings.TrimSpace(request.URL.Query().Get("description")),
		Filename:    strings.TrimSpace(request.URL.Query().Get("filename")),
		Scope:       strings.TrimSpace(request.URL.Query().Get("scope")),
	}
	request.Body = http.MaxBytesReader(writer, request.Body, limit+1024)

	var data []byte
	if strings.HasPrefix(request.Header.Get("Content-Type"), "multipart/form-data") {
		if err := request.ParseMultipartForm(8 << 20); err != nil {
			if refusedForSize(writer, request, err, limit) {
				return nil, meta, false
			}
			writeError(writer, request, http.StatusBadRequest, "invalid_upload", "The upload could not be read", map[string]any{"reason": err.Error()})
			return nil, meta, false
		}
		defer func() { _ = request.MultipartForm.RemoveAll() }()
		file, header, err := request.FormFile("file")
		if err != nil {
			writeError(writer, request, http.StatusBadRequest, "invalid_upload", "A PowerPoint file must be supplied in the 'file' field", nil)
			return nil, meta, false
		}
		defer file.Close()
		if value := strings.TrimSpace(request.FormValue("name")); value != "" {
			meta.Name = value
		}
		if value := strings.TrimSpace(request.FormValue("description")); value != "" {
			meta.Description = value
		}
		if value := strings.TrimSpace(request.FormValue("scope")); value != "" {
			meta.Scope = value
		}
		if meta.Filename == "" && header != nil {
			meta.Filename = header.Filename
		}
		data, err = io.ReadAll(io.LimitReader(file, limit+1))
		if err != nil {
			if refusedForSize(writer, request, err, limit) {
				return nil, meta, false
			}
			writeError(writer, request, http.StatusBadRequest, "invalid_upload", "The upload could not be read", nil)
			return nil, meta, false
		}
	} else {
		var err error
		data, err = io.ReadAll(io.LimitReader(request.Body, limit+1))
		if err != nil {
			if refusedForSize(writer, request, err, limit) {
				return nil, meta, false
			}
			writeError(writer, request, http.StatusBadRequest, "invalid_upload", "The upload could not be read", nil)
			return nil, meta, false
		}
	}

	if int64(len(data)) > limit {
		writeError(writer, request, http.StatusRequestEntityTooLarge, "template_too_large",
			fmt.Sprintf("The template must not exceed %d MiB", limit>>20), nil)
		return nil, meta, false
	}
	if len(data) == 0 {
		writeError(writer, request, http.StatusBadRequest, "invalid_upload", "올린 파일이 비어 있습니다", nil)
		return nil, meta, false
	}
	// A template must be a PowerPoint file. An import may also be the material
	// for a deck — a spreadsheet, a report, a page of notes — and the endpoint
	// that reads those says so by allowing them here.
	if extension := strings.ToLower(meta.Filename); extension != "" && !accepted(extension) {
		writeError(writer, request, http.StatusUnprocessableEntity, "template_invalid",
			refusal, map[string]any{"filename": meta.Filename})
		return nil, meta, false
	}
	if utf8.RuneCountInString(meta.Name) > 120 || utf8.RuneCountInString(meta.Description) > 1000 {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "Template name or description is too long", nil)
		return nil, meta, false
	}
	meta.Filename = safeFilename(meta.Filename)
	return data, meta, true
}

type templatePatchRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Scope       *string `json:"scope"`
}

// favoriteTemplate pins a design for one person.
//
// It needs no write access to the template: pinning the company deck template is
// a note about one's own workspace, and nobody else's copy changes.
func (s *Server) favoriteTemplate(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var body struct {
		Favorite bool `json:"favorite"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	id := request.PathValue("id")
	if _, err := s.store.GetTemplate(request.Context(), id, user.ID, user.IsAdmin); err != nil {
		s.handleStoreError(writer, request, err, "template_read_failed")
		return
	}
	if err := s.store.SetFavorite(request.Context(), user.ID, store.FavoriteTemplate, id, body.Favorite); err != nil {
		s.internalError(writer, request, "template_favorite_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, map[string]any{"id": id, "favorite": body.Favorite})
}

func (s *Server) patchTemplate(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	current, err := s.store.GetTemplate(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "template_read_failed")
		return
	}
	if current.Kind == "builtin" {
		writeError(writer, request, http.StatusConflict, "template_is_builtin", "Built-in templates cannot be modified", nil)
		return
	}
	var input templatePatchRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	update := store.TemplateInput{Name: current.Name, Description: current.Description, Scope: current.Scope}
	if input.Name != nil {
		update.Name = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		update.Description = strings.TrimSpace(*input.Description)
	}
	if input.Scope != nil {
		update.Scope = strings.TrimSpace(*input.Scope)
	}
	if update.Name == "" || utf8.RuneCountInString(update.Name) > 120 || utf8.RuneCountInString(update.Description) > 1000 {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "Template name is required and must not exceed 120 characters", nil)
		return
	}
	if update.Scope != "private" && update.Scope != "shared" {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "scope must be private or shared", nil)
		return
	}
	updated, err := s.store.UpdateTemplate(request.Context(), current.ID, user.ID, false, update)
	if err != nil {
		s.handleStoreError(writer, request, err, "template_update_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "template.update", "template", updated.ID, map[string]any{"scope": updated.Scope})
	writeData(writer, request, http.StatusOK, updated)
}

func (s *Server) deleteTemplate(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	id := request.PathValue("id")
	if err := s.store.DeleteTemplate(request.Context(), id, user.ID, false); err != nil {
		s.handleStoreError(writer, request, err, "template_delete_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "template.delete", "template", id, nil)
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) downloadTemplate(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	data, template, err := s.store.TemplateData(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "template_read_failed")
		return
	}
	filename := template.Filename
	if strings.TrimSpace(filename) == "" {
		filename = safeFilename(template.Name) + ".pptx"
	}
	writer.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.presentationml.presentation")
	writer.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	writer.Header().Set("Content-Length", strconv.Itoa(len(data)))
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}

// templateHealth says what this template will do to a deck, before somebody
// puts forty decks through it.
//
// The promise of this product is that the template is the design. The other
// side of that promise is that a template decides what a deck can be: a design
// whose only content layout has no body region turns every component into a
// paragraph, and nothing said so until the decks came out. Rather than judging
// a template against a list of rules, this compiles one representative deck
// into it and reports what the compiler and the measurement actually said.
func (s *Server) templateHealth(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	data, template, err := s.store.TemplateData(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "template_read_failed")
		return
	}
	manifest, _, err := s.templateArtwork(request.Context(), template, data)
	if err != nil {
		s.internalError(writer, request, "template_manifest_unreadable", err)
		return
	}
	profile, _ := s.store.GetProfile(request.Context(), user.ID)
	probe := model.Presentation{Title: template.Name, Language: "ko", RequestedSlideCount: 7}
	// The probe resolves its own picture: a customer's question is whether this
	// design has a place for one, and nothing here is saved, so no asset of
	// theirs needs to exist for the answer.
	compiled := generation.CompileSourceWith(templateProbeSource, probe, profile,
		generation.Template{ID: template.ID, Manifest: manifest}, probePicture, nil)
	report := templateReport{
		TemplateID: template.ID, Name: template.Name,
		AspectRatio: manifest.AspectRatio, Layouts: len(manifest.Layouts),
		Warnings: compiled.Warnings, Slides: len(compiled.Slides),
		Roles: map[string]bool{
			"cover":   hasLayoutForRole(manifest, pptx.RoleTitle),
			"section": hasLayoutForRole(manifest, pptx.RoleSection),
			"content": hasLayoutForRole(manifest, pptx.RoleContent),
			"closing": hasLayoutForRole(manifest, pptx.RoleClosing),
		},
	}
	if report.Warnings == nil {
		report.Warnings = []string{}
	}
	report.Components = componentsDrawn(compiled.Slides)
	findings := s.inspectCompiled(request, user.ID, probe, manifest, compiled.Slides)
	for _, finding := range findings {
		if finding.Advisory {
			report.Advisories++
			continue
		}
		report.Defects++
		report.DefectDetails = append(report.DefectDetails,
			fmt.Sprintf("%d번 슬라이드 %s: %s", finding.Slide, finding.Slot, finding.Detail))
	}
	writeData(writer, request, http.StatusOK, report)
}

// templateReport is what a template does to a deck.
type templateReport struct {
	TemplateID    string          `json:"templateId"`
	Name          string          `json:"name"`
	AspectRatio   string          `json:"aspectRatio,omitempty"`
	Layouts       int             `json:"layouts"`
	Slides        int             `json:"slides"`
	Roles         map[string]bool `json:"roles"`
	Components    map[string]bool `json:"components"`
	Warnings      []string        `json:"warnings"`
	Defects       int             `json:"defects"`
	Advisories    int             `json:"advisories"`
	DefectDetails []string        `json:"defectDetails,omitempty"`
}

// templateProbeSource is one deck with every kind of thing a deck holds: a
// cover, prose, the four components a brief most often produces, notes and a
// closing. What a template does to this is what it will do to real work.
// componentsDrawn says which of the drawn things this template drew, and which
// came out as paragraphs. It is the answer the question is really about: a
// design whose content layout has no free region turns every component into
// prose, and until this said so the way to find out was to make forty decks.
func componentsDrawn(slides []model.Slide) map[string]bool {
	drawn := map[string]bool{}
	for _, kind := range []string{pptx.BlockSteps, pptx.BlockKPI, pptx.BlockShare, pptx.BlockTable} {
		drawn[kind] = false
	}
	// A picture is not a block: it is carried on the slide, and a layout with
	// nowhere to put one drops it. That drop is worth the same warning as a
	// table that came out as prose.
	drawn["image"] = false
	for _, slide := range slides {
		var content deck.Content
		if json.Unmarshal(slide.Content, &content) != nil {
			continue
		}
		if len(content.Images) > 0 {
			drawn["image"] = true
		}
		for _, block := range content.Blocks {
			if _, wanted := drawn[block.Kind]; wanted {
				drawn[block.Kind] = true
			}
		}
	}
	return drawn
}

// hasLayoutForRole answers whether this design has a layout for a part of a
// deck, rather than whether the manifest names one.
//
// Every manifest names one for all four: a template with a single content
// layout gets that layout for its cover and its closing too, which is what
// makes it usable. But it means the design has no cover, and the report used to
// say "cover ✓" directly above the compiler saying "this template has no
// closing layout" about the very same deck.
func hasLayoutForRole(manifest pptx.Manifest, role string) bool {
	for _, layout := range manifest.Layouts {
		if layout.Role == role {
			return true
		}
	}
	return false
}

// probePicture stands in for one of the customer's own images.
func probePicture(name string) (deck.ContentImage, bool) {
	return deck.ContentImage{AssetID: templateProbeAsset, Name: name}, true
}

// templateProbeAsset is not an asset anybody has: the report is about layout,
// and the measurement treats an unreadable picture as a placed one.
const templateProbeAsset = "00000000-0000-0000-0000-000000000000"

const templateProbeSource = `# 표지
@cover
> 오늘 결정할 것은 예산과 시점입니다

# 현황과 문제
- 전환 대상 42개 시스템, 이관 기간 18개월
- 운영 비용은 매년 12% 늘고 있습니다
!notes 숫자는 재무팀 확정본

# 이행 순서
::steps
- 준비 | 범위 · 조직 · 예산을 확정
- 이행 | 단계별로 적용하고 완료 조건을 확인
- 안정화 | 운영 이관과 점검 기준 확정
::

# 기대 효과
::kpi
- 전환 시스템 | 42개
- 절감 | 18억
- 복구 시간 | 30분
::

# 채널별 비중
::share
- 직판 | 46%
- 대리점 | 33%
- 온라인 | 21%
::

# 연간 비용
::table
- 항목 | 2026 | 2027
- 인건비 | 4.2 | 3.4
::

# 현장 사진
@picture
::image 사진

# 다음 단계
@closing
- 오늘 요청하는 결정 한 가지
`

// templateLayoutPreview renders one layout of a template as SVG so the picker
// can show the customer their own design rather than a generic thumbnail.
func (s *Server) templateLayoutPreview(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	// The package itself is needed, not just the manifest: a template's identity
	// is usually a photograph or a logo, and those live in the package.
	data, template, err := s.store.TemplateData(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "template_read_failed")
		return
	}
	manifest, media, err := s.templateArtwork(request.Context(), template, data)
	if err != nil {
		s.internalError(writer, request, "template_manifest_unreadable", err)
		return
	}
	layoutID := request.PathValue("layoutId")
	layout, ok := manifest.Layout(layoutID)
	if !ok {
		if layoutID != "" && layoutID != "default" {
			writeError(writer, request, http.StatusNotFound, "not_found", "The requested layout does not exist", nil)
			return
		}
		if layout, ok = manifest.Layout(manifest.TitleLayout); !ok {
			layout = manifest.Layouts[0]
		}
	}
	svg := pptx.PreviewLayoutSVG(manifest, layout, pptx.PreviewOptions{Width: previewWidth(request), Media: media})
	writeSVG(writer, svg)
}

// presentationPreview renders a stored slide through its template so the
// editor can show what the exported file will look like.
func (s *Server) presentationPreview(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	presentation, err := s.store.GetPresentation(request.Context(), request.PathValue("id"), user.ID, false)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_read_failed")
		return
	}
	if len(presentation.Slides) == 0 {
		writeError(writer, request, http.StatusConflict, "presentation_has_no_slides", "Generate or add slides before previewing", nil)
		return
	}
	data, manifest, err := s.presentationTemplate(request.Context(), presentation)
	if err != nil {
		s.handleStoreError(writer, request, err, "presentation_template_unavailable")
		return
	}
	position, _ := strconv.Atoi(request.URL.Query().Get("slide"))
	if position < 1 {
		position = 1
	}
	options := pptx.PreviewOptions{Width: previewWidth(request), Media: templateMedia(data),
		Reveal: revealCount(request)}
	if position <= len(presentation.Slides) {
		query := request.URL.Query()
		// The canvas asks for the template-bound layer without freeform objects and
		// draws its unsaved local objects above it. Other previews include everything.
		dropFreeform := query.Get("freeform") == "false"
		// It also asks for one region on its own, or for the page without the
		// regions it is currently moving, so that dragging a generated component
		// moves the drawing itself instead of an outline over a stale copy of it.
		only := strings.TrimSpace(query.Get("only"))
		exclude := splitSlots(query.Get("exclude"))
		if dropFreeform || only != "" || len(exclude) > 0 {
			presentation.Slides = append([]model.Slide(nil), presentation.Slides...)
			presentation.Slides[position-1] = filterSlideRegions(presentation.Slides[position-1], only, exclude, dropFreeform)
			options.Bare = only != ""
		}
	}
	svg, err := export.PreviewSlideSVG(presentation, manifest, position, options, s.imageSource(request, user.ID))
	if err != nil {
		writeError(writer, request, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	writeSVG(writer, svg)
}

// splitSlots reads a comma-separated slot list, ignoring the empty entries a
// client produces when it joins nothing.
func splitSlots(value string) []string {
	slots := make([]string, 0, 4)
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" && len(trimmed) <= 100 {
			slots = append(slots, trimmed)
		}
	}
	return slots
}

// filterSlideRegions narrows a slide to one region, or removes some of them.
//
// The title and the bullet list are also cleared, because rendering falls back
// to them when the fields are empty: without that, asking for the body alone
// would draw the title as well, and the canvas would show it twice.
func filterSlideRegions(slide model.Slide, only string, exclude []string, dropFreeform bool) model.Slide {
	content := deck.Decode(slide.Content)
	if dropFreeform {
		content.Elements = nil
	}
	if only == "" && len(exclude) == 0 {
		slide.Content = content.Encode()
		return slide
	}
	keep := func(slot string) bool {
		if only != "" {
			return slot == only
		}
		for _, excluded := range exclude {
			if excluded == slot {
				return false
			}
		}
		return true
	}
	for slot := range content.Fields {
		if !keep(slot) {
			delete(content.Fields, slot)
		}
	}
	for slot := range content.Blocks {
		if !keep(slot) {
			delete(content.Blocks, slot)
		}
	}
	for slot := range content.Images {
		if !keep(slot) {
			delete(content.Images, slot)
		}
	}
	if only != "" {
		content.Elements = nil
	}
	content.Bullets = nil
	content.Body = ""
	if !keep(pptx.SlotTitle) {
		slide.Title = ""
	}
	if !keep(pptx.SlotSubtitle) {
		slide.Subtitle = ""
	}
	slide.Content = content.Encode()
	return slide
}

// describeTemplate fills in what a template looks like, for a gallery that has
// to be narrowed rather than scrolled. A shipped design knows its own tags; an
// uploaded one is read from its manifest, because the customer's design is the
// only thing that can answer for it.
func describeTemplate(template *model.Template) {
	if template.Kind == "builtin" {
		design := pptx.LookupBuiltinDesign(template.PaletteKey)
		template.Dark = design.Palette.Dark
		template.Tags = design.Tags()
		template.DesignRank = pptx.BuiltinDesignRank(design.Key)
		return
	}
	tags := []string{"내 템플릿"}
	if manifest, err := decodeManifest(*template); err == nil {
		template.Dark = manifest.IsDark()
		if template.Dark {
			tags = append(tags, "어두운")
		} else {
			tags = append(tags, "밝은")
		}
		if face := strings.ToLower(manifest.Theme.MajorLatin); strings.Contains(face, "georgia") ||
			strings.Contains(face, "serif") || strings.Contains(face, "times") || strings.Contains(face, "명조") {
			tags = append(tags, "세리프")
		}
	}
	template.Tags = tags
}

func writeSVG(writer http.ResponseWriter, svg string) {
	writer.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	// Pictures are embedded as data URIs, which the policy has to allow.
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data:; style-src 'unsafe-inline'")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, svg)
}

// revealCount is how many of a slide's points to draw, for a slide being built
// up a line at a time while somebody presents it. Anything else draws the whole
// slide, which is every other caller.
func revealCount(request *http.Request) int {
	reveal, _ := strconv.Atoi(request.URL.Query().Get("reveal"))
	if reveal < 1 || reveal > 200 {
		return 0
	}
	return reveal
}

func previewWidth(request *http.Request) int {
	width, _ := strconv.Atoi(request.URL.Query().Get("width"))
	if width < 160 {
		return 960
	}
	if width > 1920 {
		return 1920
	}
	return width
}

func decodeManifest(template model.Template) (pptx.Manifest, error) {
	var manifest pptx.Manifest
	if err := json.Unmarshal(template.Manifest, &manifest); err != nil {
		return pptx.Manifest{}, fmt.Errorf("template %q has an unreadable manifest", template.Name)
	}
	if len(manifest.Layouts) == 0 {
		return pptx.Manifest{}, fmt.Errorf("template %q does not expose any usable layout", template.Name)
	}
	return manifest, nil
}

// presentationTemplate resolves the package and manifest a deck renders with,
// falling back to a built-in design when the deck predates templates.
// templateArtwork returns a template's manifest and a resolver for its pictures,
// re-analyzing the package when the stored manifest predates the current
// analyzer. An older manifest has no artwork recorded at all, so a template
// uploaded before this release would otherwise keep previewing as a blank slide.
func (s *Server) templateArtwork(ctx context.Context, template model.Template, data []byte) (pptx.Manifest, pptx.MediaResolver, error) {
	manifest, err := decodeManifest(template)
	if err != nil {
		return pptx.Manifest{}, nil, err
	}
	if manifest.Version != pptx.ManifestVersion && len(data) > 0 {
		if _, refreshed, analyzeErr := pptx.AnalyzeBytes(data); analyzeErr == nil {
			manifest = refreshed
			// Store it so the next preview, and every export, uses the same
			// analysis rather than repeating it.
			if updateErr := s.store.UpdateTemplateManifest(ctx, template.ID, refreshed); updateErr != nil {
				s.logger.Warn("could not store a refreshed template manifest",
					"request_id", RequestID(ctx), "template_id", template.ID, "error", updateErr)
			}
		}
	}
	return manifest, templateMedia(data), nil
}

// templateMedia builds a picture resolver over template bytes.
func templateMedia(data []byte) pptx.MediaResolver {
	if len(data) == 0 {
		return nil
	}
	pkg, err := pptx.Open(data)
	if err != nil {
		return nil
	}
	return pptx.PackageMedia(pkg, 0)
}

func (s *Server) presentationTemplate(ctx context.Context, presentation model.Presentation) ([]byte, pptx.Manifest, error) {
	id := ""
	if presentation.TemplateID != nil {
		id = *presentation.TemplateID
	}
	if id == "" {
		resolved, err := s.store.DefaultTemplateID(ctx, presentation.OwnerID, presentation.Theme)
		if err != nil {
			return nil, pptx.Manifest{}, err
		}
		id = resolved
	}
	data, template, err := s.store.TemplateData(ctx, id, presentation.OwnerID, true)
	if err != nil {
		return nil, pptx.Manifest{}, err
	}
	manifest, err := decodeManifest(template)
	if err != nil {
		return nil, pptx.Manifest{}, err
	}
	return data, manifest, nil
}

func (s *Server) maximumTemplateBytes(ctx context.Context) int64 {
	megabytes := 32
	if s.settings == nil {
		return int64(megabytes) << 20
	}
	if s.settings.Get(ctx, "generation.max_template_mb", &megabytes) != nil ||
		!settings.Numbers["generation.max_template_mb"].Holds(megabytes) {
		megabytes = 32
	}
	limit := int64(megabytes) << 20
	if limit > templateUploadCeiling {
		limit = templateUploadCeiling
	}
	return limit
}

func (s *Server) templateUploadsAllowed(ctx context.Context) bool {
	allowed := true
	if s.settings == nil {
		return true
	}
	if s.settings.Get(ctx, "generation.allow_user_uploads", &allowed) != nil {
		return true
	}
	return allowed
}

// holdTemplateRead takes one of the slots for reading a template, waiting for
// one to come free. A site whose people all upload at the same moment waits a
// couple of seconds each; without it the fourth of them killed the process and
// took every other request in flight with it.
func (s *Server) holdTemplateRead(writer http.ResponseWriter, request *http.Request) (func(), bool) {
	release, ok := holdSlot(writer, request, s.analysingTemplates, templateReadWait, "templates_busy",
		"This deployment is already reading as many templates as it can hold at once. Try again in a moment.")
	if ok {
		s.templateReadsTaken.Add(1)
	}
	return release, ok
}

// holdBudget waits for room in the document-building budget, so that work whose
// cost is measured in hundreds of megabytes cannot all happen at once. What it
// waits for is room for this kind of document, not a turn in a queue: a .pptx
// costs three times what a PDF costs and takes the whole budget.
func (s *Server) holdBudget(writer http.ResponseWriter, request *http.Request, cost int64) (func(), bool) {
	if s.building == nil {
		return func() {}, true
	}
	waiting, cancel := context.WithTimeout(request.Context(), printWait)
	defer cancel()
	if err := s.building.Acquire(waiting, cost); err != nil {
		if request.Context().Err() != nil {
			return func() {}, false
		}
		writeError(writer, request, http.StatusServiceUnavailable, "printing_busy",
			"This deployment is already building as many documents as it can at once. Try again in a moment.", nil)
		return func() {}, false
	}
	return func() { s.building.Release(cost) }, true
}

// holdSlot waits for one of a bounded set of slots, so that work whose cost is
// measured in hundreds of megabytes cannot all happen at once. A caller that
// waits too long is answered rather than left hanging.
func holdSlot(writer http.ResponseWriter, request *http.Request, slots chan struct{},
	wait time.Duration, code, message string) (func(), bool) {
	if slots == nil {
		return func() {}, true
	}
	waiting, cancel := context.WithTimeout(request.Context(), wait)
	defer cancel()
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, true
	case <-waiting.Done():
		if request.Context().Err() != nil {
			return func() {}, false
		}
		writeError(writer, request, http.StatusServiceUnavailable, code, message, nil)
		return func() {}, false
	}
}

// templateReadWait is how long an upload waits for a slot. Reading the largest
// template the settings allow takes a little over two seconds, so this is room
// for a queue of a dozen rather than a promise about any one of them.
var templateReadWait = 30 * time.Second

// printWait is how long a print waits for its turn. A forty-slide deck with a
// photograph on every page draws in a couple of seconds.
var printWait = 60 * time.Second

// refusedForSize answers an upload that is over the deployment's limit with the
// limit, rather than with the fact that reading stopped.
//
// The reader that enforces the limit trips before the check further down ever
// runs, so the clear answer that check writes — "the template must not exceed
// 32 MiB" — was unreachable for a file that was genuinely too big. What a
// person uploading a 60 MB deck to a 32 MB deployment actually got was "The
// upload could not be read", with "http: request body too large" tucked into a
// details field.
func refusedForSize(writer http.ResponseWriter, request *http.Request, err error, limit int64) bool {
	var tooBig *http.MaxBytesError
	if !errors.As(err, &tooBig) {
		return false
	}
	writeError(writer, request, http.StatusRequestEntityTooLarge, "template_too_large",
		fmt.Sprintf("The upload must not exceed %d MiB", limit>>20),
		map[string]any{"limitBytes": limit})
	return true
}
