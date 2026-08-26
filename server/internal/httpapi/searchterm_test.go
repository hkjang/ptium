package httpapi

import (
	"net/http/httptest"
	"testing"
)

// Half of this API called it "q" and half of it "search", and the name that was
// not read was ignored in silence — a client searching the template library
// with "q" was handed every template and told nothing.
func TestBothNamesForTheSameQuestionAreRead(t *testing.T) {
	cases := map[string]string{
		"/x?q=클라우드":                     "클라우드",
		"/x?search=클라우드":                "클라우드",
		"/x?q=&search=클라우드":             "클라우드",
		"/x?q=클라우드&search=something":    "클라우드",
		"/x?q=%20%20":                   "",
		"/x":                            "",
		"/x?q=metadata_demo":            "metadata_demo",
		"/x?search=%20metadata_demo%20": "metadata_demo",
	}
	for target, want := range cases {
		if got := searchTerm(httptest.NewRequest("GET", target, nil)); got != want {
			t.Errorf("searchTerm(%q) = %q, want %q", target, got, want)
		}
	}
}
