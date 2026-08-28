package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

func TestPostgresWebClaimEvidenceReviewStationIsConsumedByExactGapAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	claim := claimStationTestJob(t, repository, "review-station")
	job, err := assemblyline.NewWebReviewClaimCoverageJob(assemblyline.WebReviewClaimLeafInput{
		ExactQuestion:  "Which release is current?",
		Context:        assemblyline.ObjectiveContext{},
		ParagraphText:  "Version 2 is current.",
		AcceptedClaims: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	const contextTokens = 8192
	opening, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: job, Station: station.WebClaimEvidenceReview,
		ContextTokens: contextTokens,
		MaxOutputTokens: portableStationTestMaxOutputTokens(
			t, job, contextTokens,
		),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
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
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	claim := claimStationTestJob(t, repository, "correction-station")
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
	const contextTokens = 8192
	opening, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: claim.Authority, Job: job, Station: station.WebGroundedSynthesisCorrection,
		ContextTokens: contextTokens,
		MaxOutputTokens: portableStationTestMaxOutputTokens(
			t, job, contextTokens,
		),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
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
