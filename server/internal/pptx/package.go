// Package pptx reads PowerPoint templates and rebuilds them into finished
// decks. It works directly on the Office Open XML package so that every
// design decision a customer already made — masters, layouts, theme colors,
// fonts, logos and placeholder geometry — survives generation untouched.
package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// MaxPackageBytes bounds both the compressed upload and the inflated package
// so a malicious archive cannot exhaust server memory.
const MaxPackageBytes = 64 << 20

// Package is an in-memory Office Open XML container. Part names are stored
// without a leading slash, exactly as they appear in the zip archive.
type Package struct {
	parts map[string][]byte
	order []string
}

// Open reads an OOXML package from raw bytes.
func Open(data []byte) (*Package, error) {
	if len(data) == 0 {
		return nil, errors.New("the uploaded file is empty")
	}
	if len(data) > MaxPackageBytes {
		return nil, fmt.Errorf("the uploaded file exceeds the %d MiB limit", MaxPackageBytes>>20)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("the uploaded file is not a valid PowerPoint package")
	}
	result := &Package{parts: make(map[string][]byte, len(reader.File))}
	var total int
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := path.Clean(strings.TrimPrefix(file.Name, "/"))
		if name == "." || strings.HasPrefix(name, "..") {
			return nil, fmt.Errorf("package entry %q is not a valid part name", file.Name)
		}
		if _, exists := result.parts[name]; exists {
			return nil, fmt.Errorf("package contains duplicate part %q", name)
		}
		total += int(file.UncompressedSize64)
		if total > MaxPackageBytes || file.UncompressedSize64 > MaxPackageBytes {
			return nil, fmt.Errorf("the package expands beyond the %d MiB limit", MaxPackageBytes>>20)
		}
		handle, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		content, err := io.ReadAll(io.LimitReader(handle, MaxPackageBytes+1))
		_ = handle.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		result.parts[name] = content
		result.order = append(result.order, name)
	}
	if _, ok := result.parts["[Content_Types].xml"]; !ok {
		return nil, errors.New("the uploaded file is not a valid PowerPoint package")
	}
	if _, ok := result.parts["ppt/presentation.xml"]; !ok {
		return nil, errors.New("the package does not contain a PowerPoint presentation")
	}
	return result, nil
}

// Part returns the raw bytes of a part.
func (p *Package) Part(name string) ([]byte, bool) {
	value, ok := p.parts[strings.TrimPrefix(name, "/")]
	return value, ok
}

// Text returns a part as a string.
func (p *Package) Text(name string) (string, bool) {
	value, ok := p.Part(name)
	return string(value), ok
}

// Set writes or replaces a part, preserving a stable ordering for new parts.
func (p *Package) Set(name string, content []byte) {
	name = strings.TrimPrefix(name, "/")
	if _, exists := p.parts[name]; !exists {
		p.order = append(p.order, name)
	}
	p.parts[name] = content
}

// SetText writes or replaces a textual part.
func (p *Package) SetText(name, content string) { p.Set(name, []byte(content)) }

// Delete removes a part.
func (p *Package) Delete(name string) {
	name = strings.TrimPrefix(name, "/")
	if _, exists := p.parts[name]; !exists {
		return
	}
	delete(p.parts, name)
	for index, existing := range p.order {
		if existing == name {
			p.order = append(p.order[:index], p.order[index+1:]...)
			break
		}
	}
}

// Names lists every part in package order.
func (p *Package) Names() []string {
	result := make([]string, len(p.order))
	copy(result, p.order)
	return result
}

// NamesUnder lists parts inside a directory prefix, sorted for determinism.
func (p *Package) NamesUnder(prefix string) []string {
	var result []string
	for _, name := range p.order {
		if strings.HasPrefix(name, prefix) {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result
}

// Bytes serializes the package back into a zip archive.
func (p *Package) Bytes() ([]byte, error) {
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	// Content types must be the first entry for maximum reader compatibility.
	names := append([]string{}, p.order...)
	sort.SliceStable(names, func(i, j int) bool {
		return names[i] == "[Content_Types].xml" && names[j] != "[Content_Types].xml"
	})
	for _, name := range names {
		entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
		if _, err := entry.Write(p.parts[name]); err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Clone returns a deep copy so a stored template can be rendered repeatedly.
func (p *Package) Clone() *Package {
	result := &Package{parts: make(map[string][]byte, len(p.parts)), order: append([]string{}, p.order...)}
	for name, content := range p.parts {
		copied := make([]byte, len(content))
		copy(copied, content)
		result.parts[name] = copied
	}
	return result
}

// Relationship is a single entry of an OOXML `.rels` part.
type Relationship struct {
	ID         string
	Type       string
	Target     string
	TargetMode string
}

// ShortType is the final segment of the relationship type URI, for example
// "slideLayout" or "theme".
func (r Relationship) ShortType() string {
	index := strings.LastIndex(r.Type, "/")
	if index < 0 {
		return r.Type
	}
	return r.Type[index+1:]
}

// RelationshipsPath maps a part name to the name of its relationships part.
func RelationshipsPath(partName string) string {
	partName = strings.TrimPrefix(partName, "/")
	dir, file := path.Split(partName)
	return dir + "_rels/" + file + ".rels"
}

// Relationships parses the relationships declared by a part.
func (p *Package) Relationships(partName string) []Relationship {
	content, ok := p.Part(RelationshipsPath(partName))
	if !ok {
		return nil
	}
	var parsed struct {
		Relationships []struct {
			ID         string `xml:"Id,attr"`
			Type       string `xml:"Type,attr"`
			Target     string `xml:"Target,attr"`
			TargetMode string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(content, &parsed) != nil {
		return nil
	}
	result := make([]Relationship, 0, len(parsed.Relationships))
	for _, item := range parsed.Relationships {
		result = append(result, Relationship{ID: item.ID, Type: item.Type, Target: item.Target, TargetMode: item.TargetMode})
	}
	return result
}

// Resolve turns a relationship target into an absolute part name.
func Resolve(sourcePart, target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(target, "/")
	}
	return path.Clean(path.Join(path.Dir(strings.TrimPrefix(sourcePart, "/")), target))
}

// RelatedPart returns the absolute name of the first related part with the
// given short relationship type.
func (p *Package) RelatedPart(sourcePart, shortType string) (string, bool) {
	for _, relationship := range p.Relationships(sourcePart) {
		if relationship.TargetMode == "External" {
			continue
		}
		if relationship.ShortType() == shortType {
			return Resolve(sourcePart, relationship.Target), true
		}
	}
	return "", false
}

// RelatedParts returns every related part with the given short relationship
// type, in declaration order.
func (p *Package) RelatedParts(sourcePart, shortType string) []string {
	var result []string
	for _, relationship := range p.Relationships(sourcePart) {
		if relationship.TargetMode == "External" || relationship.ShortType() != shortType {
			continue
		}
		result = append(result, Resolve(sourcePart, relationship.Target))
	}
	return result
}

// RelationshipByID resolves a relationship identifier to a part name.
func (p *Package) RelationshipByID(sourcePart, id string) (string, bool) {
	for _, relationship := range p.Relationships(sourcePart) {
		if relationship.ID == id && relationship.TargetMode != "External" {
			return Resolve(sourcePart, relationship.Target), true
		}
	}
	return "", false
}
