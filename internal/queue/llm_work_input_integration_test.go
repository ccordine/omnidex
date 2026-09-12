package queue

import (
	"bytes"
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPersistedSemanticWorkUsesExactInputsAndCallIDs(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	for _, fixture := range []struct {
		kind              assemblyline.WorkKind
		prompt, candidate string
	}{
		{assemblyline.WorkApplicationClassify, "Classify one interface.", "A"},
		{assemblyline.WorkFragmentGeneration, "Return the sum of both inputs.", "return left + right;"},
	} {
		t.Run(string(fixture.kind), func(t *testing.T) {
			pool, repository := freshEvidenceRepository(t, databaseURL)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJob(ctx, "exercise semantic work reuse", t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "work-input-test")
			if err != nil || claim == nil || claim.Job.ID != job.ID {
				t.Fatalf("claim = %#v, error = %v", claim, err)
			}
			record := exactLLMEvidenceFixture(t, fixture.kind, fixture.prompt, fixture.candidate)
			record.Authority = claim.Authority
			work := assemblyline.PortableJob{
				Schema: assemblyline.PortableJobSchemaV2, Kind: fixture.kind, Payload: record.WorkInput,
			}
			if _, found, err := repository.ReusableLLMCallRootEvidence(ctx, claim.Authority, work); err != nil || found {
				t.Fatalf("new input already has a provider call: found = %t, error = %v", found, err)
			}
			stored, err := recordExactLLMEvidenceFixture(ctx, repository, record)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
				Authority: claim.Authority, CallEvidenceID: stored.ID, Candidate: fixture.candidate,
			}); err != nil {
				t.Fatal(err)
			}
			reused, found, err := repository.ReusableLLMCallRootEvidence(ctx, claim.Authority, work)
			if err != nil || !found || reused.ID != stored.ID ||
				!bytes.Equal(reused.WorkInput, work.Payload) || reused.Candidate != fixture.candidate ||
				reused.Outcome == nil || reused.Outcome.Status != LLMCallAccepted {
				t.Fatalf("exact input did not recover its accepted call: %#v, found = %t, error = %v", reused, found, err)
			}
			if _, err := repository.ReserveLLMCallEvidence(ctx, record.LLMCallOpeningRecord); err == nil {
				t.Fatal("same initial input reserved a second provider call")
			}
			changed := work
			changed.Payload = []byte(`"different necessary question"`)
			if _, found, err := repository.ReusableLLMCallRootEvidence(ctx, claim.Authority, changed); err != nil || found {
				t.Fatalf("changed input recovered old work: found = %t, error = %v", found, err)
			}
			changed = work
			changed.Kind = assemblyline.WorkArtifactHandling
			if _, found, err := repository.ReusableLLMCallRootEvidence(ctx, claim.Authority, changed); err != nil || found {
				t.Fatalf("different station recovered old work: found = %t, error = %v", found, err)
			}
			var obsoleteColumns int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
				WHERE table_schema=current_schema() AND table_name='llm_call_evidence' AND column_name='work_id'
			`).Scan(&obsoleteColumns); err != nil || obsoleteColumns != 0 {
				t.Fatalf("content-addressed work column remains: count = %d, error = %v", obsoleteColumns, err)
			}
		})
	}
}
