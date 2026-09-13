package worker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func rejectNativeCompiledLanguageTools(t *testing.T) {
	t.Helper()
	directory := t.TempDir()
	for _, name := range []string{"node", "npm", "go", "gofmt", "rustc", "cargo", "javac", "java", "jar"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("#!/bin/sh\nprintf 'unexpected host toolchain execution' >&2\nexit 97\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func assertCompiledLanguageContainersRemoved(t *testing.T, records []queue.VerificationCommandEvidence) {
	t.Helper()
	seen := make(map[string]bool)
	for _, record := range records {
		if record.ContainerID == "" || seen[record.ContainerID] {
			continue
		}
		seen[record.ContainerID] = true
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		output, err := exec.CommandContext(ctx, "docker", "container", "ls", "--all", "--quiet", "--no-trunc", "--filter", "id="+record.ContainerID).CombinedOutput()
		cancel()
		if err != nil || strings.TrimSpace(string(output)) != "" {
			t.Fatalf("compiled experiment remains: %s %v", output, err)
		}
	}
	if len(seen) == 0 {
		t.Fatal("compiled verification recorded no Docker container")
	}
}
