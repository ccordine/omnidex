package omni

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedScriptRootCandidatesFindInstalledParentFromBinary(t *testing.T) {
	tmp := t.TempDir()
	installRoot := filepath.Join(tmp, "install")
	binDir := filepath.Join(installRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}
	updatePath := filepath.Join(installRoot, "update.sh")
	if err := os.WriteFile(updatePath, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatalf("write update script: %v", err)
	}

	roots := managedScriptRootCandidates("", filepath.Join(tmp, "work"), filepath.Join(binDir, "omni"))
	got := locateManagedScript(roots, "update.sh")
	if got != updatePath {
		t.Fatalf("locateManagedScript()=%q, want %q; roots=%v", got, updatePath, roots)
	}
}

func TestOmniUpdateCommandRunsManagedUpdateScript(t *testing.T) {
	tmp := t.TempDir()
	marker := filepath.Join(tmp, "ran.txt")
	script := filepath.Join(tmp, "update.sh")
	body := "#!/usr/bin/env bash\nprintf '%s\\n' \"$@\" > " + shellQuote(marker) + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write update script: %v", err)
	}
	t.Setenv("OMNIDEX_DIR", tmp)

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)
	if err := app.Run([]string{"update", "--host-only", "--no-pull"}); err != nil {
		t.Fatalf("Run(update): %v\nstderr=%s", err, errOut.String())
	}

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	fields := strings.Fields(string(data))
	if len(fields) != 2 || fields[0] != "--host-only" || fields[1] != "--no-pull" {
		t.Fatalf("update args=%q", strings.TrimSpace(string(data)))
	}
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}
