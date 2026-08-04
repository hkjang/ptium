package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/store"
)

// A grid is a component an organisation defines for itself: labelled columns,
// labelled rows, and cell values drawn as coloured chips. The definition names
// colour roles rather than colours, so one definition works in every template.

func (s *Server) listGrids(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	specs, err := s.store.ListGrids(request.Context(), user.ID)
	if err != nil {
		s.internalError(writer, request, "grids_read_failed", err)
		return
	}
	writeData(writer, request, http.StatusOK, specs)
}

func (s *Server) saveGrid(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	var spec pptx.GridSpec
	if !decodeJSON(writer, request, &spec) {
		return
	}
	if name := request.PathValue("name"); name != "" {
		spec.Name = name
	}
	saved, err := s.store.SaveGrid(request.Context(), user.ID, spec)
	if err != nil {
		// Everything SaveGrid rejects is the definition's fault and is worth saying
		// out loud; a storage failure surfaces as an incident instead.
		if errors.Is(err, store.ErrGridInvalid) || isValidationMessage(err) {
			writeError(writer, request, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
			return
		}
		s.internalError(writer, request, "grid_save_failed", err)
		return
	}
	s.store.Audit(request.Context(), &user.ID, "grid.save", "grid", saved.Name, nil)
	writeData(writer, request, http.StatusOK, saved)
}

func (s *Server) deleteGrid(writer http.ResponseWriter, request *http.Request) {
	user, _ := UserFromContext(request.Context())
	if err := s.store.DeleteGrid(request.Context(), user.ID, request.PathValue("name")); err != nil {
		s.handleStoreError(writer, request, err, "grid_delete_failed")
		return
	}
	s.store.Audit(request.Context(), &user.ID, "grid.delete", "grid", request.PathValue("name"), nil)
	writer.WriteHeader(http.StatusNoContent)
}

// isValidationMessage distinguishes a rejected definition from a failed write.
func isValidationMessage(err error) bool {
	return strings.HasPrefix(err.Error(), "a grid ")
}

// gridResolver looks up the definitions a deck's source names.
func (s *Server) gridResolver(request *http.Request, ownerID string) func(string) (pptx.GridSpec, bool) {
	return func(name string) (pptx.GridSpec, bool) {
		return s.store.ResolveGrid(request.Context(), ownerID, name)
	}
}
