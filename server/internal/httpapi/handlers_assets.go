package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/store"
)

// Images a deck places are owned by the person who uploaded them and referenced
// from deck source by the name they gave. The bytes go wherever the deployment
// said: into PostgreSQL by default, so an air-gapped install has one thing to
// back up, or onto a mounted volume when ASSET_STORAGE=filesystem.

// listAssets returns the caller's images.
func (s *Server) listAssets(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	limit, offset := pagination(request)
	items, total, err := s.store.ListAssets(request.Context(), user.ID, limit, offset)
	if err != nil {
		s.internalError(writer, request, "assets_read_failed", err)
		return
	}
	writeList(writer, request, items, total, limit, offset)
}

// createAsset stores an uploaded image.
func (s *Server) createAsset(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	request.Body = http.MaxBytesReader(writer, request.Body, store.MaximumAssetBytes+(1<<20))
	if err := request.ParseMultipartForm(8 << 20); err != nil {
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
				"Ptium places PNG, JPEG, GIF and SVG images", nil)
			return
		}
		s.internalError(writer, request, "asset_create_failed", err)
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
		}, true
	}
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
