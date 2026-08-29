package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestPostgresRoleplayPortableResultReuseRejectsDivergentExactResults(t *testing.T) {
	fixture := newRoleplayPortableReuseDatabaseFixture(t, "roleplay-reuse-divergent")
	root := roleplayPortableReuseRootJob(t, "resolve one ambiguity-sensitive leaf")
	instruction := "Keep the gate closed."
	seedRoleplayPortableReuseAcceptedSource(
		t, fixture, "divergent-first", instruction, root,
		"The first accepted result.",
	)
	seedRoleplayPortableReuseAcceptedSource(
		t, fixture, "divergent-second", instruction, root,
		"The divergent accepted result.",
	)

	request, targetJob := claimRoleplayPortableReuseTarget(
		t, fixture, "divergent-target", instruction, root,
	)
	reused, found, err := fixture.Repository.ReuseRoleplayPortableResult(t.Context(), request)
	if err == nil || !strings.Contains(err.Error(), "divergent exact results") {
		t.Fatalf("divergent reuse=%+v found=%t err=%v", reused, found, err)
	}
	if found {
		t.Fatalf("divergent reuse unexpectedly found result: %+v", reused)
	}
	var receipts int
	if err := fixture.Pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM roleplay_portable_result_reuses WHERE target_job_id=$1
	`, targetJob.ID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 0 {
		t.Fatalf("divergent reuse persisted %d receipts before rejecting ambiguity", receipts)
	}
}

func TestPostgresRoleplayPortableResultReuseAllowsIdenticalExactResults(t *testing.T) {
	fixture := newRoleplayPortableReuseDatabaseFixture(t, "roleplay-reuse-identical")
	root := roleplayPortableReuseRootJob(t, "resolve one duplicate leaf")
	instruction := "Watch the eastern road."
	first := seedRoleplayPortableReuseAcceptedSource(
		t, fixture, "identical-first", instruction, root, roleplayPortableReuseExactCandidate,
	)
	second := seedRoleplayPortableReuseAcceptedSource(
		t, fixture, "identical-second", instruction, root, roleplayPortableReuseExactCandidate,
	)
	request, targetJob := claimRoleplayPortableReuseTarget(
		t, fixture, "identical-target", instruction, root,
	)

	reused, found, err := fixture.Repository.ReuseRoleplayPortableResult(t.Context(), request)
	if err != nil || !found {
		t.Fatalf("identical reuse=%+v found=%t err=%v", reused, found, err)
	}
	if reused.Result.Candidate != roleplayPortableReuseExactCandidate ||
		reused.Receipt.SourceGapOutcomeID != second.Outcome.ID ||
		reused.Receipt.SourceAuthority.JobID != second.Job.ID {
		t.Fatalf("identical reuse did not retain deterministic newest provenance: %+v", reused)
	}
	if reused.Receipt.SourceGapOutcomeID == first.Outcome.ID {
		t.Fatalf("identical reuse selected older source outcome %d", first.Outcome.ID)
	}

	again, againFound, err := fixture.Repository.ReuseRoleplayPortableResult(t.Context(), request)
	if err != nil || !againFound || again.Receipt.ID != reused.Receipt.ID ||
		again.Receipt.SourceGapOutcomeID != reused.Receipt.SourceGapOutcomeID {
		t.Fatalf("idempotent identical reuse=%+v found=%t err=%v", again, againFound, err)
	}
	var receipts int
	if err := fixture.Pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM roleplay_portable_result_reuses WHERE target_job_id=$1
	`, targetJob.ID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 {
		t.Fatalf("identical reuse receipts=%d want 1", receipts)
	}
}

type roleplayPortableReuseAcceptedSource struct {
	Job     model.Job
	Outcome StationGapOutcome
}

func seedRoleplayPortableReuseAcceptedSource(
	t *testing.T,
	fixture roleplayPortableReuseDatabaseFixture,
	marker string,
	instruction string,
	root assemblyline.PortableJob,
	candidate string,
) roleplayPortableReuseAcceptedSource {
	t.Helper()
	_, job, err := enqueueNarratorRoleplayTurn(
		t.Context(), fixture.Repository, fixture.Channel.ID, instruction,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.Repository.ClaimNextStep(t.Context(), marker+"-source")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("source claim=%+v err=%v", claim, err)
	}
	outcome := persistRoleplayPortableReuseLeaf(
		t, fixture.Repository, claim, root, candidate,
	)
	failRoleplayPortableReuseJob(t, fixture.Repository, claim, marker+"-failure")
	return roleplayPortableReuseAcceptedSource{Job: job, Outcome: outcome}
}

func claimRoleplayPortableReuseTarget(
	t *testing.T,
	fixture roleplayPortableReuseDatabaseFixture,
	marker string,
	instruction string,
	root assemblyline.PortableJob,
) (RoleplayPortableResultReuseRequest, model.Job) {
	t.Helper()
	_, job, err := enqueueNarratorRoleplayTurn(
		t.Context(), fixture.Repository, fixture.Channel.ID, instruction,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.Repository.ClaimNextStep(t.Context(), marker+"-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("target claim=%+v err=%v", claim, err)
	}
	return RoleplayPortableResultReuseRequest{
		Authority: claim.Authority,
		Job:       root,
		Station:   roleplayPortableReuseStation(t, root),
	}, job
}
