package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/manifest"
)

func TestCLICoreURLUsesHomeManifest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CORE_URL", "http://old-loopback:18090")
	path, _ := manifest.HomePath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	value := manifest.Defaults()
	value.CoreURL = "https://core.omni.worknet"
	raw, _ := json.Marshal(value)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := clientCoreURL(); err != nil || got != value.CoreURL {
		t.Fatalf("manifest URL not consumed: %q %v", got, err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := clientCoreURL(); err == nil {
		t.Fatal("invalid manifest silently used an environment URL")
	}
}
