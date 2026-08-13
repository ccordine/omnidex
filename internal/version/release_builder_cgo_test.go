package version

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseBuilderRequiresNativeCGOCompilation(t *testing.T) {
	script := filepath.Join(releaseRepositoryRoot(t), "scripts", "build-release.sh")
	source, err := exec.Command("sed", "-n", "1,340p", script).CombinedOutput()
	if err != nil {
		t.Fatalf("read release builder: %v: %s", err, source)
	}
	if !strings.Contains(string(source), "CGO_ENABLED=1") {
		t.Fatal("release builder does not enable required CGO compilation")
	}
	if strings.Contains(string(source), "CGO_ENABLED=0") {
		t.Fatal("release builder retains the forbidden CGO-disabled build path")
	}
	if !strings.Contains(string(source), `TARGETS=("$(go env GOOS)/$(go env GOARCH)")`) ||
		!strings.Contains(string(source), "Defaults to the native Go host target") {
		t.Fatal("release builder does not default to its supported native CGO target")
	}

	host := runtime.GOOS + "/" + runtime.GOARCH
	command := exec.Command("bash", "-c",
		`source "$1"; validate_release_cgo_targets "$2"`,
		"release-native-cgo", script, host)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("native CGO preflight failed: %v: %s", err, output)
	}
}

func TestReleaseBuilderRejectsCGOCrossCompilationBeforeBuild(t *testing.T) {
	script := filepath.Join(releaseRepositoryRoot(t), "scripts", "build-release.sh")
	cross := "linux/arm64"
	if runtime.GOOS == "linux" && runtime.GOARCH == "arm64" {
		cross = "linux/amd64"
	}
	command := exec.Command("bash", "-c",
		`source "$1"; validate_release_cgo_targets "$2"`,
		"release-cross-cgo", script, cross)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "CGO release cross-compilation is unsupported") {
		t.Fatalf("cross CGO error=%v output=%q", err, output)
	}
}
