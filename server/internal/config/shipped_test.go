package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every setting this product ships in its deployment files is one the server
// reads.
//
// The offline bundle hands an operator a ConfigMap and an .env example, and
// those are the only description of how to configure the service that reaches a
// site with no internet. A name that the server stopped reading — renamed,
// folded into another, dropped — goes on sitting in the file that the operator
// copies, quietly doing nothing: they set their administrator roles and nobody
// is an administrator.
func TestEverySettingWeShipIsOneTheServerReads(t *testing.T) {
	t.Parallel()
	root := "../../.."
	source := serverSource(t, filepath.Join(root, "server"))

	shipped := map[string][]string{}
	manifest, err := os.ReadFile(filepath.Join(root, "deploy/kubernetes.yaml"))
	if err != nil {
		t.Fatalf("read the manifest: %v", err)
	}
	// The ConfigMap block, which is the part an operator edits.
	for _, line := range strings.Split(string(manifest), "\n") {
		if found := regexp.MustCompile(`^\s{2}([A-Z][A-Z0-9_]{2,}):`).FindStringSubmatch(line); found != nil {
			shipped["deploy/kubernetes.yaml"] = append(shipped["deploy/kubernetes.yaml"], found[1])
		}
	}
	example, err := os.ReadFile(filepath.Join(root, ".env.offline.example"))
	if err != nil {
		t.Fatalf("read the env example: %v", err)
	}
	for _, found := range regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]{2,})=`).FindAllStringSubmatch(string(example), -1) {
		shipped[".env.offline.example"] = append(shipped[".env.offline.example"], found[1])
	}
	if len(shipped) != 2 {
		t.Fatalf("nothing was read out of the deployment files: %v", shipped)
	}

	// Names that belong to something else a deployment runs, not to this server.
	elsewhere := map[string]bool{
		"POSTGRES_PASSWORD": true, "POSTGRES_USER": true, "POSTGRES_DB": true,
		"PTIUM_PORT": true, "PTIUM_VERSION": true, "PTIUM_IMAGE": true,
	}
	for file, names := range shipped {
		for _, name := range names {
			if elsewhere[name] {
				continue
			}
			if !strings.Contains(source, `"`+name+`"`) {
				t.Errorf("%s ships %s and the server never reads it", file, name)
			}
		}
	}
}

func serverSource(t *testing.T, dir string) string {
	t.Helper()
	var all strings.Builder
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		all.Write(data)
		return nil
	})
	if err != nil {
		t.Fatalf("read the server: %v", err)
	}
	return all.String()
}
