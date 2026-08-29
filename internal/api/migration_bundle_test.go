package api

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/version"
)

func loadAPITestMigrationBundle(t testing.TB) queue.MigrationBundle {
	t.Helper()
	directory, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := queue.LoadMigrationBundle(directory, version.MigrationsSHA256)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func loadAPITestMigrationBundleThroughPrefix(t testing.TB, maximum string) queue.MigrationBundle {
	t.Helper()
	if matched, _ := regexp.MatchString(`^[0-9]{3}$`, maximum); !matched {
		t.Fatalf("migration prefix %q is not canonical", maximum)
	}
	source, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0)
	foundMaximum := false
	migrationName := regexp.MustCompile(`^[0-9]{3}_[A-Za-z0-9_]+\.sql$`)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !migrationName.MatchString(name) || name[:3] > maximum {
			continue
		}
		if name[:3] == maximum {
			foundMaximum = true
		}
		raw, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if !foundMaximum {
		t.Fatalf("migration prefix %s is unavailable", maximum)
	}
	sort.Strings(names)
	var manifest strings.Builder
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(target, name))
		if err != nil {
			t.Fatal(err)
		}
		manifest.WriteString(fmt.Sprintf("%x  %s\n", sha256.Sum256(raw), name))
	}
	manifestBytes := []byte(manifest.String())
	if err := os.WriteFile(filepath.Join(target, queue.MigrationManifestName), manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	manifestDigest := fmt.Sprintf("%x", sha256.Sum256(manifestBytes))
	bundle, err := queue.LoadMigrationBundle(target, manifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}
