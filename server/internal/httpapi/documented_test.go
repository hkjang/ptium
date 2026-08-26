package httpapi

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// An answer that says nothing about its own shape is an answer nobody can code
// against.
//
// Eight of this API's replies — what the deployment is holding, the audit
// trail, the state of the model host, the generation queue and the two buttons
// that act on it — documented a status and a sentence and no shape at all. A
// client had to send the request to find out what came back, and a client that
// validates replies had nothing to validate against.
func TestEveryAnswerSaysWhatItSends(t *testing.T) {
	schema, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("openapi.yaml: %v", err)
	}
	lines := strings.Split(string(schema), "\n")
	answer := regexp.MustCompile(`^\s+'(200|201)':\s*(\{.*\})?\s*$`)
	var offenders []string
	route := ""
	for index, line := range lines {
		if strings.HasPrefix(line, "  /") {
			route = strings.TrimSpace(strings.TrimSuffix(line, ":"))
		}
		found := answer.FindStringSubmatch(line)
		if found == nil {
			continue
		}
		// An inline answer carries nothing but a sentence.
		if found[2] != "" && !strings.Contains(found[2], "content") {
			offenders = append(offenders, route+" "+found[1])
			continue
		}
		// A block answer has to name a content type before the next response.
		said := false
		for ahead := index + 1; ahead < len(lines); ahead++ {
			next := strings.TrimSpace(lines[ahead])
			if next == "" {
				continue
			}
			if regexp.MustCompile(`^'\d{3}':`).MatchString(next) || strings.HasPrefix(lines[ahead], "  /") ||
				strings.HasPrefix(lines[ahead], "components:") {
				break
			}
			if next == "content:" {
				said = true
				break
			}
		}
		if !said {
			offenders = append(offenders, route+" "+found[1])
		}
	}
	for _, one := range offenders {
		t.Errorf("%s answers without saying what it sends", one)
	}
}
