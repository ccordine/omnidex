package queue

import (
	"maps"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

const (
	stationGapOpeningPortableEnvelopePriorConstraint = "station_gap_openings_portable_envelope_authority"
	stationGapOpeningPortableEnvelopeV2Constraint    = "station_gap_openings_portable_envelope_v2"
	stationGapOpeningPortableJobIdentityConstraint   = "station_gap_openings_portable_job_identity"
	stationGapOpeningPortableEnvelopePriorSHA256     = "c409ff59831ba5afce4ab802bd8e1c695c3f01db117bf04456841c007cd60c63"
	stationGapOpeningPortableJobIdentitySHA256       = "d50107974696b6779d265ef6b03910ce85ccfb0ce84097f88b794e704179b1d3"
)

func TestPostgresStationGapOpeningPortableEnvelopeV2IsExactAndPreservesAuthority(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "166"),
	); err != nil {
		t.Fatal(err)
	}
	preserved := stationGapOpeningConstraintCatalog(
		t, pool, stationGapOpeningPortableEnvelopePriorConstraint,
	)

	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "167"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, stationGapOpeningPortableEnvelopeV2Migration, 1)
	if after := stationGapOpeningConstraintCatalog(
		t, pool, stationGapOpeningPortableEnvelopeV2Constraint,
	); !maps.Equal(preserved, after) {
		t.Fatalf("unrelated station-gap constraints changed: before=%v after=%v", preserved, after)
	}
	want := map[string]string{
		stationGapOpeningPortableEnvelopeV2Constraint:  stationGapOpeningPortableEnvelopeV2ConstraintSHA256,
		stationGapOpeningPortableJobIdentityConstraint: stationGapOpeningPortableJobIdentitySHA256,
	}
	if got := stationGapOpeningConstraintHashes(t, pool, stationGapOpeningMapKeys(want)); !maps.Equal(got, want) {
		t.Fatalf("station-gap V2 constraints=%v want %v", got, want)
	}
	if stationGapOpeningConstraintExists(
		t, pool, stationGapOpeningPortableEnvelopePriorConstraint,
	) {
		t.Fatal("prior station-gap envelope constraint remains after V2 migration")
	}

	if _, err := pool.Exec(t.Context(), `
		CREATE TEMP TABLE station_gap_opening_envelope_v2_probe
		(LIKE station_gap_openings INCLUDING ALL)
	`); err != nil {
		t.Fatal(err)
	}
	for index, projected := range []bool{false, true} {
		opening := stationGapOpeningEnvelopeV2Probe(t, index+1, projected)
		if err := insertStationGapOpeningEnvelopeV2Probe(t, pool, opening); err != nil {
			t.Fatalf("insert valid station-gap envelope projected=%t: %v", projected, err)
		}
	}
	for index, field := range []string{"schema", "id", "kind"} {
		opening := stationGapOpeningEnvelopeV2Probe(t, index+10, false)
		opening = stationGapOpeningEnvelopeV2NullField(t, opening, field)
		requireStationGapOpeningEnvelopeCheckViolation(
			t, insertStationGapOpeningEnvelopeV2Probe(t, pool, opening),
		)
	}
}

func TestPostgresStationGapOpeningPortableEnvelopeV2RejectsChangedPriorAtomically(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "166"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE station_gap_openings
		DROP CONSTRAINT station_gap_openings_portable_envelope_authority,
		ADD CONSTRAINT station_gap_openings_portable_envelope_authority CHECK (TRUE)
	`); err != nil {
		t.Fatal(err)
	}
	before := stationGapOpeningConstraintCatalog(t, pool)
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "167"))
	if err == nil || !strings.Contains(
		err.Error(), "requires the exact prior portable envelope and job identity authority",
	) {
		t.Fatalf("changed prior station-gap envelope authority error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, stationGapOpeningPortableEnvelopeV2Migration, 0)
	if after := stationGapOpeningConstraintCatalog(t, pool); !maps.Equal(before, after) {
		t.Fatalf("changed-prior rejection mutated constraints: before=%v after=%v", before, after)
	}
	if stationGapOpeningConstraintExists(
		t, pool, stationGapOpeningPortableEnvelopeV2Constraint,
	) {
		t.Fatal("changed-prior rejection installed the V2 envelope constraint")
	}
}

func TestPostgresStationGapOpeningPortableEnvelopeV2RejectsMalformedExistingRowAtomically(
	t *testing.T,
) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "166"),
	); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		t.Context(), "station-gap-envelope-v2-malformed", model.PipelineCoding, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "station-gap-envelope-v2-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("malformed-row claim=%+v err=%v", claim, err)
	}
	opening, err := validateStationGapOpening(stationGapOpenFixture(t, claim.Authority))
	if err != nil {
		t.Fatal(err)
	}
	opening = freezeHistoricalRawStationGapV4(t, opening)
	opening = stationGapOpeningEnvelopeV2NullField(t, opening, "schema")
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := insertHistoricalStationGapOpeningTx(t.Context(), tx, &opening); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatalf("insert historically accepted malformed envelope: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	before := stationGapOpeningConstraintCatalog(t, pool)

	err = repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "167"))
	if err == nil || !strings.Contains(
		err.Error(), "rejects malformed existing envelope authority",
	) {
		t.Fatalf("malformed existing station-gap envelope error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, stationGapOpeningPortableEnvelopeV2Migration, 0)
	if after := stationGapOpeningConstraintCatalog(t, pool); !maps.Equal(before, after) {
		t.Fatalf("malformed-row rejection mutated constraints: before=%v after=%v", before, after)
	}
	var rowCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM station_gap_openings WHERE id=$1
	`, opening.ID).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 1 || stationGapOpeningConstraintExists(
		t, pool, stationGapOpeningPortableEnvelopeV2Constraint,
	) {
		t.Fatalf("malformed-row rejection rows/V2 constraint=%d/%t want 1/false",
			rowCount, stationGapOpeningConstraintExists(
				t, pool, stationGapOpeningPortableEnvelopeV2Constraint,
			))
	}
}
