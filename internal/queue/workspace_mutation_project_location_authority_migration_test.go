package queue

import (
	"os"
	"strings"
	"testing"
)

const workspaceMutationProjectLocationAuthorityMigration = "179_workspace_mutation_project_location_authority.sql"

func TestWorkspaceMutationProjectLocationAuthorityMigrationIsExact(t *testing.T) {
	t.Parallel()
	priorRaw, err := os.ReadFile("../../migrations/168_workspace_mutation_pipeline_action_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../migrations/" + workspaceMutationProjectLocationAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	compact := strings.Join(strings.Fields(source), " ")
	for _, required := range []string{
		"LOCK TABLE projects, jobs, job_steps, job_step_attempts, workspace_mutation_operations IN ACCESS EXCLUSIVE MODE",
		workspaceMutationInsertSourceSHA256,
		workspaceMutationCurrentSourceSHA256,
		"3f2b6f2d24f2ce88260f46d7cc2c27377d4c8d2150d78dc22751569608b92d5b",
		workspaceMutationProjectLocationInsertFunctionSHA256,
		workspaceMutationProjectLocationCurrentFunctionSHA256,
		workspaceMutationProjectLocationImmutableFunctionSHA256,
		workspaceMutationProjectLocationChangeGuardSHA256,
		"ADD COLUMN project_location TEXT",
		"DISABLE TRIGGER workspace_mutation_update_validate",
		"UPDATE workspace_mutation_operations AS operation SET project_location=operation.workspace_root",
		"ENABLE TRIGGER workspace_mutation_update_validate",
		"ALTER COLUMN project_location SET NOT NULL",
		"CREATE TRIGGER workspace_mutation_project_location_immutable BEFORE UPDATE ON workspace_mutation_operations",
		"CREATE TRIGGER projects_active_work_location_guard BEFORE UPDATE ON projects",
		"operation.status NOT IN ('verified','verification_failed')",
		"requires the exact prior insert guard",
		"requires exact prior transition guards",
		"found stale operation",
		"project-location authority postcondition failed",
		"COMMIT;",
	} {
		if !strings.Contains(compact, required) {
			t.Fatalf("workspace mutation project-location migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"DROP FUNCTION", "DROP TRIGGER", "DROP COLUMN", "DELETE FROM",
		"IF NOT EXISTS", "CASCADE", "fallback",
	} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("workspace mutation project-location migration contains forbidden %q", forbidden)
		}
	}
	if strings.Count(compact, "UPDATE workspace_mutation_operations AS operation SET project_location=operation.workspace_root") != 1 {
		t.Fatal("workspace mutation project-location migration must have one exact deterministic backfill")
	}
	for _, terminalAwareBinding := range []string{
		"operation.status NOT IN ('verified','verification_failed') AND projects.location IS DISTINCT FROM operation.workspace_root",
		"operation.status NOT IN ('verified','verification_failed') AND projects.location IS DISTINCT FROM operation.project_location",
	} {
		if strings.Count(compact, terminalAwareBinding) != 1 {
			t.Fatalf("workspace mutation project-location migration omitted terminal-aware binding %q", terminalAwareBinding)
		}
	}

	assertWorkspaceMutationProjectLocationInsertBody(t, string(priorRaw), source)
	assertWorkspaceMutationProjectLocationCurrentBody(t, string(priorRaw), source)
	assertWorkspaceMutationProjectLocationImmutableBody(t, source)
	assertProjectLocationActiveWorkGuardBody(t, source)
	assertWorkspaceMutationProjectLocationConstraint(t, source)
}

func assertWorkspaceMutationProjectLocationInsertBody(t *testing.T, prior, current string) {
	t.Helper()
	priorBody := workspaceMutationAuthorityBody(
		t, prior, "CREATE OR REPLACE FUNCTION validate_workspace_mutation_insert()",
		"$$ LANGUAGE plpgsql;",
	)
	currentBody := workspaceMutationAuthorityBody(
		t, current, "CREATE OR REPLACE FUNCTION validate_workspace_mutation_insert()",
		"$$ LANGUAGE plpgsql;",
	)
	rootBinding := "projects.location=NEW.workspace_root"
	locationBinding := "projects.location=NEW.project_location"
	rootFailure := "workspace mutation requires the exact current active step attempt and root"
	locationFailure := "workspace mutation requires the exact current active step attempt and project location"
	if strings.Count(priorBody, rootBinding) != 1 || strings.Count(priorBody, rootFailure) != 1 {
		t.Fatal("migration 168 insert guard has unexpected workspace-root authority")
	}
	want := strings.Replace(priorBody, rootBinding, locationBinding, 1)
	want = strings.Replace(want, rootFailure, locationFailure, 1)
	if currentBody != want {
		t.Fatal("migration 179 changed insert authority beyond project-location binding")
	}
}

func assertWorkspaceMutationProjectLocationCurrentBody(t *testing.T, prior, current string) {
	t.Helper()
	declaration := "CREATE OR REPLACE FUNCTION workspace_mutation_current_authority_valid(\n    operation workspace_mutation_operations\n)"
	priorBody := workspaceMutationAuthorityBody(t, prior, declaration, "$$ LANGUAGE SQL;")
	currentBody := workspaceMutationAuthorityBody(t, current, declaration, "$$ LANGUAGE SQL;")
	projectBinding := "               jobs.project_id=operation.project_id AND (\n"
	locationBinding := "               jobs.project_id=operation.project_id AND\n" +
		"               projects.location=operation.project_location AND (\n"
	projectJoin := "        FROM jobs\n        JOIN job_steps AS steps"
	locationJoin := "        FROM jobs\n        JOIN projects ON projects.id=jobs.project_id\n" +
		"        JOIN job_steps AS steps"
	if strings.Count(priorBody, projectBinding) != 1 || strings.Count(priorBody, projectJoin) != 1 {
		t.Fatal("migration 168 current guard has unexpected project authority")
	}
	want := strings.Replace(priorBody, projectBinding, locationBinding, 1)
	want = strings.Replace(want, projectJoin, locationJoin, 1)
	if currentBody != want {
		t.Fatal("migration 179 changed transition authority beyond project-location binding")
	}
}

func assertWorkspaceMutationProjectLocationImmutableBody(t *testing.T, source string) {
	t.Helper()
	body := workspaceMutationAuthorityBody(
		t, source, "CREATE FUNCTION prevent_workspace_mutation_project_location_change()",
		"$$ LANGUAGE plpgsql;",
	)
	want := `BEGIN
    IF OLD.project_location IS DISTINCT FROM NEW.project_location THEN
        RAISE EXCEPTION 'workspace mutation project location is immutable';
    END IF;
    RETURN NEW;
END;`
	if strings.TrimSpace(body) != want {
		t.Fatal("migration 179 project-location immutability function is not exact")
	}
}

func assertProjectLocationActiveWorkGuardBody(t *testing.T, source string) {
	t.Helper()
	body := workspaceMutationAuthorityBody(
		t, source, "CREATE FUNCTION prevent_project_location_change_during_active_work()",
		"$$ LANGUAGE plpgsql;",
	)
	for _, exact := range []string{
		"IF OLD.location IS NOT DISTINCT FROM NEW.location THEN",
		"WHERE project_id=OLD.id AND status NOT IN ('completed','failed','canceled')",
		"RAISE EXCEPTION 'project location cannot change while job % remains %'",
	} {
		if strings.Count(body, exact) != 1 {
			t.Fatalf("migration 179 active-work guard omitted exact %q", exact)
		}
	}
}

func assertWorkspaceMutationProjectLocationConstraint(t *testing.T, source string) {
	t.Helper()
	want := `ADD CONSTRAINT workspace_mutation_project_location_valid CHECK (
        project_location<>'' AND project_location=BTRIM(project_location) AND
        octet_length(project_location)<=4096 AND left(project_location,1)='/' AND
        project_location<>'/' AND right(project_location,1)<>'/' AND
        position(chr(92) IN project_location)=0 AND
        project_location !~ E'[\r\n]' AND position('//' IN project_location)=0 AND
        project_location !~ '(^|/)[.][.]?(/|$)'
    )`
	if strings.Count(source, want) != 1 {
		t.Fatal("migration 179 project-location constraint is not exact")
	}
}
