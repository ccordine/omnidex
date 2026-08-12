package codingobjective

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestExistingRepositoryCodingObjectiveCompletesThroughOneBoundedDeclaration(t *testing.T) {
	fixtures := []struct {
		name string
		new  func(*testing.T) codingFixture
	}{
		{name: "numeric delegation", new: numericCodingFixture},
		{name: "presentation delegation", new: presentationCodingFixture},
	}
	for _, test := range fixtures {
		t.Run(test.name, func(t *testing.T) {
			fixture := test.new(t)
			before := snapshotFixture(t, fixture.root)
			station := &recordingDeclarationStation{candidate: fixture.candidate}

			result, err := Run(context.Background(), fixtureObjective(fixture), station)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Complete || result.ModelCalls != 1 || station.calls != 1 {
				t.Fatalf("complete=%v model calls=%d/%d", result.Complete, result.ModelCalls, station.calls)
			}
			if result.ObjectiveID != fixtureObjective(fixture).ID || result.CommitOutcome != CommitSucceeded {
				t.Fatalf("objective/commit binding=%+v", result)
			}
			if !result.Satisfied || !reflect.DeepEqual(
				result.Acceptance, []AcceptancePredicate{AcceptanceGoTestsPass},
			) {
				t.Fatalf("acceptance binding=%+v", result)
			}
			if result.BeforeSnapshotID != before.ID || result.StageID == "" || result.PatchSHA256 == "" || len(result.ChangedFileIDs) != 1 {
				t.Fatalf("accepted authority=%+v, initial=%q", result, before.ID)
			}
			if len(result.ExpectedFiles) != 1 {
				t.Fatalf("expected file authority=%+v", result.ExpectedFiles)
			}
			if err := reconcileExpectedRepository(
				context.Background(), fixture.root, before, result.ExpectedFiles,
			); err != nil {
				t.Fatalf("final authoritative bytes do not match result: %v", err)
			}
			if result.DirectCapabilities != 1 || result.DirectTests != 1 {
				t.Fatalf("direct capabilities/tests=%d/%d, want 1/1", result.DirectCapabilities, result.DirectTests)
			}
			if got, want := result.Steps, successfulStepSequence(); !reflect.DeepEqual(got, want) {
				t.Fatalf("steps=%v, want %v", got, want)
			}
			if station.job.Kind != assemblyline.WorkFragmentModification {
				t.Fatalf("station work kind=%q", station.job.Kind)
			}
			for _, forbidden := range append(
				fixture.forbidden, fixture.root, fixtureObjective(fixture).ID,
				string(AcceptanceGoTestsPass),
			) {
				if strings.Contains(station.prompt, forbidden) {
					t.Fatalf("model envelope leaked %q:\n%s", forbidden, station.prompt)
				}
			}
			for _, required := range []string{fixture.requirement, fixture.directCapability, "CURRENT_DECLARATION", "EXACT_SIGNATURE"} {
				if !strings.Contains(station.prompt, required) {
					t.Fatalf("model envelope omitted %q:\n%s", required, station.prompt)
				}
			}
		})
	}
}

var errUnexpectedFragmentSchema = errors.New("fragment modification unexpectedly exposed a response schema")

type recordingDeclarationStation struct {
	candidate string
	resultID  string
	err       error
	calls     int
	job       assemblyline.PortableJob
	prompt    string
}

func (station *recordingDeclarationStation) Generate(
	_ context.Context,
	job assemblyline.PortableJob,
) (assemblyline.PortableResult, error) {
	station.calls++
	station.job = job
	prompt, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	if schema != nil {
		return assemblyline.PortableResult{}, errUnexpectedFragmentSchema
	}
	station.prompt = prompt
	if station.err != nil {
		return assemblyline.PortableResult{}, station.err
	}
	jobID := station.resultID
	if jobID == "" {
		jobID = job.ID
	}
	return assemblyline.PortableResult{JobID: jobID, Candidate: station.candidate}, nil
}
