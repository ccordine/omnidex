package queue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMigrationBundleSealsExactBytesAndManifest(t *testing.T) {
	directory := t.TempDir()
	expected := writeTestMigrationManifest(t, directory, map[string]string{
		"001_first.sql":  "CREATE TABLE first_probe (id BIGINT PRIMARY KEY);\n",
		"002_second.sql": "CREATE TABLE second_probe (id BIGINT PRIMARY KEY);\n",
	})
	bundle, err := LoadMigrationBundle(directory, expected)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.ManifestSHA256() != expected || len(bundle.entries) != 2 {
		t.Fatalf("bundle manifest=%q entries=%d", bundle.ManifestSHA256(), len(bundle.entries))
	}
	original := string(bundle.entries[0].body)
	if err := os.WriteFile(
		filepath.Join(directory, bundle.entries[0].name), []byte("SELECT 'changed';\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if string(bundle.entries[0].body) != original || bundle.validate() != nil {
		t.Fatal("loaded migration bundle changed with its source path")
	}
}

func TestLoadMigrationBundleRejectsEveryUnboundDirectoryState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, string, string) (string, string)
	}{
		{name: "relative directory", mutate: func(t *testing.T, dir, digest string) (string, string) {
			return filepath.Base(dir), digest
		}},
		{name: "changed manifest authority", mutate: func(t *testing.T, dir, _ string) (string, string) {
			return dir, strings.Repeat("f", 64)
		}},
		{name: "extra entry", mutate: func(t *testing.T, dir, digest string) (string, string) {
			writeTestMigrationFile(t, dir, "notes.txt", "unregistered\n")
			return dir, digest
		}},
		{name: "empty migration", mutate: func(t *testing.T, dir, _ string) (string, string) {
			writeTestMigrationFile(t, dir, "001_probe.sql", " \n")
			return dir, rewriteTestMigrationManifest(t, dir, []string{"001_probe.sql"})
		}},
		{name: "changed migration", mutate: func(t *testing.T, dir, digest string) (string, string) {
			writeTestMigrationFile(t, dir, "001_probe.sql", "SELECT 2;\n")
			return dir, digest
		}},
		{name: "symlink migration", mutate: func(t *testing.T, dir, _ string) (string, string) {
			target := filepath.Join(t.TempDir(), "target.sql")
			if err := os.WriteFile(target, []byte("SELECT 1;\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(dir, "001_probe.sql")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, "001_probe.sql")); err != nil {
				t.Fatal(err)
			}
			return dir, rewriteTestMigrationManifest(t, dir, []string{"001_probe.sql"})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			digest := writeTestMigrationManifest(t, directory, map[string]string{
				"001_probe.sql": "SELECT 1;\n",
			})
			directory, digest = test.mutate(t, directory, digest)
			if _, err := LoadMigrationBundle(directory, digest); err == nil {
				t.Fatal("invalid migration bundle was accepted")
			}
		})
	}
}

func TestLoadMigrationBundleRejectsDirectoryAndFileSwapDuringLoad(t *testing.T) {
	directory := t.TempDir()
	expected := writeTestMigrationManifest(t, directory, map[string]string{
		"001_probe.sql": "SELECT 1;\n",
	})
	changed := false
	_, err := loadMigrationBundle(directory, expected, func(name string) {
		if name != "001_probe.sql" || changed {
			return
		}
		changed = true
		path := filepath.Join(directory, name)
		if renameErr := os.Rename(path, path+".old"); renameErr != nil {
			t.Fatal(renameErr)
		}
		if writeErr := os.WriteFile(path, []byte("SELECT 2;\n"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
	})
	if err == nil || !changed {
		t.Fatalf("swap during migration load error=%v changed=%t", err, changed)
	}

	parent := t.TempDir()
	realDirectory := filepath.Join(parent, "real")
	if err := os.Mkdir(realDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	digest := writeTestMigrationManifest(t, realDirectory, map[string]string{
		"001_probe.sql": "SELECT 1;\n",
	})
	symlink := filepath.Join(parent, "bundle")
	if err := os.Symlink(realDirectory, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadMigrationBundle(symlink, digest); err == nil {
		t.Fatal("symlink migration directory was accepted")
	}
}

func TestLoadMigrationBundleRejectsMissingNumericPrefix(t *testing.T) {
	directory := t.TempDir()
	digest := writeTestMigrationManifest(t, directory, map[string]string{
		"001_first.sql": "SELECT 1;\n",
		"003_third.sql": "SELECT 3;\n",
	})
	if _, err := LoadMigrationBundle(directory, digest); err == nil ||
		!strings.Contains(err.Error(), "missing numeric migration prefix") {
		t.Fatalf("LoadMigrationBundle error=%v, want missing-prefix rejection", err)
	}
}

func writeTestMigrationManifest(
	t *testing.T,
	directory string,
	files map[string]string,
) string {
	t.Helper()
	names := make([]string, 0, len(files))
	for name, body := range files {
		writeTestMigrationFile(t, directory, name, body)
		names = append(names, name)
	}
	return rewriteTestMigrationManifest(t, directory, names)
}

func rewriteTestMigrationManifest(t *testing.T, directory string, names []string) string {
	t.Helper()
	sortMigrationNames(names)
	var manifest strings.Builder
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&manifest, "%s  %s\n", digestMigrationBytes(raw), name)
	}
	raw := []byte(manifest.String())
	if err := os.WriteFile(filepath.Join(directory, MigrationManifestName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return digestMigrationBytes(raw)
}

func sortMigrationNames(names []string) {
	for index := 1; index < len(names); index++ {
		for cursor := index; cursor > 0 && names[cursor] < names[cursor-1]; cursor-- {
			names[cursor], names[cursor-1] = names[cursor-1], names[cursor]
		}
	}
}

func writeTestMigrationFile(t *testing.T, directory, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
