package host

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func fencedHostAttestedBrain(t *testing.T) cognitionpolicy.AttestedBrain {
	t.Helper()
	host, err := cognitionpolicy.NewHostHardwareAttestation(
		"linux", "amd64", 1, fencedHostTestDigest, fencedHostTestDigest,
	)
	if err != nil {
		t.Fatal(err)
	}
	sampling, err := cognitionpolicy.NewSamplingIdentity(32_768, 16_384, 256)
	if err != nil {
		t.Fatal(err)
	}
	brain, err := cognitionpolicy.NewBrainRef(
		"host-fence-model", fencedHostTestDigest, "Q4", "host-fence-provider", "1.0.0",
		"host-attestation:"+host.AttestationSHA256, sampling,
	)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := brain.ProviderExpectation()
	if err != nil {
		t.Fatal(err)
	}
	provider, err := llm.NewProviderIdentityAttestation(
		expected, "backend-version", "installed-model", "runner-allocation",
	)
	if err != nil {
		t.Fatal(err)
	}
	attested, err := cognitionpolicy.NewAttestedBrain(brain, provider, host)
	if err != nil {
		t.Fatal(err)
	}
	return attested
}

func stepAuthority(actor cognition.AttemptRef) model.StepAttemptAuthority {
	return model.StepAttemptAuthority{
		JobID: actor.JobID, Generation: actor.Generation, StepID: actor.StepID,
		Attempt: int64(actor.Attempt), WorkerID: actor.WorkerID,
	}
}

func attemptRef(authority model.StepAttemptAuthority) cognition.AttemptRef {
	return cognition.AttemptRef{
		JobID: authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
		Attempt: uint64(authority.Attempt), WorkerID: authority.WorkerID,
	}
}

func fencedWitnessAction(
	t *testing.T,
	fixture fencedHostFixture,
	started cognition.Transition,
	actor cognition.AttemptRef,
) cognition.RegisteredAction {
	t.Helper()
	witness := fixture.witness[0]
	schema, exists := fixture.scenario.Catalog().Schema(witness.Request.Kind)
	if !exists {
		t.Fatalf("witness schema %q is absent", witness.Request.Kind)
	}
	var evidence []cognition.EvidenceRef
	if schema.EvidencePolicy == cognition.EvidenceRequired {
		evidence = make([]cognition.EvidenceRef, len(started.Observations))
		for index, observation := range started.Observations {
			evidence[index] = observation.EvidenceRef()
		}
	}
	action, err := cognition.NewRegisteredAction(
		"lease-fenced-action", actor, schema, witness.Request, evidence,
	)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func assertHostHead(
	t *testing.T,
	fixture fencedHostFixture,
	want cognition.WorldRevision,
	wantReceipts int,
) {
	t.Helper()
	receipt, err := fixture.store.Episode(t.Context(), fixture.episode)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := fixture.pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM `+fixture.store.relation("action_receipts")+` WHERE episode_id=$1
	`, fixture.episode.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if receipt.Current != want || count != wantReceipts {
		t.Fatalf("host head/receipts=%+v/%d want=%+v/%d", receipt.Current, count, want, wantReceipts)
	}
}
