package worker

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationRequirementQueueRejectsUnboundCandidateBeforeSemanticWork(t *testing.T) {
	t.Parallel()
	authority, entry := directCodingRequirementQueueEntry(
		t,
		"Build a browser status display.",
		"Display the current status.",
		nil,
	)
	entry.Inventory.RawSHA256 = strings.Repeat("0", 64)
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			return assemblyline.PortableResult{JobID: job.ID}, nil
		},
	}
	_, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime,
		"intent-model",
		authority,
		entry,
		nil,
		nil,
		nil,
	)
	if err == nil || calls != 0 {
		t.Fatalf("unbound queue entry error=%v semantic calls=%d", err, calls)
	}
}

func TestApplicationRequirementQueueRejectsPartitionAncestryCycle(t *testing.T) {
	t.Parallel()
	authority, root := directCodingRequirementQueueEntry(
		t,
		"Build a browser utility with two status outputs.",
		"Show the primary and secondary status.",
		nil,
	)
	firstInput, firstPartition := applicationRequirementQueuePartitionFixture(
		t,
		"Show the primary and secondary status.",
		"Show the primary status.\nShow the secondary status.",
	)
	children, err := root.partitionChildren(authority, firstInput, firstPartition)
	if err != nil {
		t.Fatal(err)
	}
	cycleInput, cyclePartition := applicationRequirementQueuePartitionFixture(
		t,
		"Show the primary status.",
		"Show the primary and secondary status.\nShow the primary status indicator.",
	)
	if _, err := children[0].partitionChildren(
		authority,
		cycleInput,
		cyclePartition,
	); err == nil || !strings.Contains(err.Error(), "ancestry cycle") {
		t.Fatalf("cycle error=%v", err)
	}
}

func TestApplicationRequirementQueuePartitionDepthIsExactAndBounded(t *testing.T) {
	t.Parallel()
	authority, entry := directCodingRequirementQueueEntry(
		t,
		"Build a browser utility with nested status output wording.",
		"status level zero compound",
		nil,
	)
	for depth := 0; depth < assemblyline.MaxApplicationRequirementCandidatePartitionDepth; depth++ {
		current, _, err := entry.validateFor(authority)
		if err != nil {
			t.Fatal(err)
		}
		next := "status level " + string(rune('a'+depth))
		partitionInput, partition := applicationRequirementQueuePartitionFixture(
			t,
			current,
			next+"\nstatus sibling "+string(rune('a'+depth)),
		)
		children, err := entry.partitionChildren(authority, partitionInput, partition)
		if err != nil {
			t.Fatal(err)
		}
		entry = children[0]
	}
	current, _, err := entry.validateFor(authority)
	if err != nil {
		t.Fatal(err)
	}
	partitionInput, partition := applicationRequirementQueuePartitionFixture(
		t,
		current,
		"one narrower child\nanother narrower child",
	)
	if _, err := entry.partitionChildren(authority, partitionInput, partition); err == nil ||
		!strings.Contains(err.Error(), "partition depth") {
		t.Fatalf("depth error=%v", err)
	}
}

func TestApplicationRequirementQueueSkipsPartitionCallAtDepthBoundary(t *testing.T) {
	t.Parallel()
	authority, entry := directCodingRequirementQueueEntry(
		t,
		"Build a browser utility with nested status output wording.",
		"status level zero compound",
		nil,
	)
	for depth := 0; depth < assemblyline.MaxApplicationRequirementCandidatePartitionDepth; depth++ {
		current, _, err := entry.validateFor(authority)
		if err != nil {
			t.Fatal(err)
		}
		next := "status nested level " + string(rune('a'+depth))
		partitionInput, partition := applicationRequirementQueuePartitionFixture(
			t,
			current,
			next+"\nstatus sibling level "+string(rune('a'+depth)),
		)
		children, err := entry.partitionChildren(authority, partitionInput, partition)
		if err != nil {
			t.Fatal(err)
		}
		entry = children[0]
	}

	var calls []assemblyline.WorkKind
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			var candidate string
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				candidate = assemblyline.ApplicationRequirementCandidateEntailed
			case assemblyline.WorkApplicationRequirementCandidateKind:
				var err error
				candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(
					job,
					assemblyline.ApplicationRequirementCandidateMixed,
				)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
			case assemblyline.WorkApplicationRequirementCandidatePartition:
				return assemblyline.PortableResult{}, fmt.Errorf("partition call reached exhausted depth")
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	resolved, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime, "intent-model", authority, entry, nil, nil, nil,
	)
	if err != nil || resolved.Disposition != directCodingApplicationRequirementUnresolved {
		t.Fatalf("resolution=%+v calls=%v error=%v", resolved, calls, err)
	}
	want := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateAuthorization,
		assemblyline.WorkApplicationRequirementCandidateKind,
		assemblyline.WorkApplicationRequirementCandidateKind,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("semantic calls=%v want=%v", calls, want)
	}
}

func TestApplicationRequirementQueueDiscardsCandidateWhenDirectRuntimeIsAbsent(
	t *testing.T,
) {
	t.Parallel()
	authority, entry := directCodingRequirementQueueEntry(
		t,
		"Build a browser status display with a current-status indicator.",
		"Candidate wording with no classified content.",
		nil,
	)
	var calls []assemblyline.WorkKind
	var dimensions []assemblyline.ApplicationRequirementCandidateContentDimension
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			switch job.Kind {
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				return assemblyline.PortableResult{
					JobID:     job.ID,
					Candidate: assemblyline.ApplicationRequirementCandidateEntailed,
				}, nil
			case assemblyline.WorkApplicationRequirementCandidateKind:
				input, err := applicationRequirementCandidateContentPresenceInputForTest(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				dimensions = append(dimensions, input.Dimension)
				return assemblyline.PortableResult{
					JobID:     job.ID,
					Candidate: string(assemblyline.ApplicationRequirementCandidateContentAbsent),
				}, nil
			default:
				return assemblyline.PortableResult{}, fmt.Errorf(
					"runtime-absent candidate reached downstream work %q",
					job.Kind,
				)
			}
		},
	}
	resolved, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime,
		"intent-model",
		authority,
		entry,
		nil,
		nil,
		nil,
	)
	if err != nil || resolved.Disposition != directCodingApplicationRequirementUnresolved {
		t.Fatalf("resolution=%+v calls=%v error=%v", resolved, calls, err)
	}
	if want := []assemblyline.WorkKind{
		assemblyline.WorkApplicationRequirementCandidateAuthorization,
		assemblyline.WorkApplicationRequirementCandidateKind,
	}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("semantic calls=%v want=%v", calls, want)
	}
	if want := []assemblyline.ApplicationRequirementCandidateContentDimension{
		assemblyline.ApplicationRequirementCandidateRuntimeContentDimension,
	}; !reflect.DeepEqual(dimensions, want) {
		t.Fatalf("content dimensions=%v want=%v", dimensions, want)
	}
}

func applicationRequirementQueuePartitionFixture(
	t *testing.T,
	parent string,
	raw string,
) (
	assemblyline.ApplicationRequirementCandidatePartitionInput,
	assemblyline.ApplicationRequirementCandidatePartition,
) {
	t.Helper()
	cardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
		Candidate: parent,
	}
	cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		cardinalityInput,
		assemblyline.ApplicationRequirementMultipleRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	input := assemblyline.ApplicationRequirementCandidatePartitionInput{
		Candidate:   parent,
		Cardinality: &cardinality,
	}
	partition, err := assemblyline.DecodeApplicationRequirementCandidatePartition(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	return input, partition
}
