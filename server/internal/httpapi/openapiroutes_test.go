package httpapi

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// api/openapi.yaml is what this product tells whoever writes against it. It is
// written by hand beside the routes, and nothing held the two together: five
// endpoints answered 200 and appeared nowhere in it — the two export routes,
// the second way to write a deck's source, one way to delete an API key, and
// the call a browser makes to find out who it is signed in as.
//
// A path missing from the spec is not a broken endpoint. It is an endpoint
// nobody outside this repository can know exists.
//
// Skipped when the spec is not beside the server.
func TestEveryRouteIsInTheSpecThatDescribesThem(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Skip("the API description is not beside the server here")
	}
	routes, err := os.ReadFile(filepath.Join("server.go"))
	if err != nil {
		t.Fatal(err)
	}

	// Routes registered with an explicit method. A handler mounted for every
	// method — the MCP endpoint — names no method and is described on its own.
	registered := map[string]map[string]bool{}
	for _, found := range regexp.MustCompile(`"(GET|POST|PUT|PATCH|DELETE) (/\S*?)"`).
		FindAllStringSubmatch(string(routes), -1) {
		path := placeholders(found[2])
		if registered[path] == nil {
			registered[path] = map[string]bool{}
		}
		registered[path][found[1]] = true
	}

	described := map[string]map[string]bool{}
	var path string
	inPaths := false
	for _, line := range strings.Split(string(spec), "\n") {
		if strings.TrimRight(line, " ") == "paths:" {
			inPaths = true
			continue
		}
		if inPaths && len(line) > 0 && !strings.HasPrefix(line, " ") {
			inPaths = false
		}
		if !inPaths {
			continue
		}
		if found := regexp.MustCompile(`^  (/\S*):\s*$`).FindStringSubmatch(line); found != nil {
			path = placeholders(found[1])
			if described[path] == nil {
				described[path] = map[string]bool{}
			}
			continue
		}
		if found := regexp.MustCompile(`^    (get|post|put|patch|delete):\s*$`).FindStringSubmatch(line); found != nil && path != "" {
			described[path][strings.ToUpper(found[1])] = true
		}
	}

	var undescribed []string
	for route, methods := range registered {
		for method := range methods {
			if !described[route][method] {
				undescribed = append(undescribed, method+" "+route)
			}
		}
	}
	sort.Strings(undescribed)
	if len(undescribed) > 0 {
		t.Errorf("these routes answer and are in no API description: %v", undescribed)
	}

	// And the other way: a description of something that does not answer is a
	// promise this product does not keep.
	var unbuilt []string
	for route, methods := range described {
		if route == "/mcp" {
			continue // mounted for every method rather than registered per method
		}
		for method := range methods {
			if !registered[route][method] {
				unbuilt = append(unbuilt, method+" "+route)
			}
		}
	}
	sort.Strings(unbuilt)
	if len(unbuilt) > 0 {
		t.Errorf("these routes are described and do not answer: %v", unbuilt)
	}

	// And every $ref names something this file holds. Two operations pointed at
	// a response component that was never written, so anything generating a
	// client from this description either failed on them or quietly dropped the
	// answer they describe.
	held := map[string]bool{}
	section := ""
	for _, line := range strings.Split(string(spec), "\n") {
		if found := regexp.MustCompile(`^  (\w+):\s*$`).FindStringSubmatch(line); found != nil {
			section = found[1]
			continue
		}
		if found := regexp.MustCompile(`^    (\w+):\s*$`).FindStringSubmatch(line); found != nil && section != "" {
			held["#/components/"+section+"/"+found[1]] = true
		}
	}
	var dangling []string
	for _, found := range regexp.MustCompile(`\$ref: '(#/components/[^']+)'`).
		FindAllStringSubmatch(string(spec), -1) {
		if !held[found[1]] {
			dangling = append(dangling, found[1])
		}
	}
	sort.Strings(dangling)
	if len(dangling) > 0 {
		t.Errorf("these descriptions point at nothing: %v", unique(dangling))
	}
}

// unique keeps one of each, because a broken reference used twice is one thing
// to fix.
func unique(values []string) []string {
	seen := map[string]bool{}
	var kept []string
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			kept = append(kept, value)
		}
	}
	return kept
}

// placeholders reads {id} and {token} as the same shape of route, because the
// spec names a path parameter for what it is and a route names it for how it is
// read.
func placeholders(route string) string {
	return strings.TrimSuffix(regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(route, "{}"), "/")
}
