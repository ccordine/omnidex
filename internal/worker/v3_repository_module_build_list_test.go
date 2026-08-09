package worker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRepositoryGoBuildListRejectsStdoutOverflowAfterDraining(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/source\n\ngo 1.22\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	goRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(goRoot, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	fakeGo := filepath.Join(goRoot, "bin", "go")
	script := "#!/bin/sh\n" +
		"test \"$(cat \"$HOME/.config/go/telemetry/mode\")\" = 'off 2000-01-01' || exit 23\n" +
		"dd if=/dev/zero bs=1048576 count=5 2>/dev/null\n" +
		"printf complete > .fake-go-complete\n"
	if err := os.WriteFile(fakeGo, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	modules, err := resolveRepositoryGoBuildList(
		context.Background(),
		root,
		repositoryGoSandboxConfig{GoRoot: goRoot, ModuleCache: t.TempDir()},
	)
	if err == nil || modules != nil ||
		!strings.Contains(err.Error(), "Go build-list stdout exceeded its exact 4194304-byte") {
		t.Fatalf("overflow modules=%+v error=%v", modules, err)
	}
	if content, readErr := os.ReadFile(filepath.Join(root, ".fake-go-complete")); readErr != nil || string(content) != "complete" {
		t.Fatalf("fake Go process was not completely drained and waited: content=%q error=%v", content, readErr)
	}
}

func TestResolveRepositoryGoBuildListRejectsStderrOverflowAfterDraining(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/source\n\ngo 1.22\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	goRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(goRoot, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	fakeGo := filepath.Join(goRoot, "bin", "go")
	script := "#!/bin/sh\n" +
		"test \"$(cat \"$HOME/.config/go/telemetry/mode\")\" = 'off 2000-01-01' || exit 23\n" +
		"dd if=/dev/zero bs=65536 count=2 1>&2\n" +
		"printf complete > .fake-go-stderr-complete\n"
	if err := os.WriteFile(fakeGo, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	modules, err := resolveRepositoryGoBuildList(
		context.Background(), root,
		repositoryGoSandboxConfig{GoRoot: goRoot, ModuleCache: t.TempDir()},
	)
	if err == nil || modules != nil ||
		!strings.Contains(err.Error(), "Go build-list stderr exceeded its exact 65536-byte") {
		t.Fatalf("stderr overflow modules=%+v error=%v", modules, err)
	}
	if content, readErr := os.ReadFile(filepath.Join(root, ".fake-go-stderr-complete")); readErr != nil || string(content) != "complete" {
		t.Fatalf("fake Go stderr was not completely drained and waited: content=%q error=%v", content, readErr)
	}
}
