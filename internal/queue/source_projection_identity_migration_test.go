package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

const sourceProjectionIdentityMigration = "162_source_projection_identity.sql"

func TestSourceProjectionIdentityMigrationIsConditionalAndRejectsUnboundNewWork(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + sourceProjectionIdentityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"station_gap_openings_check5",
		"station_gap_openings_check6",
		"65d7ff6cd58426c491196ef386f0414f9eb58abaee0bcb6d513b95d61accc4be",
		"bced50709cf6a0558483df99eff168e5348871e912db70b59b2e64449a3964cd",
		"portable_envelope::jsonb ? 'source_projection'",
		"'go','javascript','java','rust','php'",
		"station_gap_openings_source_projection_authority",
		"NOT VALID",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("source projection identity migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_gap_openings",
		"DELETE FROM station_gap_openings",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("source projection identity migration contains %q", forbidden)
		}
	}
}

func TestPostgresSourceProjectionIdentityPersistsBoundDecoderAndRejectsOldNewPath(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "161"),
	); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		t.Context(), "source-projection-identity", model.PipelineCoding, []byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "source-projection-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v, want job %d", claim, job.ID)
	}

	projected := sourceProjectionCorrectionJob(t)
	if _, err := repository.OpenStationGap(
		t.Context(), sourceProjectionGapRecord(claim.Authority, projected),
	); err == nil {
		t.Fatal("pre-cutover database accepted a projection-bound work identity")
	}
	historical := withoutSourceProjection(projected)
	historicalOpening, err := repository.OpenStationGap(
		t.Context(), sourceProjectionGapRecord(claim.Authority, historical),
	)
	if err != nil {
		t.Fatalf("open historical unbound correction before cutover: %v", err)
	}
	if historicalOpening.WorkID != historical.ID ||
		strings.Contains(historicalOpening.PortableEnvelope, `"source_projection"`) {
		t.Fatalf("historical opening=%+v", historicalOpening)
	}
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "162"),
	); err != nil {
		t.Fatal(err)
	}
	opened, err := repository.OpenStationGap(
		t.Context(), sourceProjectionGapRecord(claim.Authority, projected),
	)
	if err != nil {
		t.Fatal(err)
	}
	if opened.WorkID != projected.ID ||
		!strings.Contains(opened.PortableEnvelope, `"source_projection":"go"`) {
		t.Fatalf("opening=%+v", opened)
	}

	unbound := withoutSourceProjection(sourceProjectionCorrectionJobForCurrent(
		t, "func OtherValue() int { return missing() }",
	))
	if err := unbound.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.OpenStationGap(
		t.Context(), sourceProjectionGapRecord(claim.Authority, unbound),
	); err == nil || !strings.Contains(err.Error(), "source_projection_authority") {
		t.Fatalf("unbound new correction error=%v", err)
	}

	var historicalCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM station_gap_openings
		WHERE work_id=$1 AND NOT (portable_envelope::jsonb ? 'source_projection')
	`, historical.ID).Scan(&historicalCount); err != nil {
		t.Fatal(err)
	}
	if historicalCount != 1 {
		t.Fatalf("historical unbound correction count=%d want 1", historicalCount)
	}

	var validated bool
	if err := pool.QueryRow(t.Context(), `
		SELECT convalidated FROM pg_constraint
		WHERE conrelid='station_gap_openings'::regclass AND
		      conname='station_gap_openings_source_projection_authority'
	`).Scan(&validated); err != nil {
		t.Fatal(err)
	}
	if validated {
		t.Fatal("historical source-projection cutover unexpectedly rewrote or validated old rows")
	}
}

func withoutSourceProjection(job assemblyline.PortableJob) assemblyline.PortableJob {
	job.SourceProjection = ""
	digest := sha256.New()
	_, _ = digest.Write([]byte(job.Schema))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(job.Kind))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(job.Payload)
	job.ID = hex.EncodeToString(digest.Sum(nil))
	return job
}

func sourceProjectionCorrectionJob(t *testing.T) assemblyline.PortableJob {
	return sourceProjectionCorrectionJobForCurrent(
		t, "func Value() int { return missing() }",
	)
}

func sourceProjectionCorrectionJobForCurrent(
	t *testing.T,
	current string,
) assemblyline.PortableJob {
	t.Helper()
	job, err := assemblyline.NewSourceProjectedFragmentCorrectionJob(
		assemblyline.FragmentCorrectionInput{
			CurrentDeclaration: current,
			RepairGuidance:     "Replace the missing call with a local expression.",
		},
		"go",
	)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func sourceProjectionGapRecord(
	authority model.StepAttemptAuthority,
	job assemblyline.PortableJob,
) StationGapOpenRecord {
	return StationGapOpenRecord{
		Authority: authority, Job: job, Station: station.CodingFragmentCorrection,
		ContextTokens: 8192, MaxOutputTokens: 8192,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
}
