package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOpenStationGapRejectsNonAtomicFragmentGenerationReplacement(t *testing.T) {
	job, err := assemblyline.NewFragmentGenerationReplacementJob(
		assemblyline.FragmentGenerationReplacementInput{
			Original: assemblyline.FragmentGenerationInput{
				Language: "go", Dialect: "Go 1.24", Signature: "func Value() int",
				Behavior: "Return one bounded value.",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New(nil).OpenStationGap(
		t.Context(), StationGapOpenRecord{Job: job},
	); err == nil || !strings.Contains(err.Error(), "requires atomic station gap and provider discovery opening") {
		t.Fatalf("non-atomic replacement opening error=%v", err)
	}
}

func TestPostgresReplacementProductionOpeningRejectsProviderDriftAtomically(t *testing.T) {
	fixture := newReplacementTransitionFixture(t, "replacement-provider-drift")
	wrong := llm.ProviderIdentitySelection{
		Model: "different-model:9b", NativeContextLimit: fixture.gap.ContextTokens,
	}
	if _, err := fixture.repository.OpenStationGapDiscovery(
		t.Context(), StationGapDiscoveryOpenRecord{Gap: fixture.gap, Selection: wrong},
	); err == nil || !strings.Contains(err.Error(), "exact origin provider model") {
		t.Fatalf("mismatched replacement provider error=%v", err)
	}
	assertReplacementTransitionCounts(t, fixture.pool, 0, 0, 0, 0)

	valid := llm.ProviderIdentitySelection{
		Model: fixture.originModel, NativeContextLimit: fixture.gap.ContextTokens,
	}
	opened, err := fixture.repository.OpenStationGapDiscovery(
		t.Context(), StationGapDiscoveryOpenRecord{Gap: fixture.gap, Selection: valid},
	)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Gap.ID < 1 || opened.Discovery.GapOpeningID != opened.Gap.ID {
		t.Fatalf("replacement production opening=%+v", opened)
	}
	assertReplacementTransitionCounts(t, fixture.pool, 1, 1, 0, 0)
}

func TestPostgresReplacementCallOpeningIndependentlyRejectsProviderDrift(t *testing.T) {
	fixture := newReplacementTransitionFixture(t, "replacement-call-provider-drift")
	gap := insertValidatedReplacementGapFixture(t, fixture)
	if _, err := fixture.pool.Exec(t.Context(), `
		DROP TRIGGER station_provider_replacement_model_required
		ON station_provider_discoveries
	`); err != nil {
		t.Fatal(err)
	}

	prepared := replacementTransitionPrepared(t, gap, "different-model:9b")
	discovery := persistStationDiscoverySuccess(
		t, fixture.repository, fixture.authority, gap, prepared,
	)
	if _, err := fixture.repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: fixture.authority, Gap: gap, Discovery: discovery, Prepared: prepared,
	}); err == nil || !strings.Contains(
		err.Error(), "station call opening does not match its exact gap authority",
	) {
		t.Fatalf("mismatched replacement call error=%v", err)
	}
	assertReplacementTransitionCounts(t, fixture.pool, 1, 1, 1, 0)
}

func insertValidatedReplacementGapFixture(
	t *testing.T,
	fixture replacementTransitionFixture,
) StationGapOpening {
	t.Helper()
	gap, err := validateStationGapOpening(fixture.gap)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if err := requireRunningStationAttemptTx(
		t.Context(), tx, fixture.authority,
	); err != nil {
		t.Fatal(err)
	}
	if err := insertStationGapOpeningTx(t.Context(), tx, &gap); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return gap
}

func TestPostgresReplacementProductionOpeningSerializesOneOriginReceipt(t *testing.T) {
	fixture := newReplacementTransitionFixture(t, "replacement-concurrent-opening")
	selection := llm.ProviderIdentitySelection{
		Model: fixture.originModel, NativeContextLimit: fixture.gap.ContextTokens,
	}
	type result struct {
		opening StationGapDiscoveryOpening
		err     error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-start
			opening, err := fixture.repository.OpenStationGapDiscovery(
				t.Context(), StationGapDiscoveryOpenRecord{
					Gap: fixture.gap, Selection: selection,
				},
			)
			results <- result{opening: opening, err: err}
		}()
	}
	close(start)

	var successes int
	var failure error
	for range 2 {
		result := <-results
		if result.err != nil {
			failure = result.err
			continue
		}
		if result.opening.Gap.ID < 1 ||
			result.opening.Discovery.GapOpeningID != result.opening.Gap.ID {
			t.Fatalf("concurrent replacement opening=%+v", result.opening)
		}
		successes++
	}
	if successes != 1 || failure == nil || strings.TrimSpace(failure.Error()) == "" {
		t.Fatalf("concurrent replacement results: successes=%d error=%v", successes, failure)
	}
	assertReplacementTransitionCounts(t, fixture.pool, 1, 1, 0, 0)
}

type replacementTransitionFixture struct {
	repository  *Repository
	pool        *pgxpool.Pool
	authority   model.StepAttemptAuthority
	gap         StationGapOpenRecord
	originModel string
}

func newReplacementTransitionFixture(
	t *testing.T,
	marker string,
) replacementTransitionFixture {
	t.Helper()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	claim := claimStationTestJob(t, repository, marker)
	original := assemblyline.FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func Compute() int",
		Behavior: "Return one bounded computed value.",
	}
	originJob, err := assemblyline.NewFragmentGenerationJob(original)
	if err != nil {
		t.Fatal(err)
	}
	originGap, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: originJob, Station: station.CodingFragment,
		ContextTokens: 32768, MaxOutputTokens: 32768,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationOutputProjectionTestPrepared(t, originGap)
	discovery := persistStationDiscoverySuccess(
		t, repository, claim.Authority, originGap, prepared,
	)
	call, err := repository.OpenStationCall(t.Context(), StationCallOpenRecord{
		Authority: claim.Authority, Gap: originGap, Discovery: discovery, Prepared: prepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := repository.RecordStationCallReceiptAndEvidence(
		t.Context(), StationCallReceiptEvidenceRecord{
			Receipt: StationCallReceiptRecord{
				Authority: claim.Authority, OpeningID: call.ID, GapID: originGap.GapID,
				Result: stationCallOutputLimitWithContent(t, prepared, call, "partial source"),
				Error:  "exact station provider call ended with done_reason=length",
			},
			RequestedModel: prepared.ContextModel, EvidenceAttempt: 1, LatencyMS: 1,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: originGap.ID, GapID: originGap.GapID,
		Status: StationGapFailed,
		Error:  "exact station provider call ended with done_reason=length",
	}); err != nil {
		t.Fatal(err)
	}
	replacement, err := assemblyline.NewFragmentGenerationReplacementJob(
		assemblyline.FragmentGenerationReplacementInput{Original: original},
	)
	if err != nil {
		t.Fatal(err)
	}
	return replacementTransitionFixture{
		repository: repository, pool: pool, authority: claim.Authority,
		originModel: prepared.ContextModel,
		gap: StationGapOpenRecord{
			Authority: claim.Authority, Job: replacement, Station: station.CodingFragment,
			ContextTokens: 32768, MaxOutputTokens: 32768,
			OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
			ReplacementOrigin: &StationGapReplacementOrigin{
				GapOpeningID: originGap.ID, CallReceiptID: receipt.Receipt.ID,
			},
		},
	}
}

func replacementTransitionPrepared(
	t *testing.T,
	gap StationGapOpening,
	modelName string,
) llm.PreparedModel {
	t.Helper()
	prepared := stationOutputProjectionTestPrepared(t, gap)
	expectation := *prepared.ProviderIdentityExpectation
	expectation.Model = modelName
	prepared.BaseModel, prepared.ContextModel = modelName, modelName
	prepared.ProviderIdentityExpectation = &expectation
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(
		"replacement-transition-test", expectation,
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared.ProviderObservationChallenge = challenge
	prepared.RawTextStopSequence, err = ExpectedStationCallStopSequence(gap, expectation)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func assertReplacementTransitionCounts(
	t *testing.T,
	pool *pgxpool.Pool,
	wantGaps, wantDiscoveries, wantReceipts, wantCalls int,
) {
	t.Helper()
	var gaps, discoveries, receipts, calls int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(DISTINCT gap.id),COUNT(DISTINCT discovery.id),
		       COUNT(DISTINCT receipt.id),COUNT(DISTINCT call.id)
		FROM station_gap_openings AS gap
		LEFT JOIN station_provider_discoveries AS discovery
		  ON discovery.gap_opening_id=gap.id
		LEFT JOIN station_provider_discovery_receipts AS receipt
		  ON receipt.opening_id=discovery.id
		LEFT JOIN station_call_openings AS call ON call.gap_opening_id=gap.id
		WHERE gap.work_kind='fragment_generation_replacement'
	`).Scan(&gaps, &discoveries, &receipts, &calls); err != nil {
		t.Fatal(err)
	}
	if gaps != wantGaps || discoveries != wantDiscoveries ||
		receipts != wantReceipts || calls != wantCalls {
		t.Fatalf(
			"replacement transition counts=%d/%d/%d/%d want=%d/%d/%d/%d",
			gaps, discoveries, receipts, calls,
			wantGaps, wantDiscoveries, wantReceipts, wantCalls,
		)
	}
}
