package omni

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestRuntimeVolumeInitializerAssignsExactNonRootOwnership(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("runtime volume ownership uses Linux container filesystem semantics")
	}
	uid, gid := os.Getuid(), os.Getgid()
	if uid == 0 || gid == 0 {
		t.Skip("success path requires the non-root identity used by managed Compose")
	}
	root := repoRootFromOmniTest(t)
	first := filepath.Join(t.TempDir(), "deployment")
	second := filepath.Join(t.TempDir(), "gomod")
	for _, directory := range []string{first, second} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, filepath.Join(directory, "retained"), "data\n", 0o600)
	}
	arguments := []string{fmt.Sprint(uid), fmt.Sprint(gid), first, second}
	script := filepath.Join(root, "scripts", "initialize-runtime-volumes.sh")
	if output, err := exec.Command(script, arguments...).CombinedOutput(); err != nil {
		t.Fatalf("initialize runtime volumes: %v: %s", err, output)
	}
	for _, directory := range []string{first, second} {
		assertRuntimeVolumeOwnership(t, directory, uid, gid)
		marker := filepath.Join(directory, ".omnidex-owner")
		if got, err := os.ReadFile(marker); err != nil || string(got) != fmt.Sprintf("%d:%d\n", uid, gid) {
			t.Fatalf("owner marker=%q error=%v", got, err)
		}
	}
	if output, err := exec.Command(script, arguments...).CombinedOutput(); err != nil {
		t.Fatalf("repeat runtime volume initialization: %v: %s", err, output)
	}
}

func TestRuntimeVolumeInitializerRejectsRootOrSymlinkAuthority(t *testing.T) {
	root := repoRootFromOmniTest(t)
	script := filepath.Join(root, "scripts", "initialize-runtime-volumes.sh")
	directory := t.TempDir()
	for _, arguments := range [][]string{
		{"0", "1000", directory},
		{"1000", "0", directory},
		{"01000", "1000", directory},
		{"4294967295", "1000", directory},
		{" 1000", "1000", directory},
	} {
		output, err := exec.Command(script, arguments...).CombinedOutput()
		if err == nil || !strings.Contains(string(output), "[volume-init][error]") {
			t.Fatalf("invalid identity %q error=%v output=%q", arguments, err, output)
		}
	}
	target := t.TempDir()
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(script, "1000", "1000", link).CombinedOutput()
	if err == nil || !strings.Contains(string(output), "must be a real directory") {
		t.Fatalf("symlink volume error=%v output=%q", err, output)
	}
}

func assertRuntimeVolumeOwnership(t *testing.T, path string, uid, gid int) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != uid || int(stat.Gid) != gid {
		t.Fatalf("%s ownership=%v want %d:%d", path, info.Sys(), uid, gid)
	}
}
