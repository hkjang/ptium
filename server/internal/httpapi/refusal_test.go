package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// A deployment describing how it is set up is not a deployment failing.
//
// A site with no model answers 503 when somebody asks for another draft of a
// slide. That answer is correct and it tells the caller what to do — and every
// one of them was landing in the error centre as an operator-facing error. With
// the deck's id in the path each one opened its own group, so five refusals for
// one missing setting read as five separate faults.
func TestAConfigurationRefusalIsNotRecordedAsAFault(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/presentations/x/slides/1/revise", nil)
	recorder := &responseRecorder{ResponseWriter: httptest.NewRecorder()}
	writeError(recorder, request, http.StatusServiceUnavailable, "ai_unavailable",
		"This deployment has no AI provider configured", nil)
	if !recorder.refusal {
		t.Error("a refusal the product meant to give would still be recorded as a fault")
	}
	if recorder.status != http.StatusServiceUnavailable {
		t.Errorf("the caller was answered %d", recorder.status)
	}

	// Anything else that fails with a 5xx is still a fault worth recording.
	broken := &responseRecorder{ResponseWriter: httptest.NewRecorder()}
	writeError(broken, request, http.StatusInternalServerError, "internal_error",
		"The server could not complete the request", nil)
	if broken.refusal {
		t.Error("a genuine failure was treated as a refusal and would go unrecorded")
	}
}
