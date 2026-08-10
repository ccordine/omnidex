package queue

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

type strandingCognitionCallJournal struct {
	repository *Repository
	attempt    cognitionpolicy.CallAttempt
}

func (journal *strandingCognitionCallJournal) Start(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
) (cognitionpolicy.CallReservation, error) {
	reservation, err := journal.repository.StartCognitionPolicyCall(ctx, attempt)
	if err != nil {
		return cognitionpolicy.CallReservation{}, err
	}
	journal.attempt = attempt
	return reservation, errors.New("injected crash after durable cognition call start")
}

func (*strandingCognitionCallJournal) Finish(
	context.Context,
	cognitionpolicy.CallAttempt,
	cognitionpolicy.CallResult,
) error {
	return errors.New("stranding journal cannot finish")
}

func reserveIndeterminateCognitionCall(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
) cognitionpolicy.CallAttempt {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context,
		CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	journal := &strandingCognitionCallJournal{repository: fixture.Repository}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: "unused"}, cognitionTestBrain(),
		cognitionGuardProjectionLoader{repository: fixture.Repository}, journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(fixture.Context, prepared.Prepared.Snapshot); !errors.Is(err, cognitionpolicy.ErrCallJournal) {
		t.Fatalf("stranded policy error=%v", err)
	}
	if journal.attempt.ID == "" {
		t.Fatal("stranding journal captured no call attempt")
	}
	return journal.attempt
}

func reserveTerminalCognitionPolicyCall(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	client llm.Client,
) error {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context,
		CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := cognitionpolicy.New(
		client, cognitionTestBrain(), cognitionGuardProjectionLoader{repository: fixture.Repository},
		CognitionPolicyCallJournal{Repository: fixture.Repository},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = policy.Decide(fixture.Context, prepared.Prepared.Snapshot)
	return err
}

type failingCognitionPolicyClient struct{ cognitionGuardPolicyClient }

func (failingCognitionPolicyClient) GeneratePrepared(context.Context, llm.PreparedModel) (string, error) {
	return "", fmt.Errorf("injected provider failure")
}

func providerIdentityFailureResult(attempt cognitionpolicy.CallAttempt) cognitionpolicy.CallResult {
	return cognitionpolicy.CallResult{
		Schema: cognitionpolicy.CallResultSchemaV2, CallID: attempt.ID,
		Status: cognitionpolicy.CallResultFailed, FailureCode: cognitionpolicy.CallFailureProviderIdentity,
		FailureMessage: "The frozen provider identity changed.",
	}
}

func countCognitionRecoveryRows(t *testing.T, fixture taskGenerationRetirementFixture) (int, int, int) {
	t.Helper()
	var snapshots, calls, abandonments int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT
		 (SELECT COUNT(*) FROM cognition_runtime_snapshots WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_policy_calls WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_policy_call_abandonments WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&snapshots, &calls, &abandonments); err != nil {
		t.Fatal(err)
	}
	return snapshots, calls, abandonments
}

func emptyCognitionEvidence() []cognition.EvidenceRef { return []cognition.EvidenceRef{} }
