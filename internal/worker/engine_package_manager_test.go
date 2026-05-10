package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestResolvePackageManagersPrefersHostMetadata(t *testing.T) {
	job := model.Job{
		Metadata: json.RawMessage(`{
			"host_env_package_managers": "dnf,rpm",
			"host_env_package_manager": "dnf"
		}`),
	}

	got := resolvePackageManagers(job)
	if len(got) == 0 {
		t.Fatalf("expected managers from host metadata")
	}
	if got[0] != "dnf" {
		t.Fatalf("expected first manager to be dnf, got %q", got[0])
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "rpm") {
		t.Fatalf("expected rpm in resolved manager list: %v", got)
	}
}

func TestBuildInstallHints(t *testing.T) {
	hints := buildInstallHints([]string{"apt-get", "dnf", "apt-get"}, []string{"go", "npm"})
	if len(hints) < 2 {
		t.Fatalf("expected multiple install hints, got: %v", hints)
	}
	joined := strings.Join(hints, "\n")
	if !strings.Contains(joined, "apt-get install") {
		t.Fatalf("expected apt-get hint in %v", hints)
	}
	if !strings.Contains(joined, "dnf install") {
		t.Fatalf("expected dnf hint in %v", hints)
	}
}
