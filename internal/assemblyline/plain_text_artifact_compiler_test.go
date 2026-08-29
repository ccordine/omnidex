package assemblyline

import (
	"strings"
	"testing"
)

func TestPlainTextArtifactCompilerFreezesPathBlindTaskBeforeTargetCoverage(t *testing.T) {
	t.Parallel()
	task, err := FreezePlainTextArtifactTask(
		"Create proof.txt containing one exact line.",
		"Create ARTIFACT_1 containing one exact line.",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(task.Requirement, "proof.txt") {
		t.Fatal("frozen task retained artifact placement")
	}
	target := TargetTree{
		StackID: PlainTextArtifactStackID, VersionProfileID: PlainTextArtifactProfileID,
		Paths: []string{"proof.txt"},
	}
	coverage, err := NewPlainTextArtifactCoverage(task, target)
	if err != nil {
		t.Fatal(err)
	}
	blueprint, err := CompilePlainTextArtifactBlueprint(task, target, coverage)
	if err != nil {
		t.Fatal(err)
	}
	if len(blueprint.Documents) != 1 || blueprint.Documents[0].Path != "proof.txt" ||
		len(blueprint.Documents[0].Blocks) != 1 ||
		blueprint.Documents[0].Blocks[0].Contract != task.Requirement {
		t.Fatalf("blueprint=%+v", blueprint)
	}
}

func TestPlainTextArtifactCompilerRejectsChangedAuthorityAtEveryBoundary(t *testing.T) {
	t.Parallel()
	task, err := FreezePlainTextArtifactTask(
		"Create proof.txt containing one exact line.",
		"Create ARTIFACT_1 containing one exact line.",
	)
	if err != nil {
		t.Fatal(err)
	}
	target := TargetTree{
		StackID: PlainTextArtifactStackID, VersionProfileID: PlainTextArtifactProfileID,
		Paths: []string{"proof.txt"},
	}
	coverage, err := NewPlainTextArtifactCoverage(task, target)
	if err != nil {
		t.Fatal(err)
	}

	changedTask := task
	changedTask.Requirement = "Different"
	if _, err := CompilePlainTextArtifactBlueprint(changedTask, target, coverage); err == nil {
		t.Fatal("changed frozen task was accepted")
	}
	changedTarget := target
	changedTarget.Paths = []string{"proof.go"}
	if _, err := CompilePlainTextArtifactBlueprint(task, changedTarget, coverage); err == nil {
		t.Fatal("changed adapter target was accepted")
	}
	changedCoverage := coverage
	changedCoverage.Path = "other.txt"
	if _, err := CompilePlainTextArtifactBlueprint(task, target, changedCoverage); err == nil {
		t.Fatal("changed coverage was accepted")
	}
}
