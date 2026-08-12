package objectiveworkload_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

type aliasProbeOperations struct {
	caller *objectiveworkload.Workload
	shared []byte
}

func (operations *aliasProbeOperations) Materialize(
	_ context.Context,
	item objectiveworkload.WorkItem,
) (objectiveworkload.ArtifactValue, error) {
	operations.caller.Objectives[0].DependsOn[0] = "hostile_caller_mutation"
	operations.caller.Requirements[0].SourceQuote = "hostile caller mutation"
	operations.shared = []byte("accepted\x00" + item.Requirement.SourceQuote)
	return objectiveworkload.ArtifactValue{
		Kind: objectiveworkload.ArtifactRequirementOutput, Content: operations.shared,
	}, nil
}

func (operations *aliasProbeOperations) Verify(
	_ context.Context,
	item objectiveworkload.WorkItem,
	artifact objectiveworkload.Artifact,
) error {
	for index := range operations.shared {
		operations.shared[index] ^= 0xff
	}
	for index := range artifact.Content {
		artifact.Content[index] ^= 0xff
	}
	item.Requirement.SourceQuote = "mutated input copy"
	return nil
}

func TestRunDefendsCallerAndCallbackAliases(t *testing.T) {
	t.Parallel()
	workload := compileOne(t)
	original := append([]objectiveworkload.ObjectiveID{}, workload.Objectives[0].DependsOn...)
	operations := &aliasProbeOperations{caller: &workload}
	result, err := objectiveworkload.Run(
		context.Background(), workload, operations,
		objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.ModelCalls != 0 || len(result.Artifacts) != 1 {
		t.Fatalf("result=%+v", result)
	}
	want := []byte("accepted\x00dashboard")
	if !reflect.DeepEqual(result.Artifacts[0].Content, want) {
		t.Fatalf("authoritative artifact aliased callback bytes: %q", result.Artifacts[0].Content)
	}
	if reflect.DeepEqual(workload.Objectives[0].DependsOn, original) {
		t.Fatal("hostile caller mutation did not execute; test is invalid")
	}
	if result.Objectives[0].DependsOn[0] != original[0] {
		t.Fatalf("run state followed caller alias: %+v", result.Objectives[0])
	}
	result.Artifacts[0].Content[0] ^= 0xff
	if result.Artifacts[0].ContentSHA256 == "" {
		t.Fatal("artifact digest is absent")
	}
}

func TestVerifyMayMutateOnlyItsDefensiveCopy(t *testing.T) {
	t.Parallel()
	workload := compileOne(t)
	operations := &scriptedOperations{mutateVerify: true}
	result, err := objectiveworkload.Run(
		context.Background(), workload, operations,
		objectiveworkload.RunLimits{MaxTransitions: 8, MaxDepth: 8},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || string(result.Artifacts[0].Content) != "accepted\x00dashboard" {
		t.Fatalf("result=%+v", result)
	}
}
