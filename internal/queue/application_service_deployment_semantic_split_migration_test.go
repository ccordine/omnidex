package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/scrum"
)

const applicationServiceDeploymentSemanticSplitMigration = "160_application_service_deployment_semantic_split.sql"

func TestApplicationServiceDeploymentSemanticSplitMigrationIsFailClosed(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + applicationServiceDeploymentSemanticSplitMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings, station_gap_outcomes, jobs, generated_workload_deployments IN ACCESS EXCLUSIVE MODE",
		"cannot split service deployment semantics while a coding job is active",
		"cannot split service deployment semantics while a deployment is nonterminal",
		"WHEN 'application_service_continued_availability' THEN station='coding_service_continued_availability'",
		"WHEN 'application_service_persistence_destination' THEN station='coding_service_persistence_destination'",
		"WHEN 'application_service_deployment_intent' THEN station='coding_service_deployment_intent'",
		"d373f163c91642a2d99917e9f82d54575fa3770618ac3d24a54a1d97abf91c8d",
		"5ee88ea6498bba2a89b1339a1f259d71dace780bcd9ae2ff89a66039900df1e7",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("service deployment semantic split migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"UPDATE ", "DELETE ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("service deployment semantic split migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationServiceDeploymentSemanticSplitRejectsActiveCodingAndScrumJobs(t *testing.T) {
	for _, pipeline := range []string{model.PipelineCoding, model.PipelineScrum} {
		t.Run(pipeline, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "159"),
			); err != nil {
				t.Fatal(err)
			}
			if pipeline == model.PipelineCoding {
				if _, err := repository.EnqueueJob(
					t.Context(), "active-"+pipeline, model.PipelineCoding, nil,
				); err != nil {
					t.Fatal(err)
				}
			} else {
				project, err := repository.CreateProject(
					t.Context(), "active-scrum", t.TempDir(), "",
				)
				if err != nil {
					t.Fatal(err)
				}
				card, err := repository.CreateScrumCard(
					t.Context(), project.ID, "", "Active Scrum", "", "assigned", nil, nil,
				)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := repository.EnqueueScrumJob(
					t.Context(), "active-scrum", scrum.JobMetadata{
						Source: scrum.JobMetadataSource, ProjectID: project.ID, CardID: card.ID,
						CardTitle: card.Title, CardDescription: card.Description,
						ReturnColumn: card.Column, ModelConfig: modelconfig.Config{},
					},
				); err != nil {
					t.Fatal(err)
				}
			}
			err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "160"),
			)
			if err == nil || !strings.Contains(
				err.Error(), "while a coding job is active",
			) {
				t.Fatalf("active %s migration error=%v", pipeline, err)
			}
		})
	}
}

func TestApplicationServiceDeploymentSemanticSplitMigrationOwnsNewAndRetiresOldWork(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "160")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationServiceDeploymentSemanticSplitMigration, 1)

	var availability, destination, cross, historical bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_service_continued_availability',
				'application_service_continued_availability','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_persistence_destination',
				'application_service_persistence_destination','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_persistence_destination',
				'application_service_continued_availability','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_deployment_intent',
				'application_service_deployment_intent','{}'::jsonb
			)
	`).Scan(&availability, &destination, &cross, &historical); err != nil {
		t.Fatal(err)
	}
	if !availability || !destination || cross || !historical {
		t.Fatalf(
			"deployment semantic ownership availability/destination/cross/historical=%v/%v/%v/%v",
			availability, destination, cross, historical,
		)
	}

	for name, statement := range map[string]string{
		"direct": `
			INSERT INTO station_gap_openings(work_kind,portable_payload)
			VALUES('application_service_deployment_intent','{}')
		`,
		"correction": `
			INSERT INTO station_gap_openings(work_kind,portable_payload)
			VALUES('response_correction',
			'{"original":{"kind":"application_service_deployment_intent","payload":{}}}')
		`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := pool.Exec(t.Context(), statement); err == nil ||
				!strings.Contains(err.Error(), "retired station work kind") {
				t.Fatalf("retired %s opening error=%v", name, err)
			}
		})
	}
}
