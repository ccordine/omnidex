package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestRepositoryGoVerificationBubblewrapIntegration(t *testing.T) {
	if os.Getenv("OMNIDEX_REQUIRE_BWRAP_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_REQUIRE_BWRAP_INTEGRATION=1 to require the real bubblewrap production proof")
	}
	if runtime.GOOS != "linux" {
		t.Fatalf("real repository verification sandbox requires Linux, received %s", runtime.GOOS)
	}
	t.Setenv("OMNIDEX_SANDBOX_HOST_SECRET", "must-not-enter-sandbox")
	hostModuleCache := t.TempDir()
	t.Setenv("GOMODCACHE", hostModuleCache)
	moduleCanaryRelative := filepath.Join(
		"unrelated.example", "private@v1.0.0", "verification-secret.pem",
	)
	moduleCanary := filepath.Join(hostModuleCache, moduleCanaryRelative)
	if err := os.MkdirAll(filepath.Dir(moduleCanary), 0o700); err != nil {
		t.Fatal(err)
	}
	const moduleCanaryContent = "OMNIDEX-UNRELATED-MODULE-CACHE-CANARY"
	if err := os.WriteFile(moduleCanary, []byte(moduleCanaryContent), 0o600); err != nil {
		t.Fatal(err)
	}
	hostListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("open host-only network authority: %v", err)
	}
	defer hostListener.Close()
	externalRoot := t.TempDir()
	external := filepath.Join(externalRoot, "host-sentinel")
	if err := os.WriteFile(external, []byte("host authority\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module example.com/isolation\n\ngo 1.22\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "host-escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(root, ".gitignore"),
		[]byte("host-escape\nlarge/private.env\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "large", "clean"), 0o700); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 3_100; index++ {
		path := filepath.Join(root, "large", "clean", fmt.Sprintf("file-%04d.txt", index))
		if err := os.WriteFile(path, []byte("large exact projection\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(
		filepath.Join(root, "large", "private.env"), []byte("LARGE_SECRET=forbidden\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("PROJECTION_SECRET=forbidden\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".omni"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".omni", "secret"), []byte("forbidden\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testSource := `package isolation

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSandboxAuthority(t *testing.T) {
	if cgoProbe() != 7 {
		t.Fatal("cgo probe returned the wrong value")
	}
	if os.Getenv("OMNIDEX_SANDBOX_HOST_SECRET") != "" {
		t.Fatal("host environment entered verification")
	}
	if home := os.Getenv("HOME"); home != "/home/omnidex" {
		t.Fatalf("HOME=%q", home)
	}
	if cache := os.Getenv("GOCACHE"); cache != "/tmp/gocache" {
		t.Fatalf("GOCACHE=%q", cache)
	}
	if temporary := os.Getenv("TMPDIR"); temporary != "/tmp" {
		t.Fatalf("TMPDIR=%q", temporary)
	}
	if work := os.Getenv("GOWORK"); work != "off" {
		t.Fatalf("GOWORK=%q", work)
	}
	if flags := os.Getenv("GOFLAGS"); flags != "-mod=readonly" {
		t.Fatalf("GOFLAGS=%q", flags)
	}
	if content, err := os.ReadFile("__MODULE_CANARY__"); err == nil {
		t.Fatalf("unrelated host module cache entered verification: %s", content)
	}
	if _, err := os.ReadFile("host-escape"); err == nil {
		t.Fatal("repository symlink escaped the sandbox")
	}
	if content, err := os.ReadFile("large/clean/file-3099.txt"); err != nil || string(content) != "large exact projection\n" {
		t.Fatalf("large compacted projection content=%q error=%v", content, err)
	}
	for _, excluded := range []string{".git", ".omni", ".env", "large/private.env", "/projection-base/go.mod"} {
		if _, err := os.Stat(excluded); err == nil {
			t.Fatalf("excluded projection authority %q entered verification", excluded)
		}
	}
	if connection, err := net.DialTimeout("tcp4", "__HOST_LISTENER__", 200*time.Millisecond); err == nil {
		_ = connection.Close()
		t.Fatal("verification sandbox reached a host network listener")
	}
	probe := "sandbox-write-probe"
	if err := os.WriteFile(probe, []byte("forbidden\n"), 0o600); err == nil {
		t.Fatal("repository source tree is writable")
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("GOMODCACHE"), "host-write"), []byte("x"), 0o600); err == nil {
		t.Fatal("existing module cache is writable")
	}
	if err := os.WriteFile(filepath.Join(runtime.GOROOT(), "host-write"), []byte("x"), 0o600); err == nil {
		t.Fatal("system Go toolchain is writable")
	}
	if path := os.Getenv("PATH"); !strings.HasPrefix(path, "/toolchain/bin:") {
		t.Fatalf("PATH=%q", path)
	}
}

`
	testSource = strings.ReplaceAll(testSource, "__HOST_LISTENER__", hostListener.Addr().String())
	testSource = strings.ReplaceAll(
		testSource,
		"__MODULE_CANARY__",
		filepath.ToSlash(filepath.Join("/gomodcache", moduleCanaryRelative)),
	)
	if err := os.WriteFile(filepath.Join(root, "isolation_test.go"), []byte(testSource), 0o600); err != nil {
		t.Fatal(err)
	}
	cgoSource := `package isolation

/*
static int omnidex_cgo_probe(void) { return 7; }
*/
import "C"

func cgoProbe() int { return int(C.omnidex_cgo_probe()) }
`
	if err := os.WriteFile(filepath.Join(root, "cgo_probe.go"), []byte(cgoSource), 0o600); err != nil {
		t.Fatal(err)
	}
	repositorySandboxGit(t, root, "init")
	repositorySandboxGit(t, root, "config", "user.name", "Omnidex Test")
	repositorySandboxGit(t, root, "config", "user.email", "omnidex@example.invalid")
	repositorySandboxGit(t, root, "add", "go.mod", ".gitignore", "isolation_test.go", "cgo_probe.go", "large/clean")
	repositorySandboxGit(t, root, "commit", "-m", "sandbox fixture")
	snapshot, err := repositoryfacts.BuildGitSnapshot(
		context.Background(), root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := newRepositorySnapshotProjection(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeRepositoryGoVerification(
		context.Background(), projection, repositoryGoTestCall(30),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !directCodingCommandSucceeded(result) || len(result.Evidence) != 1 {
		t.Fatalf("real sandbox result=%#v evidence=%#v", result.Output, result.Evidence)
	}
	if result.Evidence[0].Command != "go test -json -count=1 ./..." ||
		strings.Contains(result.Evidence[0].Command, "bwrap") {
		t.Fatalf("real sandbox evidence command=%q", result.Evidence[0].Command)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		moduleCanaryContent, filepath.Base(moduleCanaryRelative), filepath.ToSlash(moduleCanaryRelative),
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("unrelated module-cache canary entered command evidence: %q", forbidden)
		}
	}
}
