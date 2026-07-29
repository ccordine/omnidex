package worker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDirectCodingRuntimeContainsNoProductSpecificRecipes(t *testing.T) {
	t.Parallel()

	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve worker source directory")
	}
	root := filepath.Dir(current)
	for _, forbiddenFile := range []string{
		"v3_coding_web_audio.go",
		"v3_coding_web_app.go",
		"v3_coding_web_adapter.go",
		"v3_coding_go_record_adapter.go",
		"v3_coding_go_expense_adapter.go",
		"v3_coding_go_converter_adapter.go",
	} {
		if _, err := os.Stat(filepath.Join(root, forbiddenFile)); err == nil {
			t.Fatalf("product-specific runtime recipe still exists: %s", forbiddenFile)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}

	files, err := filepath.Glob(filepath.Join(root, "v3_coding*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"react_audio_workstation_v1",
			"go_record_cli_v1",
			"go_expense_cli_v1",
			"go_converter_cli_v1",
			"typeScriptStudio",
			"typeScriptAudio",
		} {
			if strings.Contains(string(source), forbidden) {
				t.Fatalf("product-specific runtime authority %q remains in %s", forbidden, filepath.Base(path))
			}
		}
	}
}
