package omni

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOmniNoOllamaOneShotReturnsErrorInsteadOfPanic(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader("hello"), &out, &errOut)

	err := app.Run([]string{"--no-ollama", "--no-permission-prompt"})
	if err == nil {
		t.Fatal("expected no-ollama one-shot to return an error")
	}
	if !strings.Contains(err.Error(), "llm client is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOmniOllamaPrewarmCommandReportsProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"model":"fake","done":true,"message":{"role":"assistant","content":"ok"},"total_duration":10,"load_duration":3,"prompt_eval_count":2,"eval_count":1}`))
	}))
	defer server.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(""), &out, &errOut)

	if err := app.Run([]string{"ollama", "prewarm", "--endpoint", server.URL, "--model", "fake"}); err != nil {
		t.Fatal(err)
	}
	output := out.String()
	for _, want := range []string{"model=fake", "total_duration=10", "load_duration=3"} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q: %s", want, output)
		}
	}
}

func TestOmniPatchApplyDryRunCommand(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "hello.txt"), []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := `diff --git a/hello.txt b/hello.txt
--- a/hello.txt
+++ b/hello.txt
@@ -1,2 +1,2 @@
 one
-two
+TWO
`
	var out bytes.Buffer
	var errOut bytes.Buffer
	app := NewApp(strings.NewReader(patch), &out, &errOut)

	if err := app.Run([]string{"patch", "apply", "--workspace", workspace, "--dry-run"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Patch dry-run passed") {
		t.Fatalf("unexpected output: %s", out.String())
	}
	data, err := os.ReadFile(filepath.Join(workspace, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "one\ntwo\n" {
		t.Fatalf("dry run wrote file: %q", string(data))
	}
}
