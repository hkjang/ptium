package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/hkjang/ptium/server/internal/model"
)

// Reading an uploaded template is what fills the pod, so only one is read at a
// time.
//
// A template is held in memory whole — the package and every picture in it —
// and the setting that caps its size goes to 64 MB. Measured in a pod held to
// the manifest's limit, one of those peaks at 441 MiB. Four uploaded at the
// same moment killed the process outright: every request in flight died with
// it, and the four people who uploaded got nothing back. Queued one at a time,
// eight of them all succeed and the pod never passes half its limit.
func TestOnlyOneTemplateIsReadAtATime(t *testing.T) {
	server := &Server{building: semaphore.NewWeighted(heavyBudget)}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/templates", nil)

	release, ok := server.holdTemplateRead(httptest.NewRecorder(), request)
	if !ok {
		t.Fatal("the first upload was refused")
	}
	// A second one waits rather than reading alongside it.
	waited := make(chan bool, 1)
	go func() {
		second, ok := server.holdTemplateRead(httptest.NewRecorder(), request)
		if ok {
			second()
		}
		waited <- ok
	}()
	select {
	case <-waited:
		t.Fatal("a second template was read while the first still held the slot")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case ok := <-waited:
		if !ok {
			t.Error("the waiting upload was refused after the slot came free")
		}
	case <-time.After(2 * time.Second):
		t.Error("the waiting upload never got the slot")
	}
}

// A queue that never moves is answered rather than left hanging.
func TestAnUploadThatWaitsTooLongIsToldSo(t *testing.T) {
	previous := templateReadWait
	templateReadWait = 20 * time.Millisecond
	t.Cleanup(func() { templateReadWait = previous })

	server := &Server{building: semaphore.NewWeighted(heavyBudget)}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/templates", nil)
	held, ok := server.holdTemplateRead(httptest.NewRecorder(), request)
	if !ok {
		t.Fatal("the first upload was refused")
	}
	defer held()

	recorder := httptest.NewRecorder()
	if _, ok := server.holdTemplateRead(recorder, request); ok {
		t.Fatal("a second upload took a slot that was not free")
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("the caller was answered %d, want 503", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "templates_busy") {
		t.Errorf("the answer does not say why: %s", body)
	}
}

// And the handler takes the slot. Testing the gate on its own proves the gate;
// it does not prove that an upload goes through it, which is the thing that
// keeps the process alive.
func TestTheUploadHandlerTakesASlot(t *testing.T) {
	server := &Server{building: semaphore.NewWeighted(heavyBudget)}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/templates", strings.NewReader("not a template"))
	request.Header.Set("Content-Type", "multipart/form-data; boundary=nope")
	request = request.WithContext(withUser(request.Context(), model.User{ID: "u", IsAdmin: true}))
	recorder := httptest.NewRecorder()
	server.createTemplate(recorder, request)
	if server.templateReadsTaken.Load() != 1 {
		t.Error("the upload handler did not go through the gate")
	}
	// And it gave the room back, whatever it answered.
	if !server.building.TryAcquire(heavyBudget) {
		t.Error("the budget was never released")
	}
}

// Building documents is bounded by what they cost, not by how many there are.
//
// A PDF of a forty-slide deck with a photograph on every page costs about a
// third of what the same deck costs packaged as .pptx, which is assembled whole
// with every picture in it. Counting documents was the wrong bound: three PDFs
// fit comfortably where three .pptx files took the pod past its limit and
// killed it, and sixteen exports returned nothing at all.
func TestDocumentsAreBoundedByWhatTheyCost(t *testing.T) {
	server := &Server{building: semaphore.NewWeighted(heavyBudget)}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/presentations/x/export", nil)

	// One .pptx takes the whole budget.
	release, ok := server.holdBudget(httptest.NewRecorder(), request, costOfPPTX, printWait, "printing_busy", "busy")
	if !ok {
		t.Fatal("the first .pptx was refused")
	}
	previous := printWait
	printWait = 20 * time.Millisecond
	t.Cleanup(func() { printWait = previous })
	recorder := httptest.NewRecorder()
	if _, ok := server.holdBudget(recorder, request, costOfPDF, printWait, "printing_busy", "busy"); ok {
		t.Error("a PDF was drawn while a .pptx had the whole budget")
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("the caller was answered %d, want 503", recorder.Code)
	}
	release()

	// Three PDFs fit where one .pptx did.
	held := make([]func(), 0, heavyBudget)
	for range heavyBudget {
		release, ok := server.holdBudget(httptest.NewRecorder(), request, costOfPDF, printWait, "printing_busy", "busy")
		if !ok {
			t.Fatal("a PDF was refused while the budget had room")
		}
		held = append(held, release)
	}
	if _, ok := server.holdBudget(httptest.NewRecorder(), request, costOfPDF, printWait, "printing_busy", "busy"); ok {
		t.Error("a fourth PDF drew past the budget")
	}
	for _, release := range held {
		release()
	}
	if release, ok := server.holdBudget(httptest.NewRecorder(), request, costOfPPTX, printWait, "printing_busy", "busy"); !ok {
		t.Error("a .pptx was refused after the budget came free")
	} else {
		release()
	}
}

// An upload that is too big is told the limit, not that reading stopped.
//
// The reader enforcing the limit trips before the check that writes a clear
// answer ever runs, so a person uploading a 60 MB deck to a deployment that
// accepts 32 got "The upload could not be read", with "http: request body too
// large" tucked into a details field. Both doors — a template and an imported
// deck — went through the same reader and gave the same non-answer.
func TestAnOversizeUploadIsToldTheLimit(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/templates", nil)
	if !refusedForSize(recorder, request, &http.MaxBytesError{Limit: 33554432}, 32<<20) {
		t.Fatal("an over-size upload was not recognised as one")
	}
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("answered %d, want 413", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "32 MiB") {
		t.Errorf("the answer does not name the limit: %s", body)
	}
	if !strings.Contains(body, "limitBytes") {
		t.Errorf("the answer does not carry the limit as a number: %s", body)
	}
	// Anything else keeps its own answer.
	other := httptest.NewRecorder()
	if refusedForSize(other, request, errors.New("connection reset"), 32<<20) {
		t.Error("an unrelated failure was reported as an over-size upload")
	}
}
