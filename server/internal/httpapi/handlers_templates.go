package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/export"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/store"
)

// templateUploadCeiling bounds an upload before the configured limit is read,
// so a hostile request cannot make the server buffer an arbitrary body.
const templateUploadCeiling = pptx.MaxPackageBytes

func (s *Server) listTemplates(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	limit, offset := pagination(request)
	items, total, err := s.store.ListTemplates(request.Context(), user.ID, limit, offset)
	if err != nil {
		s.internalError(writer, request, "templates_read_failed", err)
		return
	}
	// The manifest is large; a listing only needs the summary fields.
	for index := range items {
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
			writeError(writer, request, http.StatusUnprocessableEntity, "template_invalid", err.Error(), nil)
			return
		}
		s.internalError(writer, request, "template_create_failed", err)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "template.create", "template", created.ID,
		map[string]any{"filename": meta.Filename, "sizeBytes": len(data), "layouts": created.LayoutCount})
	writeData(writer, request, http.StatusCreated, created)
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
			writeError(writer, request, http.StatusBadRequest, "invalid_upload", "The upload could not be read", nil)
			return nil, meta, false
		}
	} else {
		var err error
		data, err = io.ReadAll(io.LimitReader(request.Body, limit+1))
		if err != nil {
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
		writeError(writer, request, http.StatusBadRequest, "invalid_upload", "The uploaded template is empty", nil)
		return nil, meta, false
	}
	if extension := strings.ToLower(meta.Filename); extension != "" &&
		!strings.HasSuffix(extension, ".pptx") && !strings.HasSuffix(extension, ".potx") {
		writeError(writer, request, http.StatusUnprocessableEntity, "template_invalid",
			"Only .pptx and .potx templates are supported", map[string]any{"filename": meta.Filename})
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
	svg, err := export.PreviewSVG(presentation, manifest, position, previewWidth(request), templateMedia(data))
	if err != nil {
		writeError(writer, request, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	writeSVG(writer, svg)
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
	if s.settings.Get(ctx, "generation.max_template_mb", &megabytes) != nil || megabytes < 1 {
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
	if s.settings.Get(ctx, "generation.allow_user_uploads", &allowed) != nil {
		return true
	}
	return allowed
}
