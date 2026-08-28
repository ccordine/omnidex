package queue

import (
	"os"
	"strings"
	"testing"
)

const jobExecutionIdentityImmutabilityMigration = "169_job_execution_identity_immutability.sql"

const (
	jobPipelineAuthorityPriorSHA256 = "195894f09717e1f965ca49236384c539d8fc4cf1661c1226b198f6946f108e15"
	jobPipelineAuthoritySHA256      = "ad046f5dd4f3059740425b6c2af74ee061cf1715d78ac31d5d0a19279fc7844e"
	jobStepAuthorityPriorSHA256     = "be196b60c8e6f01100ab1b01af5dc040398d3b0ceb90df2d6355bc65b714b14e"
	jobStepAuthoritySHA256          = "04423dc23b8cbeb03a0cbe0d28e1e4f805a86d61a2c1a613d3275bdf71a21bad"
)

func TestJobExecutionIdentityImmutabilityMigrationIsExact(t *testing.T) {
	t.Parallel()
	jobsPrior, err := os.ReadFile("../../migrations/087_executable_pipeline_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	stepsPrior, err := os.ReadFile("../../migrations/028_job_generations.sql")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../migrations/" + jobExecutionIdentityImmutabilityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE jobs, job_steps, workspace_mutation_operations IN ACCESS EXCLUSIVE MODE",
		jobPipelineAuthorityPriorSHA256,
		jobPipelineAuthoritySHA256,
		jobStepAuthorityPriorSHA256,
		jobStepAuthoritySHA256,
		"job pipeline identity is immutable",
		"job step action identity is immutable",
		"job execution identity found stale workspace mutation",
		"job execution identity immutability postcondition failed",
		"COMMIT;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("job execution identity migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"DROP FUNCTION", "DROP TRIGGER", "CREATE TRIGGER", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("job execution identity migration contains forbidden %q", forbidden)
		}
	}

	priorJobsBody := workspaceMutationAuthorityBody(
		t, string(jobsPrior), "CREATE FUNCTION enforce_jobs_executable_pipeline_authority()",
		"$$ LANGUAGE plpgsql;",
	)
	currentJobsBody := workspaceMutationAuthorityBody(
		t, string(raw), "CREATE OR REPLACE FUNCTION enforce_jobs_executable_pipeline_authority()",
		"$$ LANGUAGE plpgsql;",
	)
	jobsMarker := "    IF TG_OP='INSERT' AND NEW.pipeline NOT IN ('chat','coding','scrum') THEN\n" +
		"        RAISE EXCEPTION 'new job pipeline % is retired or unregistered', NEW.pipeline;\n" +
		"    END IF;\n"
	jobsReplacement := jobsMarker +
		"    IF TG_OP='UPDATE' AND OLD.pipeline IS DISTINCT FROM NEW.pipeline THEN\n" +
		"        RAISE EXCEPTION 'job pipeline identity is immutable';\n" +
		"    END IF;\n"
	if strings.Count(priorJobsBody, jobsMarker) != 1 ||
		currentJobsBody != strings.Replace(priorJobsBody, jobsMarker, jobsReplacement, 1) {
		t.Fatal("migration 169 changed job pipeline guard beyond identity immutability")
	}

	priorStepsBody := workspaceMutationAuthorityBody(
		t, string(stepsPrior),
		"CREATE OR REPLACE FUNCTION prevent_job_step_generation_identity_mutation()",
		"$$ LANGUAGE plpgsql;",
	)
	currentStepsBody := workspaceMutationAuthorityBody(
		t, string(raw),
		"CREATE OR REPLACE FUNCTION prevent_job_step_generation_identity_mutation()",
		"$$ LANGUAGE plpgsql;",
	)
	stepsMarker := "    IF OLD.status <> 'pending' AND NEW.status = 'pending' THEN\n"
	stepsReplacement := "    IF OLD.action IS DISTINCT FROM NEW.action THEN\n" +
		"        RAISE EXCEPTION 'job step action identity is immutable';\n" +
		"    END IF;\n" + stepsMarker
	if strings.Count(priorStepsBody, stepsMarker) != 1 ||
		currentStepsBody != strings.Replace(priorStepsBody, stepsMarker, stepsReplacement, 1) {
		t.Fatal("migration 169 changed job step guard beyond action identity immutability")
	}
}
