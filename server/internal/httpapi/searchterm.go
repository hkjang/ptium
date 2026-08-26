package httpapi

import (
	"net/http"
	"strings"
)

// searchTerm reads what the caller is looking for.
//
// Half of this API called it "q" and half of it "search" — decks, images and
// saved slides one way, templates, users and the audit trail the other — and
// the name that was not read was ignored in silence. A client searching the
// template library with "q" was handed all fifty templates and had no way to
// know its filter had been dropped: the wrong answer to a question, dressed as
// the right one.
//
// Both names are read now, "q" first, so nobody has to remember which door
// they are at.
func searchTerm(request *http.Request) string {
	query := request.URL.Query()
	if said := strings.TrimSpace(query.Get("q")); said != "" {
		return said
	}
	return strings.TrimSpace(query.Get("search"))
}
