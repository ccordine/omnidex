package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/station"
)

func TestPostgresRepositoryGroundingStationsUseExactGapAuthority(t *testing.T) {
	tests := []struct {
		name    string
		station station.ID
		job     func(testing.TB) assemblyline.PortableJob
	}{
		{name: "relevance", station: station.RepositoryEvidenceRelevance, job: repositoryRelevancePortableJob},
		{name: "review", station: station.RepositoryGroundedReview, job: repositoryReviewPortableJob},
		{name: "correction", station: station.RepositoryGroundedCorrection, job: repositoryCorrectionPortableJob},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "096")); err != nil {
				t.Fatal(err)
			}
			claim := seedPreInlineExecutionMigrationClaim(
				t, t.Context(), pool, "repository-grounding",
			)
			job := test.job(t)
			if _, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
				Authority: claim.Authority, Job: job, Station: station.ConversationObjectiveKind,
				ContextTokens: 8192, MaxOutputTokens: 8192,
				OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
			}); err == nil {
				t.Fatalf("%s work opened under objective-kind station", test.name)
			}
			opening, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
				Authority: claim.Authority, Job: job, Station: test.station,
				ContextTokens: 8192, MaxOutputTokens: 8192,
				OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
			})
			if err != nil {
				t.Fatal(err)
			}
			if opening.Station != test.station || opening.WorkKind != string(job.Kind) {
				t.Fatalf("opening=%+v", opening)
			}
		})
	}
}

func TestPostgresRepositoryGroundingMigrationRejectsChangedPriorFunction(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "073")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION station_owns_portable_work(station TEXT, work_kind TEXT, payload JSONB)
		RETURNS BOOLEAN AS 'SELECT FALSE' LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "074"))
	if err == nil || !strings.Contains(err.Error(), "prior station function hash") {
		t.Fatalf("migration error=%v", err)
	}
	var installed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename='074_repository_grounding_stations.sql')
	`).Scan(&installed); err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("rejected migration wrote its ledger entry")
	}
}

func repositoryRelevancePortableJob(t testing.TB) assemblyline.PortableJob {
	t.Helper()
	job, err := assemblyline.NewRepositoryEvidenceRelevanceJob(assemblyline.RepositoryEvidenceRelevanceInput{
		ExactRequirement: "Which component owns dispatch?",
		Candidates:       []assemblyline.RepositoryEvidenceCandidate{{EvidenceID: "R01", Text: "DispatchOwner owns dispatch."}},
		MaxSelections:    1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func repositoryReviewPortableJob(t testing.TB) assemblyline.PortableJob {
	t.Helper()
	job, err := assemblyline.NewRepositoryGroundedReviewJob(assemblyline.RepositoryGroundedReviewInput{
		RequirementID: "requirement-1", ExactRequirement: "Which component owns dispatch?",
		AnswerText: "DispatchOwner owns dispatch.", EvidenceIDs: []string{"R01"},
		Evidence: []assemblyline.GroundedEvidenceCapsule{{ID: "R01", Text: "DispatchOwner owns dispatch."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func repositoryCorrectionPortableJob(t testing.TB) assemblyline.PortableJob {
	t.Helper()
	review := repositoryReviewPortableJob(t)
	var reviewInput assemblyline.RepositoryGroundedReviewInput
	if err := decodePortableGapPayload(review.Payload, &reviewInput); err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewRepositoryGroundedCorrectionJob(assemblyline.RepositoryGroundedCorrectionInput{
		RequirementID: reviewInput.RequirementID, ExactRequirement: reviewInput.ExactRequirement,
		CurrentText: reviewInput.AnswerText, EvidenceIDs: reviewInput.EvidenceIDs, Evidence: reviewInput.Evidence,
		Issue: assemblyline.RepositoryGroundedReviewDecision{
			Schema:    assemblyline.RepositoryGroundedReviewSchemaV1,
			Outcome:   assemblyline.RepositoryGroundedReviewIssue,
			IssueKind: assemblyline.RepositoryGroundedUnsupportedClaim,
			Detail:    "The ownership wording is unsupported.",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}
