package objectiveworkload_test

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

type constantArtifactOperations struct{}

func (constantArtifactOperations) Materialize(
	context.Context,
	objectiveworkload.WorkItem,
) (objectiveworkload.ArtifactValue, error) {
	return objectiveworkload.ArtifactValue{
		Kind: objectiveworkload.ArtifactRequirementOutput, Content: []byte("same opaque output"),
	}, nil
}

func (constantArtifactOperations) Verify(
	context.Context,
	objectiveworkload.WorkItem,
	objectiveworkload.Artifact,
) error {
	return nil
}

func TestSameAuthorityDistinctValidPartitionsHaveDistinctCompiledAuthority(t *testing.T) {
	t.Parallel()
	authority := "Build alerts and exports."
	combinedStation := &scriptedPartitionStation{
		steps: newPartitionScript(authority, "alerts and exports"),
	}
	combined, err := objectiveworkload.Compile(
		context.Background(), authority, combinedStation,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if err != nil {
		t.Fatal(err)
	}
	splitStation := &scriptedPartitionStation{
		steps: newPartitionScript(authority, "alerts", "exports"),
	}
	split, err := objectiveworkload.Compile(
		context.Background(), authority, splitStation,
		objectiveworkload.CompileLimits{MaxStationCalls: 8},
	)
	if err != nil {
		t.Fatal(err)
	}
	if combined.CompilationID != split.CompilationID {
		t.Fatalf("same exact source has different compilation-attempt IDs: %q %q", combined.CompilationID, split.CompilationID)
	}
	if combined.Workload.ID == split.Workload.ID {
		t.Fatalf("distinct accepted partitions share compiled workload ID %q", combined.Workload.ID)
	}
	for _, result := range []objectiveworkload.CompileResult{combined, split} {
		for _, gap := range result.Gaps {
			if gap.CompilationID != result.CompilationID || gap.FinalWorkloadID != result.Workload.ID {
				t.Fatalf("gap/result authority mismatch: gap=%+v result=%+v", gap, result)
			}
		}
	}
	combinedRun, err := objectiveworkload.Run(
		context.Background(), combined.Workload, constantArtifactOperations{},
		objectiveworkload.RunLimits{MaxTransitions: 16, MaxDepth: 8},
	)
	if err != nil {
		t.Fatal(err)
	}
	splitRun, err := objectiveworkload.Run(
		context.Background(), split.Workload, constantArtifactOperations{},
		objectiveworkload.RunLimits{MaxTransitions: 16, MaxDepth: 8},
	)
	if err != nil {
		t.Fatal(err)
	}
	if combinedRun.Artifacts[0].ContentSHA256 != splitRun.Artifacts[0].ContentSHA256 ||
		combinedRun.Artifacts[0].ID == splitRun.Artifacts[0].ID {
		t.Fatalf("artifact identity is not partition-bound: combined=%+v split=%+v", combinedRun.Artifacts[0], splitRun.Artifacts[0])
	}
}
