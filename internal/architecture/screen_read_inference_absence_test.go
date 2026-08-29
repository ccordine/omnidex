package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScreenReadOwnsOnlyDeterministicCaptureAndOCR(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	raw, err := os.ReadFile(filepath.Join(root, "cmd", "cli", "screen_local.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"captureScreenImage()",
		"runScreenOCR(imagePath)",
		"permissionKeyScreenCapture",
		"permissionKeyScreenOCR",
		"retiredScreenReadFlag(args)",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("screen-read omitted deterministic boundary %q", required)
		}
	}
	for _, forbidden := range []string{
		"runOllamaVision",
		"ollamaGenerateRequest",
		"ollamaGenerateResponse",
		"screenVision",
		"VisionSummary",
		"VisionModel",
		"OLLAMA_MODEL_VISION",
		`/api/generate`,
		`"encoding/base64"`,
		`"net/http"`,
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("screen-read retained direct inference surface %q", forbidden)
		}
	}
}

func TestScreenReadHelpAndPermissionsDoNotAdvertiseRetiredInference(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", "..", "cmd", "cli"))
	helpRaw, err := os.ReadFile(filepath.Join(root, "cli_help.go"))
	if err != nil {
		t.Fatal(err)
	}
	var screenHelp string
	for _, line := range strings.Split(string(helpRaw), "\n") {
		if strings.Contains(line, `"  screen-read `) {
			screenHelp = line
			break
		}
	}
	if screenHelp == "" {
		t.Fatal("screen-read help entry is missing")
	}
	for _, forbidden := range []string{"--vision", "--prompt", "--model", "--base-url"} {
		if strings.Contains(screenHelp, forbidden) {
			t.Errorf("screen-read help advertises retired inference flag %q", forbidden)
		}
	}

	permissionRaw, err := os.ReadFile(filepath.Join(root, "permissions_local.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"permissionKeyScreenVision", "local.screen.vision"} {
		if strings.Contains(string(permissionRaw), forbidden) {
			t.Errorf("permissions advertise retired screen inference surface %q", forbidden)
		}
	}
}
