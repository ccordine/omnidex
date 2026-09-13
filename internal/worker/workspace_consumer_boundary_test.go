package worker

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestWorkerConstructionDefersUnusedHostDirectoryValidation(t *testing.T) {
	for _, configured := range []string{"", "relative", filepath.Join(t.TempDir(), "missing")} {
		service, err := New(&queue.Repository{}, nil, nil, Options{HostDirectoryAccessRoot: configured})
		if err != nil {
			t.Fatalf("unused host directory %q blocked worker construction: %v", configured, err)
		}
		if err := service.requireHostWorkspaceRoot(t.TempDir()); err == nil {
			t.Fatalf("filesystem consumer accepted invalid host directory %q", configured)
		}
	}
}

func TestNativeActionDispatchDoesNotAcquireUnusedWorkspace(t *testing.T) {
	service := &Service{}
	claim := &model.ClaimedStep{}
	if err := service.runNativeV3Step(context.Background(), claim, "unknown_action"); err == nil || !strings.Contains(err.Error(), `worker action "unknown_action" is not registered`) {
		t.Fatalf("unused workspace obscured action dispatch: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.runNativeV3Step(ctx, claim, "objective_resolve"); !errors.Is(err, context.Canceled) {
		t.Fatalf("unused workspace blocked a canceled semantic turn: %v", err)
	}
}

func TestRuntimeInstructionProvenanceNeedsOnlyDeclaredPathContext(t *testing.T) {
	for _, instruction := range []string{"explain main.go", "describe settings.json"} {
		root := filepath.Join(t.TempDir(), "absent-workspace")
		metadata, err := json.Marshal(map[string]string{"client_cwd": root})
		if err != nil {
			t.Fatal(err)
		}
		runtime := &nativeRuntimeV3{svc: &Service{}, ctx: context.Background(),
			claim: &model.ClaimedStep{Job: model.Job{Metadata: metadata, Instruction: instruction}}}
		provenance, err := runtime.deriveObjectiveInstructionProvenance()
		if err != nil {
			t.Fatalf("lexical path processing acquired the filesystem: %v", err)
		}
		redacted, identities, err := assemblyline.RedactArtifactIdentities(instruction, provenance)
		if err != nil {
			t.Fatal(err)
		}
		if len(identities) != 1 || !strings.HasSuffix(redacted, "ARTIFACT_1") {
			t.Fatalf("declared path context was lost: %q %#v", redacted, identities)
		}
	}
}
