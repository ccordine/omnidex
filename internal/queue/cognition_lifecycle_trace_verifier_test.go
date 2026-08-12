package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
)

func TestVerifyCognitionLifecycleRetirementTraceAuthorityRejectsEveryAssociationChange(t *testing.T) {
	fixture := newCognitionLifecycleRetirementUnitFixture(t)
	retirement, err := newCognitionLifecycleRetirement(
		fixture.descriptor, fixture.episode, fixture.graph,
		cognitionruntime.CancellationGenerationRetired,
	)
	if err != nil {
		t.Fatal(err)
	}
	raw, sha, err := cognitionJSON(retirement)
	if err != nil {
		t.Fatal(err)
	}
	record := CognitionSealedTraceRecord{
		Kind: "lifecycle_retirement", Phase: 79, ID: retirement.ID,
		SHA256: sha, Payload: raw,
	}
	cancellation, err := cognitionruntime.NewLifecycleCancellationEvidence(
		retirement.Code, "Lifecycle retirement test.", retirement.OperationSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	seal := CognitionTerminalSeal{
		EpisodeID: retirement.EpisodeID, Outcome: CognitionEpisodeCanceled,
		FinalRevision:         retirement.ExpectedRevision,
		ObligationGraphSHA256: retirement.GraphSHA256,
		AuthorityKind:         cognitionTerminalAuthorityLifecycle,
		SealedBy: model.StepAttemptAuthority{
			JobID: retirement.JobID, Generation: retirement.JobGeneration, StepID: retirement.StepID,
		},
		LifecycleOperationID: retirement.OperationID,
	}
	verify := func(record CognitionSealedTraceRecord, seal CognitionTerminalSeal,
		version uint64, graphSHA string, evidence cognitionruntime.CancellationEvidence) error {
		return VerifyCognitionLifecycleRetirementTraceAuthority(
			record, seal, version, graphSHA, evidence,
		)
	}
	if err := verify(record, seal, retirement.GraphVersion, retirement.GraphSHA256, cancellation); err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*CognitionSealedTraceRecord, *CognitionTerminalSeal, *uint64, *string, *cognitionruntime.CancellationEvidence){
		"tuple": func(record *CognitionSealedTraceRecord, _ *CognitionTerminalSeal, _ *uint64, _ *string, _ *cognitionruntime.CancellationEvidence) {
			record.Phase++
		},
		"authority": func(_ *CognitionSealedTraceRecord, seal *CognitionTerminalSeal, _ *uint64, _ *string, _ *cognitionruntime.CancellationEvidence) {
			seal.AuthorityKind = cognitionTerminalAuthorityWorker
		},
		"operation": func(_ *CognitionSealedTraceRecord, seal *CognitionTerminalSeal, _ *uint64, _ *string, _ *cognitionruntime.CancellationEvidence) {
			seal.LifecycleOperationID = ""
		},
		"actor": func(_ *CognitionSealedTraceRecord, seal *CognitionTerminalSeal, _ *uint64, _ *string, _ *cognitionruntime.CancellationEvidence) {
			seal.SealedBy.JobID++
		},
		"revision": func(_ *CognitionSealedTraceRecord, seal *CognitionTerminalSeal, _ *uint64, _ *string, _ *cognitionruntime.CancellationEvidence) {
			seal.FinalRevision.Number++
		},
		"version": func(_ *CognitionSealedTraceRecord, _ *CognitionTerminalSeal, version *uint64, _ *string, _ *cognitionruntime.CancellationEvidence) {
			(*version)++
		},
		"graph": func(_ *CognitionSealedTraceRecord, _ *CognitionTerminalSeal, _ *uint64, graph *string, _ *cognitionruntime.CancellationEvidence) {
			*graph = cognitionTestDigest("changed-graph")
		},
		"code": func(_ *CognitionSealedTraceRecord, _ *CognitionTerminalSeal, _ *uint64, _ *string, evidence *cognitionruntime.CancellationEvidence) {
			evidence.Code = cognitionruntime.CancellationJobCanceled
		},
		"source": func(_ *CognitionSealedTraceRecord, _ *CognitionTerminalSeal, _ *uint64, _ *string, evidence *cognitionruntime.CancellationEvidence) {
			evidence.SourceErrorSHA256 = cognitionTestDigest("changed-source")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changedRecord, changedSeal := record, seal
			version, graph, evidence := retirement.GraphVersion, retirement.GraphSHA256, cancellation
			mutate(&changedRecord, &changedSeal, &version, &graph, &evidence)
			if verify(changedRecord, changedSeal, version, graph, evidence) == nil {
				t.Fatal("changed lifecycle association was accepted")
			}
		})
	}
}
