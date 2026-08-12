package cognitiongauntlet

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/labyrinth"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

// semanticReplayMatrixFixture is contaminated deterministic test machinery.
// Its private witness proves the serious replay boundary is wired to an exact
// preregistered Matrix coordinate; it is never competence or autonomy evidence.
func semanticReplayMatrixFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	repository *queue.Repository,
	hostStore *labyrinthhost.Store,
) (PublicInferenceBundle, PublicFullCognitionRunRequest, matrixReplayPreregistration) {
	t.Helper()
	seed := InitialMicrogauntletsV2()[0].Generator.Seed
	config, registration := replayPreregistrationTestConfig(t, []uint64{seed})
	coordinate := registration.Cases[0]
	credential, err := loadMatrixReplayPreregistration(
		config, coordinate.ID, VariantFullCognition,
	)
	if err != nil {
		t.Fatal(err)
	}
	run, err := config.derivedRunConfig(coordinate, VariantFullCognition)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := generateOfflineScenario(run.Scenario)
	if err != nil {
		t.Fatal(err)
	}
	paired, err := generated.pairedAuthority(
		run.Surface, run.RatGeneration, run.Repetition, run.RuntimeFingerprint,
	)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := newScenarioPublicInferenceBundle(
		generated.scenario, paired, VariantFullCognition,
	)
	if err != nil || bundle.Authority != credential.authority {
		t.Fatalf("derive preregistered semantic replay bundle: %v", err)
	}
	claim := claimFailureStep(t, repository, run.Scenario.Budget().WorkingSetBytes)
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		t.Fatal(err)
	}
	store, err := cognitionstore.New(
		repository, cognitionstate.NewNoFactAcceptanceAuthority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver := func(_ context.Context, ref cognition.ScenarioRef) (labyrinth.Scenario, error) {
		if ref != generated.scenario.Ref() {
			return labyrinth.Scenario{}, cognition.ErrInvalidScenario
		}
		return generated.scenario, nil
	}
	surfaceVersion, err := run.Surface.Version()
	if err != nil {
		t.Fatal(err)
	}
	environment, err := labyrinthhost.NewSurfaceEnvironment(
		hostStore, episode, resolver, store.AuthorizeAttempt,
		transactionAttemptAuthorizer(repository), surfaceVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	evidence := generated.evidenceAuthority()
	request := PublicFullCognitionRunRequest{
		Attempt: claim, Pool: pool,
		Client: &witnessPolicyClient{
			model: run.RatGeneration.Fixed.Brain.Model, witness: evidence.Witness,
			evidenceUses: evidence.EvidenceUses, emitHypothesis: true,
		},
		Environment: environment, Completion: environment,
		EpisodeSealPath:         filepath.Join(t.TempDir(), "episode.json"),
		OmnidexCommit:           credential.execution.omnidexCommit,
		LedgerSchemaVersion:     credential.execution.ledgerSchemaVersion,
		WorkingSetPolicyVersion: credential.execution.workingSetPolicyVersion,
		ProjectionPolicyVersion: credential.execution.projectionPolicyVersion,
	}
	return bundle, request, credential
}
