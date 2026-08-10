package cognitiongauntlet

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/workingset"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This is contaminated world-validation machinery: the private witness policy
// proves one production runtime crosses two real surfaces without code changes.
func TestPostgresFullCognitionTransferExecutesFilesystemAndRecordSurfaces(t *testing.T) {
	ctx, pool, repository, hostStore := openFullCognitionDatabase(t)
	fixture, err := GenerateMicrogauntlet(InitialMicrogauntletsV1()[4])
	if err != nil {
		t.Fatal(err)
	}
	generation := mustRatGeneration(t)
	surfaces := []Surface{SurfaceFilesystem, SurfaceRecord}
	requests := make([]FullCognitionRunRequest, len(surfaces))
	for index, surface := range surfaces {
		claim := claimScaleStep(t, repository, fixture.spec.Budget.WorkingSetBytes, index)
		episodeDirectory, evaluationDirectory := t.TempDir(), t.TempDir()
		requests[index] = FullCognitionRunRequest{
			Surface: surface, RatGeneration: generation,
			RuntimeFingerprint: transferTestFingerprint(), Repetition: 1,
			Attempt: claim, Pool: pool, HostStore: hostStore,
			Client: &witnessPolicyClient{
				model:        generation.Fixed.Brain.Model,
				witness:      fixture.generated.PrivateOracle().Witness,
				evidenceUses: fixture.generated.PrivateOracle().EvidenceUses,
			},
			EpisodeSealPath:     filepath.Join(episodeDirectory, "episode.json"),
			EvaluationPath:      filepath.Join(evaluationDirectory, "evaluation.json"),
			LedgerSchemaVersion: "task-ledger.v1", WorkingSetPolicyVersion: "working-set.v1",
			ProjectionPolicyVersion: "context-projection.v1",
		}
	}
	result, err := RunFullCognitionTransfer(ctx, fixture, surfaces, requests)
	if err != nil {
		for _, request := range requests {
			diagnoseTransferEvidence(t, pool, repository, fixture, request)
		}
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if !result.Report.Gate.Passed || len(result.Report.Gate.Reasons) != 0 {
		t.Fatalf("actual transfer report=%+v", result.Report)
	}
}

func diagnoseTransferEvidence(
	t *testing.T,
	pool *pgxpool.Pool,
	repository interface {
		CurrentWorkingSet(context.Context, int64) (workingset.Snapshot, error)
		CognitionEpisode(context.Context, cognition.EpisodeID) (queue.CognitionEpisode, error)
	},
	fixture MicrogauntletCase,
	request FullCognitionRunRequest,
) {
	t.Helper()
	authority, err := fixture.PairedAuthority(
		request.Surface, request.RatGeneration, request.Repetition, request.RuntimeFingerprint,
	)
	if err != nil {
		t.Log(err)
		return
	}
	episodeRef, err := VariantEpisodeRef(authority, VariantFullCognition)
	if err != nil {
		t.Log(err)
		return
	}
	episode, episodeErr := repository.CognitionEpisode(t.Context(), episodeRef.ID)
	set, setErr := repository.CurrentWorkingSet(t.Context(), request.Attempt.JobID)
	t.Logf("failed transfer episode=%+v episode_error=%v working_set_error=%v", episode, episodeErr, setErr)
	for _, item := range set.Items {
		if item.Role == workingset.RoleEvidence && item.State == workingset.ItemResident {
			t.Logf("resident evidence item=%s ref=%+v memberships=%+v retention=%s", item.ID, item.Ref, item.Memberships, item.Retention)
		}
	}
	rows, queryErr := pool.Query(t.Context(), `
		SELECT expected_revision,status,decision_json,COALESCE(failure_json,'')
		FROM cognition_actions WHERE episode_id=$1 ORDER BY expected_revision
	`, episodeRef.ID)
	if queryErr != nil {
		t.Log(queryErr)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var revision int64
		var status, decision, failure string
		if err := rows.Scan(&revision, &status, &decision, &failure); err != nil {
			t.Log(err)
			return
		}
		t.Logf("action revision=%d status=%s decision=%s failure=%s", revision, status, decision, failure)
	}
}
