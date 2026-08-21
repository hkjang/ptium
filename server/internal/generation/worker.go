package generation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
	"github.com/hkjang/ptium/server/internal/store"
)

type Worker struct {
	store     *store.Store
	generator *Generator
	logger    *slog.Logger
	interval  time.Duration
	wake      chan struct{}
}

func NewWorker(st *store.Store, generator *Generator, logger *slog.Logger, interval time.Duration) *Worker {
	return &Worker{store: st, generator: generator, logger: logger, interval: interval, wake: make(chan struct{}, 1)}
}

func (w *Worker) Notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if err := w.processOne(ctx); err != nil && !errors.Is(err, store.ErrNotFound) && !errors.Is(err, context.Canceled) {
			w.logger.Error("generation worker iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.wake:
		}
	}
}

func (w *Worker) processOne(ctx context.Context) error {
	presentation, err := w.store.ClaimGeneration(ctx)
	if err != nil {
		return err
	}
	profile, err := w.store.GetProfile(ctx, presentation.OwnerID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return w.fail(ctx, presentation, err)
	}
	template, err := w.resolveTemplate(ctx, presentation)
	if err != nil {
		return w.fail(ctx, presentation, err)
	}
	generated, err := w.generator.Generate(ctx, presentation, profile, template)
	if err != nil {
		return w.fail(ctx, presentation, err)
	}
	if err := w.store.CompleteGeneration(ctx, presentation.ID, generated.Outline, generated.Slides, generated.Source); err != nil {
		return w.fail(ctx, presentation, err)
	}
	w.logger.Info("presentation generated", "presentation_id", presentation.ID,
		"slides", len(generated.Slides), "template", template.Name,
		// What compiling adjusted and what the repair pass rewrote. An operator
		// asking why a deck looks the way it does should not have to guess.
		"warnings", generated.Warnings)
	return nil
}

// resolveTemplate loads the design a deck must be written into, falling back
// to the built-in design that matches the requested theme when a presentation
// predates templates or its template was deleted.
func (w *Worker) resolveTemplate(ctx context.Context, presentation model.Presentation) (Template, error) {
	id := ""
	if presentation.TemplateID != nil {
		id = *presentation.TemplateID
	}
	if id == "" {
		resolved, err := w.store.DefaultTemplateID(ctx, presentation.OwnerID, presentation.Theme)
		if err != nil {
			return Template{}, fmt.Errorf("no presentation template is available: %w", err)
		}
		id = resolved
	}
	stored, err := w.store.GetTemplate(ctx, id, presentation.OwnerID, true)
	if err != nil {
		return Template{}, fmt.Errorf("load presentation template: %w", err)
	}
	var manifest pptx.Manifest
	if err := json.Unmarshal(stored.Manifest, &manifest); err != nil {
		return Template{}, fmt.Errorf("template %q has an unreadable manifest", stored.Name)
	}
	if len(manifest.Layouts) == 0 {
		return Template{}, fmt.Errorf("template %q does not expose any usable layout", stored.Name)
	}
	return Template{ID: stored.ID, Name: stored.Name, Manifest: manifest}, nil
}

func (w *Worker) fail(ctx context.Context, presentation model.Presentation, cause error) error {
	message := truncate(cause.Error(), 1000)
	_ = w.store.FailGeneration(ctx, presentation.ID, message)
	details, _ := json.Marshal(map[string]any{"presentationId": presentation.ID, "ownerId": presentation.OwnerID})
	_ = w.store.CaptureIncident(ctx, model.Incident{UserID: stringPointer(presentation.OwnerID), Kind: "generation", Severity: "error", Message: message, Details: details})
	return cause
}

func stringPointer(value string) *string { return &value }
