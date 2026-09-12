package worker

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestDatabaseWorkflowFailureRetainsActualCallsWithoutDuplicateLedger(t *testing.T) {
	for _, fixture := range databaseWorkflowCases() {
		t.Run(fixture.table, func(t *testing.T) {
			run := newDatabaseWorkflowFixture(t, fixture)
			raw := assemblyline.DatabaseNoQueryPurposeCandidates + "\nReturn each " + fixture.field + "."
			run.provider.fixtures = []exactEvidenceStationFixture{{candidate: "A"}, {candidate: raw}}
			ctx := context.Background()
			requirement := objectiveRequirementID(objectiveTurnID(run.authority))
			for attempt := range 2 {
				result, err := runObjectiveDatabaseEvidenceWorkflow(ctx, run.authority, requirement,
					run.snapshot, portableObjectiveDatabaseStations{runtime: run.runtime()}, run.execute)
				if err == nil || !strings.Contains(err.Error(), "cannot mix") || len(result.Evidence) != 0 ||
					result.ModelCalls != 2*(1-attempt) || run.provider.calls != 2 {
					t.Fatalf("attempt=%d result=%+v provider=%d error=%v", attempt, result, run.provider.calls, err)
				}
			}
			calls, err := listAllWorkerLLMCallEvidence(ctx, run.repository, run.claim.Job.ID)
			if err != nil || len(calls) != 2 {
				t.Fatalf("recorded calls=%d error=%v", len(calls), err)
			}
			assertPortableLeafRecordedCall(t, calls[0], run.provider.prepared[0], assemblyline.WorkDatabaseQueryShape, "A", "fixture-query")
			request, err := llm.ExactPreparedRequestBytes(run.provider.prepared[1])
			if err != nil {
				t.Fatal(err)
			}
			generation, err := exactEvidenceSuccessfulGeneration(run.provider.prepared[1], raw)
			if err != nil {
				t.Fatal(err)
			}
			recorded := calls[1]
			if recorded.WorkKind != string(assemblyline.WorkDatabaseQueryPurposeInventory) ||
				recorded.Candidate != raw || recorded.Model != "fixture-query" || recorded.RequestedModel != "fixture-query" ||
				recorded.ModelInputBytes != len(recorded.ModelInput) || !bytes.Equal(recorded.ProviderRequest, request) ||
				!recorded.RawResponsePresent || !bytes.Equal(recorded.RawResponse, generation.ProviderResponseCapture) ||
				recorded.Outcome == nil || recorded.Outcome.Status != queue.LLMCallRejected ||
				!strings.Contains(recorded.Outcome.ValidationError, "cannot mix") {
				t.Fatalf("failed call lost its actual input, output, or error: %+v", recorded)
			}
			var reads int
			if err := run.pool.QueryRow(ctx, "SELECT count(*) FROM database_evidence WHERE job_id=$1", run.claim.Job.ID).Scan(&reads); err != nil || reads != 0 {
				t.Fatalf("invalid query interpretation reached execution: reads=%d error=%v", reads, err)
			}
		})
	}
}
