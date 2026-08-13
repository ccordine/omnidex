package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
)

func TestPostgresWebClaimEvidenceReviewStationIsConsumedByExactGapAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "070")); err != nil {
		t.Fatal(err)
	}
	jobRecord, err := repository.EnqueueJob(t.Context(), "review station", model.PipelineCoding, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "review-station-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != jobRecord.ID {
		t.Fatalf("claim=%+v want job %d", claim, jobRecord.ID)
	}
	job, err := assemblyline.NewWebClaimEvidenceReviewJob(assemblyline.WebClaimEvidenceReviewInput{
		ExactQuestion: "Which release is current?",
		Paragraph: assemblyline.WebReviewParagraph{
			ParagraphID: "P1", Text: "Version 2 is current.", EvidenceIDs: []string{"E31"},
		},
		Evidence: []assemblyline.WebReviewEvidence{{EvidenceID: "E31", Content: "Version 2 is current."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	opening, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: job, Station: station.WebClaimEvidenceReview,
		ContextTokens: 8192, MaxOutputTokens: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opening.Station != station.WebClaimEvidenceReview || opening.WorkKind != string(job.Kind) {
		t.Fatalf("opening=%+v", opening)
	}
}

func TestPostgresWebSynthesisCorrectionStationIsConsumedByExactGapAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "070")); err != nil {
		t.Fatal(err)
	}
	jobRecord, err := repository.EnqueueJob(t.Context(), "correction station", model.PipelineCoding, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "correction-station-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != jobRecord.ID {
		t.Fatalf("claim=%+v want job %d", claim, jobRecord.ID)
	}
	job, err := assemblyline.NewWebGroundedSynthesisCorrectionJob(assemblyline.WebGroundedSynthesisCorrectionInput{
		ExactQuestion: "Which release is current?",
		Paragraphs: []assemblyline.WebReviewParagraph{{
			ParagraphID: "P1", Text: "Version 3 is current.", EvidenceIDs: []string{"E31"},
		}},
		Issue: assemblyline.WebClaimEvidenceReviewDecision{
			Schema:      assemblyline.WebClaimEvidenceReviewSchemaV1,
			Outcome:     assemblyline.WebClaimEvidenceReviewIssue,
			ParagraphID: "P1", EvidenceIDs: []string{"E31"},
			IssueKind: assemblyline.WebClaimEvidenceContradictedSupport,
			Detail:    "The evidence says version 2, not version 3.",
		},
		Evidence:          []assemblyline.WebGroundedEvidence{{EvidenceID: "E31", Content: "Version 2 is current."}},
		MaxParagraphBytes: 512,
	})
	if err != nil {
		t.Fatal(err)
	}
	opening, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: job, Station: station.WebGroundedSynthesisCorrection,
		ContextTokens: 8192, MaxOutputTokens: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if opening.Station != station.WebGroundedSynthesisCorrection || opening.WorkKind != string(job.Kind) {
		t.Fatalf("opening=%+v", opening)
	}
}

func TestPostgresWebClaimEvidenceReviewMigrationRejectsChangedPriorFunction(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "069")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION station_owns_portable_work(station TEXT, work_kind TEXT, payload JSONB)
		RETURNS BOOLEAN AS 'SELECT FALSE' LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "070"))
	if err == nil || !strings.Contains(err.Error(), "prior station function hash") {
		t.Fatalf("migration error=%v", err)
	}
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename='070_web_claim_evidence_review_station.sql')
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected migration wrote its ledger entry")
	}
}
