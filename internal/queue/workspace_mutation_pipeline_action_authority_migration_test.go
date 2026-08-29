package queue

import (
	"os"
	"strings"
	"testing"
)

const workspaceMutationPipelineActionAuthorityMigration = "168_workspace_mutation_pipeline_action_authority.sql"

const (
	workspaceMutationInsertPriorSourceSHA256 = "9f2b7387e90d9021089532021ab55c09d7177b81a3fd7bdd2358e362e2930294"
	workspaceMutationInsertSourceSHA256      = "39c9598b0ecaad64aebd36391e71ba18406a23d75dc0c33606654789b2c15d2b"
	workspaceMutationCurrentPriorSHA256      = "fc8209a7ce7b843458f1e3f2104a5de50ab8167fa104c4a106b6f6c4a8a67c53"
	workspaceMutationCurrentSourceSHA256     = "8312f18e7a7d2265bcf38f0103bfcd0d02fc8845a6edb197a7211830422d7a79"
)

func TestWorkspaceMutationPipelineActionAuthorityMigrationIsExact(t *testing.T) {
	t.Parallel()
	priorRaw, err := os.ReadFile("../../migrations/159_workspace_mutation_journal_cutover.sql")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../migrations/" + workspaceMutationPipelineActionAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE jobs, job_steps, workspace_mutation_operations IN ACCESS EXCLUSIVE MODE",
		workspaceMutationInsertPriorSourceSHA256,
		workspaceMutationInsertSourceSHA256,
		workspaceMutationCurrentPriorSHA256,
		workspaceMutationCurrentSourceSHA256,
		"trigger_row.tgname='workspace_mutation_insert_validate'",
		"trigger_row.tgenabled='O'",
		"trigger_row.tgtype=7",
		"trigger_row.tgconstraint=0",
		"trigger_row.tgnargs=0",
		"procedure.prokind='f'",
		"procedure.pronargs=0",
		"procedure.prorettype='trigger'::regtype",
		"language.lanname='plpgsql'",
		"procedure.provolatile='v'",
		"procedure.proparallel='u'",
		"(jobs.pipeline='chat' AND steps.action='objective_resolve')",
		"(jobs.pipeline='coding' AND steps.action='v3_coding')",
		"(jobs.pipeline='scrum' AND steps.action='v3_coding')",
		"requires the exact prior insert guard",
		"requires the exact prior transition guard",
		"has stale pipeline/action authority",
		"pipeline/action authority postcondition failed",
		"transition pipeline/action authority postcondition failed",
		"COMMIT;",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("workspace mutation pipeline/action migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"DROP FUNCTION", "DROP TRIGGER", "CREATE TRIGGER", "UPDATE ", "DELETE ",
		"jobs.pipeline IN", "fallback",
	} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("workspace mutation pipeline/action migration contains forbidden %q", forbidden)
		}
	}

	priorBody := workspaceMutationAuthorityBody(
		t, string(priorRaw), "CREATE FUNCTION validate_workspace_mutation_insert()",
		"$$ LANGUAGE plpgsql;",
	)
	currentBody := workspaceMutationAuthorityBody(
		t, string(raw), "CREATE OR REPLACE FUNCTION validate_workspace_mutation_insert()",
		"$$ LANGUAGE plpgsql;",
	)
	priorPredicate := "    SELECT jobs.status='running' AND jobs.pipeline='coding' AND\n"
	currentPredicate := "    SELECT jobs.status='running' AND (\n" +
		"               (jobs.pipeline='chat' AND steps.action='objective_resolve') OR\n" +
		"               (jobs.pipeline='coding' AND steps.action='v3_coding') OR\n" +
		"               (jobs.pipeline='scrum' AND steps.action='v3_coding')\n" +
		"           ) AND\n"
	if strings.Count(priorBody, priorPredicate) != 1 {
		t.Fatal("migration 159 insert guard has unexpected pipeline authority")
	}
	wantBody := strings.Replace(priorBody, priorPredicate, currentPredicate, 1)
	if currentBody != wantBody {
		t.Fatal("migration 168 changed insert-guard authority beyond the exact pipeline/action predicate")
	}

	priorCurrentBody := workspaceMutationAuthorityBody(
		t, string(priorRaw),
		"CREATE FUNCTION workspace_mutation_current_authority_valid(operation workspace_mutation_operations)",
		"$$ LANGUAGE SQL;",
	)
	currentCurrentBody := workspaceMutationAuthorityBody(
		t, string(raw),
		"CREATE OR REPLACE FUNCTION workspace_mutation_current_authority_valid(\n    operation workspace_mutation_operations\n)",
		"$$ LANGUAGE SQL;",
	)
	priorCurrentPredicate := "               jobs.project_id=operation.project_id AND\n" +
		"               steps.status='running'"
	currentCurrentPredicate := "               jobs.project_id=operation.project_id AND (\n" +
		"                   (jobs.pipeline='chat' AND steps.action='objective_resolve') OR\n" +
		"                   (jobs.pipeline='coding' AND steps.action='v3_coding') OR\n" +
		"                   (jobs.pipeline='scrum' AND steps.action='v3_coding')\n" +
		"               ) AND\n" +
		"               steps.status='running'"
	if strings.Count(priorCurrentBody, priorCurrentPredicate) != 1 {
		t.Fatal("migration 159 transition guard has unexpected authority")
	}
	wantCurrentBody := strings.Replace(
		priorCurrentBody, priorCurrentPredicate, currentCurrentPredicate, 1,
	)
	if currentCurrentBody != wantCurrentBody {
		t.Fatal("migration 168 changed transition authority beyond the exact pipeline/action predicate")
	}
}

func workspaceMutationAuthorityBody(
	t *testing.T,
	source string,
	declaration string,
	terminator string,
) string {
	t.Helper()
	startMarker := declaration + "\nRETURNS TRIGGER AS $$"
	if strings.HasSuffix(declaration, ")") && strings.Contains(declaration, "current_authority_valid") {
		startMarker = declaration + "\nRETURNS BOOLEAN AS $$"
	}
	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("migration omitted %q", declaration)
	}
	start += len(startMarker)
	end := strings.Index(source[start:], terminator)
	if end < 0 {
		t.Fatalf("migration %q has no function-body terminator", declaration)
	}
	return source[start : start+end]
}
