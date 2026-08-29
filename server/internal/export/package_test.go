package export

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/hkjang/ptium/server/internal/deck"
	"github.com/hkjang/ptium/server/internal/model"
	"github.com/hkjang/ptium/server/internal/pptx"
)

// What a downloaded deck owes PowerPoint before it owes it anything else: it
// has to open.
//
// A package that opens is one where every relationship points at a part that is
// there, every part is declared in [Content_Types].xml, every part parses, and
// every r:id a slide uses is in that slide's own relationships. Break any of
// them and PowerPoint offers to repair the file, which is the worst thing this
// product can hand somebody: a deck they cannot trust before they have read a
// word of it.
//
// The tests beside this one each check one relationship they care about. None
// of them would notice a part left undeclared, or a slide naming a picture that
// is not in the file, so the deck is read here as a package.

// oneOfEach draws every component this product ships, so a defect that belongs
// to one of them cannot hide behind the fourteen that are fine.
const oneOfEach = `# 표지
- 여는 줄

# 목록
::bullets 요점
- 첫 항목
- 둘째 항목
::

# 지표
::kpi 핵심 지표
- 매출 | 128억 | +24%
- 고객 | 312곳 | +18%
::

# 대표 숫자
::hero 가동률
- 가동률 | 99.95% | 목표 달성
::

# 단계
::steps 절차
- 준비 | 자료 모으기
- 실행 | 유통망 확대
::

# 일정
::timeline 로드맵
- 1분기 | 착수
- 3분기 | 확장
::

# 비교
::comparison 전후
- 지금 | 수작업
- 이후 | 자동화
::

# 세로막대
::columns 지역별
- 서울 | 42
- 부산 | 27
::

# 가로막대
::hbars 경로별
- 검색 | 61
- 추천 | 23
::

# 추이
::line 월별 처리량
- 월 | 1월, 2월, 3월
- 처리량 | 12, 19, 31
::

# 비중
::share 매출 비중
- 국내 | 68
- 해외 | 32
::

# 달성률
::meter 예산
- 예산 집행 | 74
::

# 표
::table 연간 비용
- 항목 | 작년 | 올해
- 매출 | 103억 | 128억
::

# 격자
::grid raci 전환
- 설계 | 담당 | 검토
- 개발 | 검토 | 담당
::

# 인용
::quote 한마디
- 준비된 자료가 회의를 짧게 만듭니다
::

# 강조
::callout 먼저 할 것
- 예산 승인이 먼저 필요합니다
::

# 마무리
!notes 예산 질문이 나오면 이 장에서 답합니다
- [둘째 장으로](#2) 돌아갑니다
- 바깥 [안내](https://example.com/g) 도 있습니다
`

func TestEveryDesignWritesAPackagePowerPointCanOpen(t *testing.T) {
	for _, design := range []string{"slate-classic", "midnight-wash", "azure-classic", "forest-orbit"} {
		t.Run(design, func(t *testing.T) {
			data, err := pptx.BuiltinTemplate(design)
			if err != nil {
				t.Fatalf("BuiltinTemplate(%q): %v", design, err)
			}
			_, manifest, err := pptx.AnalyzeBytes(data)
			if err != nil {
				t.Fatalf("AnalyzeBytes: %v", err)
			}
			compiled := deck.Compile(deck.ParseSource(oneOfEach), manifest, deck.CompileOptions{Language: "ko"})
			file, err := PPTX(model.Presentation{Title: "구성 요소 전부", Language: "ko", Slides: compiled.Slides},
				Options{TemplateData: data, Manifest: manifest, Author: "Ptium"})
			if err != nil {
				t.Fatalf("PPTX: %v", err)
			}
			// A file with nothing in it passes every check below, so what the
			// checks are reading is measured first.
			if held := whatItHolds(t, file); held.slides < 15 || held.charts < 1 || held.rels < 5 {
				t.Fatalf("%s: the deck under test holds %d slides, %d charts and %d relationship files",
					design, held.slides, held.charts, held.rels)
			}
			for _, complaint := range unopenable(t, file) {
				t.Errorf("%s: %s", design, complaint)
			}
		})
	}
}

type relationships struct {
	Items []struct {
		ID         string `xml:"Id,attr"`
		Target     string `xml:"Target,attr"`
		TargetMode string `xml:"TargetMode,attr"`
	} `xml:"Relationship"`
}

type contentTypes struct {
	Defaults []struct {
		Extension string `xml:"Extension,attr"`
	} `xml:"Default"`
	Overrides []struct {
		PartName string `xml:"PartName,attr"`
	} `xml:"Override"`
}

var usesRelationship = regexp.MustCompile(`r:(?:id|embed|link)="([^"]+)"`)

// unopenable reads the package the way a reader has to and says what it cannot
// resolve. An empty result is a file that opens.
func unopenable(t *testing.T, file []byte) []string {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		return []string{fmt.Sprintf("the file is not a zip at all: %v", err)}
	}
	parts := map[string][]byte{}
	for _, entry := range archive.File {
		handle, err := entry.Open()
		if err != nil {
			return []string{fmt.Sprintf("%s cannot be opened: %v", entry.Name, err)}
		}
		body, err := io.ReadAll(handle)
		handle.Close()
		if err != nil {
			return []string{fmt.Sprintf("%s cannot be read: %v", entry.Name, err)}
		}
		parts[entry.Name] = body
	}

	var complaints []string
	for name, body := range parts {
		if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
			continue
		}
		if err := xml.Unmarshal(body, new(struct{})); err != nil {
			complaints = append(complaints, fmt.Sprintf("%s is not XML a reader can parse: %v", name, err))
		}
	}

	// Every relationship names a part that is in the file.
	for name, body := range parts {
		if !strings.HasSuffix(name, ".rels") {
			continue
		}
		var found relationships
		if err := xml.Unmarshal(body, &found); err != nil {
			continue // already reported above
		}
		from := path.Dir(path.Dir(name))
		for _, one := range found.Items {
			if one.TargetMode == "External" || strings.HasPrefix(one.Target, "http") ||
				strings.HasPrefix(one.Target, "mailto:") || strings.HasPrefix(one.Target, "#") {
				continue
			}
			landed := path.Clean(path.Join(from, one.Target))
			if _, ok := parts[landed]; !ok {
				complaints = append(complaints,
					fmt.Sprintf("%s points at %s, which is not in the file", name, one.Target))
			}
		}
	}

	// Every part is declared, or PowerPoint does not know what it is holding.
	var declared contentTypes
	if err := xml.Unmarshal(parts["[Content_Types].xml"], &declared); err != nil {
		complaints = append(complaints, "the file does not declare what it holds")
	}
	extensions := map[string]bool{}
	for _, one := range declared.Defaults {
		extensions[strings.ToLower(one.Extension)] = true
	}
	named := map[string]bool{}
	for _, one := range declared.Overrides {
		named[one.PartName] = true
	}
	for name := range parts {
		if name == "[Content_Types].xml" || strings.Contains(name, "_rels/") || strings.HasSuffix(name, "/") {
			continue
		}
		if named["/"+name] {
			continue
		}
		dot := strings.LastIndex(name, ".")
		if dot < 0 || !extensions[strings.ToLower(name[dot+1:])] {
			complaints = append(complaints, fmt.Sprintf("%s is in the file but never declared", name))
		}
	}

	// Every r:id a slide uses is one of its own relationships.
	for name, body := range parts {
		if !strings.HasPrefix(name, "ppt/slides/slide") || !strings.HasSuffix(name, ".xml") {
			continue
		}
		var own relationships
		xml.Unmarshal(parts["ppt/slides/_rels/"+path.Base(name)+".rels"], &own)
		has := map[string]bool{}
		for _, one := range own.Items {
			has[one.ID] = true
		}
		for _, use := range usesRelationship.FindAllStringSubmatch(string(body), -1) {
			if !has[use[1]] {
				complaints = append(complaints, fmt.Sprintf("%s uses %s, which it does not have", name, use[1]))
			}
		}
	}
	return complaints
}

type holding struct{ parts, slides, charts, rels int }

// whatItHolds counts what the package came out with, so a check that finds
// nothing wrong is a check that had something to read.
func whatItHolds(t *testing.T, file []byte) holding {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatalf("the file is not a zip: %v", err)
	}
	var held holding
	for _, entry := range archive.File {
		held.parts++
		switch {
		case strings.HasPrefix(entry.Name, "ppt/slides/slide") && strings.HasSuffix(entry.Name, ".xml"):
			held.slides++
		case strings.HasPrefix(entry.Name, "ppt/charts/chart") && strings.HasSuffix(entry.Name, ".xml"):
			held.charts++
		case strings.HasSuffix(entry.Name, ".rels"):
			held.rels++
		}
	}
	return held
}

// And the check itself is worth no more than what it notices. Each of these is
// a way a package stops opening; a reader handed one of them offers to repair
// the file. If any of them passes here, the test above is decoration.
func TestThePackageCheckNoticesABrokenFile(t *testing.T) {
	data, err := pptx.BuiltinTemplate("slate-classic")
	if err != nil {
		t.Fatal(err)
	}
	_, manifest, err := pptx.AnalyzeBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	compiled := deck.Compile(deck.ParseSource(oneOfEach), manifest, deck.CompileOptions{Language: "ko"})
	sound, err := PPTX(model.Presentation{Title: "구성 요소 전부", Language: "ko", Slides: compiled.Slides},
		Options{TemplateData: data, Manifest: manifest, Author: "Ptium"})
	if err != nil {
		t.Fatalf("PPTX: %v", err)
	}
	if complaints := unopenable(t, sound); len(complaints) != 0 {
		t.Fatalf("the file was already broken: %v", complaints)
	}

	for _, breakage := range []struct {
		what   string
		breaks func(name string, body []byte) (string, []byte, bool)
	}{
		{"a part a relationship names is gone", func(name string, body []byte) (string, []byte, bool) {
			return name, body, name != "ppt/slides/slide2.xml"
		}},
		// A picture is added as ppt/media/…, and its extension has to be
		// declared or the reader does not know what the bytes are. Removing an
		// Override would not show this: a Default for the extension still
		// covers the part, which is the rule OPC actually has.
		{"a part is never declared", func(name string, body []byte) (string, []byte, bool) {
			return name, body, true
		}},
		{"a slide names a relationship it has not got", func(name string, body []byte) (string, []byte, bool) {
			if name != "ppt/slides/slide2.xml" {
				return name, body, true
			}
			return name, bytes.Replace(body, []byte("<p:sp>"), []byte(`<p:sp r:id="rIdNope">`), 1), true
		}},
		{"a part is not XML any more", func(name string, body []byte) (string, []byte, bool) {
			if name != "ppt/slides/slide2.xml" {
				return name, body, true
			}
			return name, []byte("<p:sld><not closed>"), true
		}},
	} {
		t.Run(breakage.what, func(t *testing.T) {
			broken := rewritten(t, sound, breakage.breaks)
			if breakage.what == "a part is never declared" {
				broken = withUndeclaredPart(t, broken)
			}
			if complaints := unopenable(t, broken); len(complaints) == 0 {
				t.Errorf("the check did not notice that %s", breakage.what)
			}
		})
	}
}

// rewritten builds the package again, letting each part be changed or dropped.
func rewritten(t *testing.T, file []byte, change func(string, []byte) (string, []byte, bool)) []byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, entry := range archive.File {
		handle, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(handle)
		handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		name, body, keep := change(entry.Name, body)
		if !keep {
			continue
		}
		part, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// withUndeclaredPart adds bytes nothing in the package says how to read.
func withUndeclaredPart(t *testing.T, file []byte) []byte {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for _, entry := range archive.File {
		handle, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(handle)
		handle.Close()
		if err != nil {
			t.Fatal(err)
		}
		part, err := writer.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	stray, err := writer.Create("ppt/media/image1.unknownext")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stray.Write([]byte("bytes nobody declared")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
