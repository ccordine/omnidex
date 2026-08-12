package objectiveworkload_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/cognitionreference/objectiveworkload"
)

type partitionStep struct {
	input    assemblyline.RequirementPartitionInput
	decision assemblyline.RequirementPartitionDecision
}

type scriptedPartitionStation struct {
	mu      sync.Mutex
	steps   []partitionStep
	calls   int
	prompts []string
	errAt   int
	err     error
}

func newPartitionScript(authority string, quotes ...string) []partitionStep {
	residual, err := assemblyline.BuildRequirementResidual(authority, quotes)
	if err != nil {
		panic(err)
	}
	steps := []partitionStep{
		{
			input: assemblyline.RequirementPartitionInput{
				SourceText: authority,
				Mode:       assemblyline.RequirementExtractFeatures,
			},
			decision: assemblyline.RequirementPartitionDecision{
				Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: append([]string{}, quotes...),
			},
		},
		{
			input: assemblyline.RequirementPartitionInput{
				SourceText: residual,
				Mode:       assemblyline.RequirementExtractFeatures,
			},
			decision: assemblyline.RequirementPartitionDecision{
				Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: []string{},
			},
		},
	}
	for _, quote := range quotes {
		steps = append(steps, partitionStep{
			input: assemblyline.RequirementPartitionInput{
				SourceText: quote,
				Mode:       assemblyline.RequirementSplitFeature,
			},
			decision: assemblyline.RequirementPartitionDecision{
				Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: []string{quote},
			},
		})
	}
	return steps
}

func (station *scriptedPartitionStation) Generate(
	ctx context.Context,
	job assemblyline.PortableJob,
) (assemblyline.PortableResult, error) {
	station.mu.Lock()
	defer station.mu.Unlock()
	station.calls++
	call := station.calls
	if err := ctx.Err(); err != nil {
		return assemblyline.PortableResult{}, err
	}
	if station.errAt == call {
		return assemblyline.PortableResult{}, station.err
	}
	if job.Kind != assemblyline.WorkRequirementPartition {
		return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
	}
	if err := job.Validate(); err != nil {
		return assemblyline.PortableResult{}, err
	}
	if call > len(station.steps) {
		return assemblyline.PortableResult{}, fmt.Errorf("unexpected partition call %d", call)
	}
	var input assemblyline.RequirementPartitionInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		return assemblyline.PortableResult{}, err
	}
	wanted := station.steps[call-1]
	if !reflect.DeepEqual(input, wanted.input) {
		return assemblyline.PortableResult{}, fmt.Errorf("call %d input=%+v want %+v", call, input, wanted.input)
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	station.prompts = append(station.prompts, prompt)
	raw, err := json.Marshal(wanted.decision)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, nil
}

type scriptedOperations struct {
	mu             sync.Mutex
	calls          []string
	materializeErr error
	verifyErr      error
	mutateVerify   bool
}

func (operations *scriptedOperations) Materialize(
	ctx context.Context,
	item objectiveworkload.WorkItem,
) (objectiveworkload.ArtifactValue, error) {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return objectiveworkload.ArtifactValue{}, err
	}
	operations.calls = append(operations.calls, "materialize:"+string(item.Requirement.ID))
	if operations.materializeErr != nil {
		return objectiveworkload.ArtifactValue{}, operations.materializeErr
	}
	return objectiveworkload.ArtifactValue{
		Kind:    objectiveworkload.ArtifactRequirementOutput,
		Content: []byte("accepted\x00" + item.Requirement.SourceQuote),
	}, nil
}

func (operations *scriptedOperations) Verify(
	ctx context.Context,
	item objectiveworkload.WorkItem,
	artifact objectiveworkload.Artifact,
) error {
	operations.mu.Lock()
	defer operations.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	operations.calls = append(operations.calls, "verify:"+string(item.Requirement.ID))
	if operations.mutateVerify && len(artifact.Content) > 0 {
		artifact.Content[0] ^= 0xff
		return nil
	}
	if operations.verifyErr != nil {
		return operations.verifyErr
	}
	want := []byte("accepted\x00" + item.Requirement.SourceQuote)
	if !reflect.DeepEqual(artifact.Content, want) {
		return fmt.Errorf("artifact content=%q want %q", artifact.Content, want)
	}
	return nil
}
