package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDirectCodingDockerProjectEnvironmentIntegration(t *testing.T) {
	if os.Getenv("OMNIDEX_PROJECT_ENVIRONMENT_DOCKER_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_PROJECT_ENVIRONMENT_DOCKER_INTEGRATION=1 to build and run the pinned development image")
	}
	if os.Getenv("DOCKER_HOST") != v3RootfulDockerHost {
		t.Fatalf("opt-in Docker integration requires DOCKER_HOST=%s", v3RootfulDockerHost)
	}
	root := t.TempDir()
	uid, gid := uint32(os.Getuid()), uint32(os.Getgid())
	if uid == 0 || gid == 0 {
		uid, gid = 65532, 65532
		if err := os.Chmod(root, 0o755); err != nil {
			t.Fatalf("make root readable by the non-root container identity: %v", err)
		}
	}
	content := []byte("host-authored project environment visibility proof\n")
	artifactPath := filepath.Join(root, "visible.txt")
	if err := os.WriteFile(artifactPath, content, 0o644); err != nil {
		t.Fatalf("write host fixture: %v", err)
	}
	environment, err := newDirectCodingDockerProjectEnvironment(
		testDirectCodingDockerEnvironmentSpec(),
		directCodingDockerWorkspaceMapping{RuntimeRoot: root, HostRoot: root},
		uid, gid, invokeDirectCodingDocker,
	)
	if err != nil {
		t.Fatalf("new environment: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := environment.Build(ctx); err != nil {
		t.Fatalf("build environment: %v", err)
	}
	defer func() {
		if err := environment.Close(context.Background()); err != nil {
			t.Errorf("close environment: %v", err)
		}
	}()
	execution, err := environment.Run(ctx, directCodingProjectEnvironmentCommand{
		Program: "sha256sum", Args: []string{"visible.txt"}, Timeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("run environment: %v", err)
	}
	digest := sha256.Sum256(content)
	wantOutput := hex.EncodeToString(digest[:]) + "  visible.txt\n"
	if execution.Stdout != wantOutput || execution.Stderr != "" {
		t.Fatalf("container visibility output=%q stderr=%q want=%q", execution.Stdout, execution.Stderr, wantOutput)
	}
	after, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("read host fixture after container verification: %v", err)
	}
	if string(after) != string(content) {
		t.Fatalf("read-only project environment changed host fixture: %q", after)
	}
}
