package objectiveworkload

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestArtifactCannotReplayAcrossAcceptedPartitionOrRequirement(t *testing.T) {
	t.Parallel()
	authority, err := newAuthority("Build alerts and exports.")
	if err != nil {
		t.Fatal(err)
	}
	combined, err := buildWorkload(authority, assemblyline.RequirementPartitionDecision{
		Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: []string{"alerts and exports"},
	})
	if err != nil {
		t.Fatal(err)
	}
	split, err := buildWorkload(authority, assemblyline.RequirementPartitionDecision{
		Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: []string{"alerts", "exports"},
	})
	if err != nil {
		t.Fatal(err)
	}
	item := WorkItem{
		WorkloadID: combined.ID, AuthoritySHA256: authority.SHA256,
		Requirement: combined.Requirements[0],
	}
	artifact, err := newArtifact(combined, item, ArtifactValue{
		Kind: ArtifactRequirementOutput, Content: []byte("same opaque output"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := validateArtifact(split, split.Requirements[0], artifact); !errors.Is(err, ErrArtifact) {
		t.Fatalf("cross-partition replay error=%v", err)
	}
	artifact.WorkloadID = split.ID
	artifact.RequirementID = split.Requirements[1].ID
	if err := validateArtifact(split, split.Requirements[1], artifact); !errors.Is(err, ErrArtifact) {
		t.Fatalf("cross-requirement replay error=%v", err)
	}
}
