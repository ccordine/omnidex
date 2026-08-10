package cognitiongauntlet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseMigrationBundleDerivesFromExecutableAndVerifiesExactManifest(t *testing.T) {
	executable, manifestSHA := writeTestReleaseBundle(t)
	directory, err := releaseMigrationBundle(executable, manifestSHA)
	if err != nil {
		t.Fatal(err)
	}
	if directory != filepath.Join(filepath.Dir(filepath.Dir(executable)), "migrations") {
		t.Fatalf("migration directory=%q", directory)
	}
}

func TestReleaseMigrationBundleRejectsEveryTamperAndUnregisteredPath(t *testing.T) {
	for name, mutate := range map[string]func(*testing.T, string){
		"migration content": func(t *testing.T, executable string) {
			writeTestFile(t, filepath.Join(filepath.Dir(filepath.Dir(executable)), "migrations", "001_first.sql"), "changed\n")
		},
		"manifest": func(t *testing.T, executable string) {
			writeTestFile(t, filepath.Join(filepath.Dir(filepath.Dir(executable)), "migrations", offlineMigrationManifestName), "invalid\n")
		},
		"extra file": func(t *testing.T, executable string) {
			writeTestFile(t, filepath.Join(filepath.Dir(filepath.Dir(executable)), "migrations", "notes.txt"), "not registered\n")
		},
	} {
		t.Run(name, func(t *testing.T) {
			executable, manifestSHA := writeTestReleaseBundle(t)
			mutate(t, executable)
			if _, err := releaseMigrationBundle(executable, manifestSHA); err == nil {
				t.Fatal("tampered release migration bundle was accepted")
			}
		})
	}
	executable := filepath.Join(t.TempDir(), "cognition-gauntlet")
	writeTestFile(t, executable, "binary")
	if _, err := releaseMigrationBundle(executable, strings.Repeat("a", 64)); err == nil {
		t.Fatal("executable outside a release bin directory was accepted")
	}
}

func writeTestReleaseBundle(t *testing.T) (string, string) {
	t.Helper()
	bundle := t.TempDir()
	bin := filepath.Join(bundle, "bin")
	migrations := filepath.Join(bundle, "migrations")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(migrations, 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(bin, "cognition-gauntlet")
	writeTestFile(t, executable, "binary")
	files := []struct{ name, content string }{
		{"001_first.sql", "SELECT 1;\n"}, {"002_second.sql", "SELECT 2;\n"},
	}
	var manifest strings.Builder
	for _, file := range files {
		writeTestFile(t, filepath.Join(migrations, file.name), file.content)
		fmt.Fprintf(&manifest, "%s  %s\n", digestBytes([]byte(file.content)), file.name)
	}
	raw := []byte(manifest.String())
	writeTestFile(t, filepath.Join(migrations, offlineMigrationManifestName), string(raw))
	return executable, digestBytes(raw)
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
