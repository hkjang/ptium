package korean

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestTheParticleFollowsTheWord(t *testing.T) {
	for _, one := range []struct {
		word          string
		topic, object string
		subject, with string
	}{
		{"머리글", "은", "을", "이", "과"},
		{"보고서", "는", "를", "가", "와"},
		{"매출", "은", "을", "이", "과"},
		{"차트", "는", "를", "가", "와"},
		// A heading lifted out of somebody's file is often not Hangul at all,
		// and Korean reads it aloud before choosing: KPI is 케이피아이, Excel
		// is 엑셀, 2026 is 이천이십육.
		{"KPI", "는", "를", "가", "와"},
		{"Excel", "은", "을", "이", "과"},
		{"2026", "은", "을", "이", "과"},
		{"v2", "는", "를", "가", "와"},
		// %q wraps the word in quotation marks, and the word is what decides.
		{`"매출"`, "은", "을", "이", "과"},
		{`"차트"`, "는", "를", "가", "와"},
	} {
		if got := Topic(one.word); got != one.topic {
			t.Errorf("Topic(%q) = %q, want %q", one.word, got, one.topic)
		}
		if got := Object(one.word); got != one.object {
			t.Errorf("Object(%q) = %q, want %q", one.word, got, one.object)
		}
		if got := Subject(one.word); got != one.subject {
			t.Errorf("Subject(%q) = %q, want %q", one.word, got, one.subject)
		}
		if got := With(one.word); got != one.with {
			t.Errorf("With(%q) = %q, want %q", one.word, got, one.with)
		}
	}
}

// A particle written straight after a value is wrong about half the time, and
// the running product will never say so: "머리글"은 is a phrase either way. So
// the source is what gets read.
//
// A particle after a counter — %d개를, %d번은 — is not this mistake: the counter
// is a fixed word and the particle after it is fixed too.
var stuckOn = regexp.MustCompile(`%[sqvd]["'”’]?(을|를|이|가|은|는|와|과|으로)[\s.,)]`)

func TestNoMessageChoosesAParticleForAValueItCannotSee(t *testing.T) {
	root := filepath.Join("..", "..")
	var guilty []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for number, line := range strings.Split(string(body), "\n") {
			if found := stuckOn.FindString(line); found != "" {
				guilty = append(guilty, filepath.ToSlash(path)+":"+itoa(number+1)+" …"+strings.TrimSpace(found))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the server's own source: %v", err)
	}
	for _, one := range guilty {
		t.Errorf("a particle chosen for a value nobody has seen yet: %s", one)
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}
