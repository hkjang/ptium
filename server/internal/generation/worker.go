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
	"strings"
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
	presentation, lease, err := w.store.ClaimGeneration(ctx)
	if err != nil {
		return err
	}
	// While this deck is being written, say so twice a minute. A worker that
	// stops saying it has gone, and its deck may be taken; a worker whose deck
	// was taken from it — stopped, requeued, claimed by somebody else — stops
	// rather than racing whoever holds it now.
	ctx, done := context.WithCancel(ctx)
	defer done()
	go w.beat(ctx, presentation.ID, lease, done)
	profile, err := w.store.GetProfile(ctx, presentation.OwnerID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return w.fail(ctx, presentation, lease, err)
	}
	template, err := w.resolveTemplate(ctx, presentation)
	if err != nil {
		return w.fail(ctx, presentation, lease, err)
	}
	// A deck queued with text in it is being rewritten — it was brought in from a
	// file, or written here, and someone asked for it to be improved. A deck with
	// no text is one being written for the first time. Generating from the brief
	// over a deck that already has slides would throw away what it is.
	generate := w.generator.Generate
	if strings.TrimSpace(presentation.Source) != "" {
		generate = w.generator.Rewrite
	}
	generated, err := generate(ctx, presentation, profile, template)
	if err != nil {
		return w.fail(ctx, presentation, lease, err)
	}
	if err := w.store.CompleteGeneration(ctx, presentation.ID, lease, generated.Outline, generated.Slides,
		generated.Source, generated.Notes); err != nil {
		return w.fail(ctx, presentation, lease, err)
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

// beat says this worker is still writing the deck, until the work is over or
// the lease is somebody else's.
func (w *Worker) beat(ctx context.Context, id, lease string, lost func()) {
	ticker := time.NewTicker(store.GenerationSilence / 6)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			held, err := w.store.HeartbeatGeneration(context.WithoutCancel(ctx), id, lease)
			if err != nil {
				continue
			}
			if !held {
				w.logger.Warn("this deck is no longer ours to write", "presentation_id", id)
				lost()
				return
			}
		}
	}
}

func (w *Worker) fail(ctx context.Context, presentation model.Presentation, lease string, cause error) error {
	// Two readers, two messages. The operator gets the cause as it happened; the
	// author gets what kind of thing went wrong, whether trying again is worth
	// it, and who to ask — in the language they asked for the deck in, and
	// without the address of an internal service in it.
	message := truncate(cause.Error(), 1000)
	// The deck may have been taken away while this attempt was running, and the
	// context it was running in cancelled with it — the failure still has to be
	// written, and FailGeneration itself refuses if the lease moved on.
	_ = w.store.FailGeneration(context.WithoutCancel(ctx), presentation.ID, lease, AuthorMessage(cause, presentation.Language))
	details, _ := json.Marshal(map[string]any{"presentationId": presentation.ID, "ownerId": presentation.OwnerID})
	_ = w.store.CaptureIncident(context.WithoutCancel(ctx), model.Incident{UserID: stringPointer(presentation.OwnerID), Kind: "generation", Severity: "error", Message: message, Details: details})
	return cause
}

func stringPointer(value string) *string { return &value }
