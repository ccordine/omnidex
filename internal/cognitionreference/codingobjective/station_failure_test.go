package codingobjective

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestStationCannotMutateFrozenJobOrCommitAfterCancellation(t *testing.T) {
	t.Run("mutated payload", func(t *testing.T) {
		fixture := numericCodingFixture(t)
		before := captureExactWorkspace(t, fixture.root)
		station := &mutatingDeclarationStation{candidate: fixture.candidate}
		applyCalls := 0
		result, err := runWithOperations(
			context.Background(), fixtureObjective(fixture), station,
			operations{apply: recordingApply(&applyCalls)},
		)
		if err == nil || !strings.Contains(err.Error(), "mutated its immutable job") {
			t.Fatalf("mutating station error=%v", err)
		}
		if result.ModelCalls != 1 || station.calls != 1 || applyCalls != 0 || result.Complete {
			t.Fatalf("mutating station result=%+v calls=%d apply=%d", result, station.calls, applyCalls)
		}
		assertExactWorkspaceUnchanged(t, fixture.root, before)
	})

	t.Run("canceled during station", func(t *testing.T) {
		fixture := numericCodingFixture(t)
		before := captureExactWorkspace(t, fixture.root)
		ctx, cancel := context.WithCancel(context.Background())
		station := &cancelingDeclarationStation{cancel: cancel, candidate: fixture.candidate}
		applyCalls := 0
		result, err := runWithOperations(
			ctx, fixtureObjective(fixture), station,
			operations{apply: recordingApply(&applyCalls)},
		)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel-during-station error=%v", err)
		}
		if result.ModelCalls != 1 || station.calls != 1 || applyCalls != 0 || result.Complete {
			t.Fatalf("canceled station result=%+v calls=%d apply=%d", result, station.calls, applyCalls)
		}
		assertExactWorkspaceUnchanged(t, fixture.root, before)
	})
}

type mutatingDeclarationStation struct {
	candidate string
	calls     int
}

func (station *mutatingDeclarationStation) Generate(
	_ context.Context,
	job assemblyline.PortableJob,
) (assemblyline.PortableResult, error) {
	station.calls++
	jobID := job.ID
	if len(job.Payload) > 0 {
		job.Payload[0] ^= 0xff
	}
	return assemblyline.PortableResult{JobID: jobID, Candidate: station.candidate}, nil
}

type cancelingDeclarationStation struct {
	cancel    context.CancelFunc
	candidate string
	calls     int
}

func (station *cancelingDeclarationStation) Generate(
	_ context.Context,
	job assemblyline.PortableJob,
) (assemblyline.PortableResult, error) {
	station.calls++
	station.cancel()
	return assemblyline.PortableResult{JobID: job.ID, Candidate: station.candidate}, nil
}
