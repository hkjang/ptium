package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/store"
)

// Images a deck places are owned by the person who uploaded them and referenced
// from deck source by the name they gave. The bytes go wherever the deployment
// said: into PostgreSQL by default, so an air-gapped install has one thing to
// back up, or onto a mounted volume when ASSET_STORAGE=filesystem.

// listAssets returns the caller's images, in the order they asked for.
//
// A picture library is searched four ways — I starred it, I used it recently, I
// use it constantly, I remember its name — and all four are this one endpoint.
func (s *Server) listAssets(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	limit, offset := pagination(request)
	query := request.URL.Query()
	items, total, err := s.store.ListAssets(request.Context(), user.ID, store.AssetQuery{
		Search:   query.Get("q"),
		Tag:      query.Get("tag"),
		Favorite: query.Get("favorite") == "true",
		Sort:     query.Get("sort"),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		s.internalError(writer, request, "assets_read_failed", err)
		return
	}
	writeList(writer, request, items, total, limit, offset)
}

// listAssetTags returns the words this person files images under.
func (s *Server) listAssetTags(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	tags, err := s.store.AssetTags(request.Context(), user.ID)
	if err != nil {
		s.internalError(writer, request, "asset_tags_read_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, tags)
}

// patchAsset renames or retags an image.
func (s *Server) patchAsset(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var body struct {
		Name *string   `json:"name"`
		Tags *[]string `json:"tags"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	asset, err := s.store.UpdateAsset(request.Context(), request.PathValue("id"), user.ID,
		store.AssetPatch{Name: body.Name, Tags: body.Tags})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(writer, request, http.StatusConflict, "asset_name_taken",
				"이미 같은 이름의 이미지가 있습니다", nil)
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			writeError(writer, request, http.StatusNotFound, "not_found", "The requested resource was not found", nil)
			return
		}
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
		return
	}
	writeData(writer, request, http.StatusOK, asset)
}

// favoriteAsset pins an image to the top of its owner's library.
func (s *Server) favoriteAsset(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var body struct {
		Favorite bool `json:"favorite"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	id := request.PathValue("id")
	if _, err := s.store.GetAsset(request.Context(), id, user.ID); err != nil {
		s.handleStoreError(writer, request, err, "asset_read_failed")
		return
	}
	if err := s.store.SetFavorite(request.Context(), user.ID, store.FavoriteAsset, id, body.Favorite); err != nil {
		s.internalError(writer, request, "asset_favorite_failed", err)
		return
	}
	asset, err := s.store.GetAsset(request.Context(), id, user.ID)
	if err != nil {
		s.handleStoreError(writer, request, err, "asset_read_failed")
		return
	}
	writeData(writer, request, http.StatusOK, asset)
}

// createAsset stores an uploaded image.
func (s *Server) createAsset(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	request.Body = http.MaxBytesReader(writer, request.Body, store.MaximumAssetBytes+(1<<20))
	if err := request.ParseMultipartForm(8 << 20); err != nil {
		// Reading the body is where an oversized upload is noticed, and "send it as
		// multipart" is a useless thing to tell someone whose file is simply too
		// big. The two cases are answered separately.
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(writer, request, http.StatusRequestEntityTooLarge, "asset_too_large",
				fmt.Sprintf("An image must be %d MiB or smaller", store.MaximumAssetBytes>>20), nil)
			return
		}
		writeError(writer, request, http.StatusBadRequest, "invalid_upload",
			"Send the image as multipart/form-data with a file field", nil)
		return
	}
	defer func() {
		if request.MultipartForm != nil {
			_ = request.MultipartForm.RemoveAll()
		}
	}()
	file, header, err := request.FormFile("file")
	if err != nil {
		writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", "a file field is required", nil)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, store.MaximumAssetBytes+1))
	if err != nil {
		s.internalError(writer, request, "asset_read_failed", err)
		return
	}
	if len(data) > store.MaximumAssetBytes {
		writeError(writer, request, http.StatusRequestEntityTooLarge, "asset_too_large",
			fmt.Sprintf("An image must be %d MiB or smaller", store.MaximumAssetBytes>>20), nil)
		return
	}
	name := strings.TrimSpace(request.FormValue("name"))
	if name == "" && header != nil {
		name = strings.TrimSpace(header.Filename)
	}
	contentType := ""
	if header != nil {
		contentType = header.Header.Get("Content-Type")
	}
	asset, err := s.store.CreateAsset(request.Context(), user.ID, store.AssetInput{
		Name: name, ContentType: contentType, Data: data,
	})
	if err != nil {
		if errors.Is(err, store.ErrAssetUnsupported) {
			writeError(writer, request, http.StatusUnprocessableEntity, "unsupported_image",
				"This file is not an image Ptium can place: PNG, JPEG, GIF and SVG", nil)
			return
		}
		if errors.Is(err, store.ErrValidation) {
			// An empty file, or one with no name. Saying so is the whole answer; a
			// five hundred would also file an incident against the server for it.
			writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
			return
		}
		s.internalError(writer, request, "asset_create_failed", err)
		return
	}
	if asset.Reused {
		// Not created: this is the picture they already had. 200 says so, and the
		// workspace tells them rather than showing what looks like a duplicate.
		writeData(writer, request, http.StatusOK, asset)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "asset.create", "asset", asset.ID, nil)
	writeData(writer, request, http.StatusCreated, asset)
}

// getAsset returns an image's bytes so the workspace can show it.
func (s *Server) getAsset(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	data, asset, err := s.store.AssetData(request.Context(), request.PathValue("id"), user.ID)
	if errors.Is(err, store.ErrBlobMissing) {
		// The description is in the database and the picture is not on the volume.
		// That is a restore that left the images behind, and saying so is more use
		// than a five hundred.
		writeError(writer, request, http.StatusGone, "asset_bytes_missing",
			"This image's file is missing from the image storage volume. Upload it again.", nil)
		return
	}
	if err != nil {
		s.handleStoreError(writer, request, err, "asset_read_failed")
		return
	}
	writer.Header().Set("Content-Type", asset.ContentType)
	writer.Header().Set("Content-Length", strconv.Itoa(len(data)))
	// The bytes never change for a given id, and the checksum proves it.
	writer.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	writer.Header().Set("ETag", `"`+asset.Checksum+`"`)
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	// An SVG is a document; it must not be able to script the workspace's origin.
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:")
	writer.Header().Set("Content-Disposition", `inline; filename="`+safeFilename(asset.Name)+`"`)
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(data)
}

func (s *Server) deleteAsset(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	if err := s.store.DeleteAsset(request.Context(), request.PathValue("id"), user.ID); err != nil {
		s.handleStoreError(writer, request, err, "asset_delete_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "asset.delete", "asset", request.PathValue("id"), nil)
	writer.WriteHeader(http.StatusNoContent)
}

// imageSource resolves the pictures a deck places, for rendering and preview.
// A deleted image is skipped rather than failing the export.
func (s *Server) imageSource(request *http.Request, ownerID string) deck.ImageSource {
	return func(assetID string) (pptx.Picture, bool) {
		data, asset, err := s.store.AssetData(request.Context(), assetID, ownerID)
		if err != nil {
			return pptx.Picture{}, false
		}
		return pptx.Picture{
			Data: data, ContentType: asset.ContentType,
			Width: asset.Width, Height: asset.Height,
			// A picture with no alternative text is an error in PowerPoint's own
			// accessibility check. The caption the author wrote is the right text
			// and wins; failing that, the name they gave the image in their
			// library describes it, where a file name does not.
			Caption: describedName(asset.Name),
		}, true
	}
}

// describedName is an image's name when the name says what the image is.
// "매장 전경" describes a photograph; "IMG_2481.png" names a file.
func describedName(name string) string {
	trimmed := strings.TrimSpace(name)
	lowered := strings.ToLower(trimmed)
	for _, extension := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".heic"} {
		if strings.HasSuffix(lowered, extension) {
			return ""
		}
	}
	if utf8.RuneCountInString(trimmed) < 2 {
		return ""
	}
	return trimmed
}

// resolveImage turns the reference an author wrote into a stored image.
func (s *Server) resolveImage(request *http.Request, ownerID string) func(string) (deck.ContentImage, bool) {
	return func(reference string) (deck.ContentImage, bool) {
		asset, err := s.store.ResolveAsset(request.Context(), ownerID, reference)
		if err != nil {
			return deck.ContentImage{}, false
		}
		return deck.ContentImage{AssetID: asset.ID, Name: asset.Name}, true
	}
}
