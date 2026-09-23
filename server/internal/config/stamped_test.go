package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every file that names the release names the version we are shipping.
//
// VERSION is not the only place the release number is written: the schema
// reports it as service.version, the manifest runs that image tag, and the
// offline install note tells an operator which archive to load and which image
// to inspect. Stamping only some of them ships a note that sends a site after
// an asset that was never published, and the only thing watching for that today
// is scripts/release.sh — after the bundle has been built, and for the archive
// line alone. A half-stamped release should be red here, in a second, long
// before anybody waits on a docker build.
func TestEveryFileThatNamesTheReleaseNamesTheVersionWeShip(t *testing.T) {
	t.Parallel()
	root := "../../.."
	raw, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	version := strings.TrimSpace(string(raw))
	if version == "" {
		t.Fatal("VERSION names no version")
	}

	// The install note writes the version inside file and image names
	// (ptium-1.2.3.tar.gz, ptium:1.2.3), so what follows the version there is
	// part of the name rather than part of the number. The other two files
	// carry it on its own. Nothing here matches a bare number: the note also
	// describes older releases on purpose ("written before 1.52.1"), and those
	// are not stamps.
	sources := []struct {
		file    string
		pattern *regexp.Regexp
		named   bool
	}{
		{"api/openapi.yaml", regexp.MustCompile(`service\.version: (\S+)`), false},
		{"deploy/kubernetes.yaml", regexp.MustCompile(`image: ptium:(\S+)`), false},
		{"docs/offline-deployment.md", regexp.MustCompile(`ptium[-:]([0-9][0-9A-Za-z.+-]*)`), true},
	}
	for _, source := range sources {
		body, err := os.ReadFile(filepath.Join(root, source.file))
		if err != nil {
			t.Errorf("read %s: %v", source.file, err)
			continue
		}
		stamps := 0
		for number, line := range strings.Split(string(body), "\n") {
			for _, found := range source.pattern.FindAllStringSubmatch(line, -1) {
				stamps++
				if namesTheRelease(found[1], version, source.named) {
					continue
				}
				t.Errorf("%s:%d says %q, and VERSION says %s", source.file, number+1, found[0], version)
			}
		}
		if stamps == 0 {
			t.Errorf("%s no longer names the release version anywhere; this check has stopped watching it", source.file)
		}
	}
}

// Whether a version read out of a file is the one being shipped. Inside a file
// or image name the version is followed by the rest of the name, which is not
// part of the number: ptium-1.2.3.tar.gz names 1.2.3, and ptium-1.2.30.tar.gz
// does not.
func namesTheRelease(found, version string, named bool) bool {
	if found == version {
		return true
	}
	if !named || !strings.HasPrefix(found, version) {
		return false
	}
	next := found[len(version)]
	return !('0' <= next && next <= '9') && !('a' <= next && next <= 'z') && !('A' <= next && next <= 'Z')
}
