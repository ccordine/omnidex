package database

import (
	"os"
	"strings"
	"testing"
)

func requireSchemaFragments(t *testing.T, source string, fragments ...string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(source, fragment) {
			t.Fatalf("agent execution schema lacks %q", fragment)
		}
	}
}

func schemaFunctionBody(t *testing.T, source, declaration string) string {
	t.Helper()
	start := strings.Index(source, declaration)
	if start < 0 {
		t.Fatalf("schema lacks function declaration %q", declaration)
	}
	bodyStart := strings.Index(source[start:], "    LANGUAGE plpgsql AS $")
	if bodyStart < 0 {
		t.Fatalf("schema function %q lacks a PL/pgSQL body", declaration)
	}
	bodyStart += start
	bodyEnd := strings.Index(source[bodyStart:], "$;")
	if bodyEnd < 0 {
		t.Fatalf("schema function %q lacks a body terminator", declaration)
	}
	return source[bodyStart : bodyStart+bodyEnd+3]
}

func TestAgentExecutionSchemaOwnsNormalizedLedgerAndLifecycle(t *testing.T) {
	setup := string(SetupSQL())
	requireSchemaFragments(t, setup,
		"CREATE TABLE coding_ledgers (",
		"CREATE TABLE coding_ledger_revisions (",
		"CREATE TABLE coding_ledger_items (",
		"CREATE TABLE coding_ledger_item_versions (",
		"CREATE TABLE coding_ledger_revision_items (",
		"CREATE TABLE coding_ledger_edges (",
		"CREATE TABLE coding_ledger_origins (",
		"CREATE TABLE coding_ledger_owner_turns (",
		"CREATE TABLE coding_ledger_owner_turn_dispositions (",
		"CREATE TABLE coding_ledger_questions (",
		"CREATE TABLE coding_ledger_findings (",
		"CREATE TABLE coding_ledger_routes (",
		"CREATE TABLE coding_ledger_checks (",
		"CREATE TABLE coding_agent_tasks (",
		"CREATE TABLE coding_agent_attempts (",
		"CREATE TABLE coding_agent_interruptions (",
		"CREATE TABLE coding_agent_grants (",
		"CREATE TABLE coding_agent_effect_manifests (",
		"CREATE TABLE coding_agent_effects (",
		"CREATE TABLE coding_agent_check_results (",
	)
	for _, retiredAuthority := range []string{
		"CREATE TABLE coding_plans (",
		"CREATE TABLE coding_plan_leaves (",
	} {
		if strings.Contains(setup, retiredAuthority) {
			t.Fatalf("flat plan remains an authority: %q", retiredAuthority)
		}
	}
}

func TestAgentExecutionSchemaRejectsInvalidAuthorityTransitions(t *testing.T) {
	setup := string(SetupSQL())
	requireSchemaFragments(t, setup,
		"stale ledger revision: expected base",
		"ledger edge creates a cycle",
		"ledger edge references an unknown item",
		"incompatible agent task state transition",
		"coding_agent_one_active_interruption",
		"terminal agent attempt is immutable",
		"late attempt write rejected for inactive, expired, or terminal ledger attempt",
		"terminal executor attempt requires an observed-effect manifest",
		"agent queue may fail only for a recorded internal durable-state or invariant failure",
		"accepted task may reopen only for a concrete overall-review finding",
		"agent attempt lacks exact current task and frozen route authority",
		"ledger completion requires accepted tasks and a successful overall review",
		"task acceptance requires completed review, passing required checks, and no open review findings",
		"CREATE TRIGGER coding_agent_interruption_validate BEFORE INSERT OR UPDATE",
		"PERFORM 1 FROM coding_ledgers WHERE id=NEW.ledger_id FOR UPDATE;",
		"item not active in its revision",
		"agent task insert requires an eligible active task in the exact frozen revision",
		"coding_agent_attempt_history_immutable",
		"interruption disposition transition is not monotonic",
		"attempt_revision IS DISTINCT FROM NEW.revision",
		"coding agent grant identity, purpose, scope, and expiry are immutable",
		"expired coding agent grant cannot be consumed",
		"new interruption must start pending on an active frozen ledger",
		"coding_agent_interruption_history_immutable",
		"coding_agent_tasks_history_immutable",
		"coding_agent_internal_failures_immutable",
		"FOREIGN KEY (attempt_id,ledger_id,revision,task_item_id)",
		"check result does not belong to the attempt task",
		"correction.attempt_id>attempt.attempt_id",
		"later_cycle.attempt_id>attempt.attempt_id",
		"EXISTS (SELECT 1 FROM coding_ledgers ledger WHERE ledger.job_id=NEW.id)",
		"COALESCE(OLD.metadata->'coding_execution'->>'mode','normal')='agents'",
		"task.task_item_id=projection.item_id AND task.state='accepted'",
		"CONSTRAINT coding_agent_interruptions_disposition_shape CHECK",
		"coding_agent_grant_truncate_immutable",
		"FOREIGN KEY (id,frozen_revision) REFERENCES coding_ledger_revisions(ledger_id,revision)",
		"disposition.disposition='represented' AND (",
		"represented_projection.revision=NEW.current_revision",
		"covering_item.item_kind='acceptance_criterion'",
		"NEW.edge_kind='verifies' AND (from_kind<>'check' OR to_kind<>'acceptance_criterion')",
		"NEW.expires_at<=clock_timestamp()",
		"final_executor.attempt_id>result.attempt_id",
		"CREATE TRIGGER coding_agent_grant_change_validate BEFORE INSERT OR UPDATE OR DELETE",
		"coding agent grant does not match resolved interruption authority or lifetime",
		"AND (\n              NOT EXISTS (",
		"OR NOT EXISTS (\n                  SELECT 1\n                  FROM coding_ledger_edges requirement_coverage",
		"executor.role='executor' AND executor.status='completed'",
		"executor.attempt_id<attempt.attempt_id",
		"review finding lacks its exact live reviewer attempt and affected acceptance criterion",
		"finding acceptance target must be an active acceptance criterion in its exact revision",
		"FOREIGN KEY (attempt_id,ledger_id,revision)",
		"check_attempt.role='executor' AND check_attempt.status='completed'",
		"final_reviewer.attempt_id>result.attempt_id",
		"interruption.task_item_id=OLD.task_item_id",
		"interruption.ledger_id=NEW.id AND interruption.disposition='pending'",
		"ledger_state IS DISTINCT FROM 'frozen'",
		"attempt_ledger_state IS DISTINCT FROM 'frozen'",
		"FOR SHARE OF attempt,queue_attempt,ledger",
		"FOREIGN KEY (evidence_id,evidence_job_id) REFERENCES evidence(id,job_id)",
		"FOREIGN KEY (ledger_id,evidence_job_id) REFERENCES coding_ledgers(id,job_id)",
		"failure.authorized_job_updated_at=NEW.updated_at",
		"NEW.updated_at>OLD.updated_at",
		"dependency.edge_kind IN ('requires','blocks')",
		"awaiting-input task state requires exactly one pending interruption",
		"agent task transitions require an active frozen parent ledger",
		"attempt.status='active'",
		"FOREIGN KEY (evidence_id,job_id) REFERENCES evidence(id,job_id)",
		"finding.finding_kind='planning'",
		"projection.revision=ledger_revision",
		"executing task state requires exactly one active executor attempt",
		"attempt may expire only after its effective lease elapses",
		"require_live_coding_agent_effect_attempt",
		"require_live_coding_agent_check_attempt",
		"resume_evidence_job_id bigint",
		"FOREIGN KEY (resume_evidence_id,resume_evidence_job_id) REFERENCES evidence(id,job_id)",
		"i.item_kind='objective' AND v.disposition='active'",
		"PRIMARY KEY (ledger_id,frozen_revision,task_item_id)",
		"FOREIGN KEY (ledger_id,route_revision,task_item_id) REFERENCES coding_agent_tasks",
		"IF NEW.status<>'active' OR task_state IS NULL",
		"verifying task state requires the latest executor attempt to complete",
		"attempt_status IS DISTINCT FROM 'completed'",
		"changed agreed ledger item must return to proposed review state",
		"new ledger item must start in proposed review state",
		"coding ledger may enter revising only at a durable task boundary without active attempts",
		"completed executor attempt must proceed to verification",
		"ready task may execute only while every dependency is accepted",
		"CREATE TRIGGER coding_agent_ready_dependents_invalidate",
		"reviewing task state requires exactly one active task reviewer attempt",
		"check results require a completed executor attempt while verifying",
		"required_check.ledger_id=NEW.id",
		"changed_dependency.from_item_id,changed_dependency.to_item_id",
		"UPDATE coding_agent_tasks dependent",
		"FOREIGN KEY (attempt_id,ledger_id) REFERENCES coding_agent_attempts(attempt_id,ledger_id)",
		"FOREIGN KEY (use_evidence_id,use_evidence_job_id) REFERENCES evidence(id,job_id)",
		"authorized_item.owner_turn_id=agreement_revision.owner_turn_id",
		"authorized_turn.disposition='represented'",
		"current_check.required IS DISTINCT FROM prior_check.required",
		"unfinished_task.state<>'accepted'",
		"ORDER BY task_item_id FOR UPDATE",
		"ledger cancellation requires every task cancelled and no active execution or interruption",
	)
	if strings.Contains(setup, "TG_TABLE_NAME='coding_ledger_item_versions' AND NEW.review_state") {
		t.Fatal("shared ledger-content trigger must guard table-specific fields before dereferencing them")
	}
	if strings.Count(setup, "CREATE UNIQUE INDEX coding_agent_one_mutating_executor") != 1 {
		t.Fatal("schema must have one global mutating-executor authority")
	}
}

func TestAgentExecutionSchemaFreezesContractAndObservedEffects(t *testing.T) {
	setup := string(SetupSQL())
	requireSchemaFragments(t, setup,
		"ledger freeze requires one exact covered, agreed, resolved revision with frozen routes and tasks",
		"role text NOT NULL CHECK (role IN ('planner','executor','reviewer'))",
		"required boolean NOT NULL",
		"jsonb_typeof(argv->0)='string'",
		`NOT jsonb_path_exists(argv,'$[*] ? (@.type() != "string")')`,
		"rank bigint NOT NULL CHECK (rank>=0)",
		"coding_ledger_revisions_owner_turn_fkey",
		"coding_ledger_item_versions_removed_turn_fkey",
		"coding_ledger_origins_owner_turn_fkey",
		"frozen or historical ledger revision contents are immutable",
		"coding_ledger_item_versions_insert_mutable",
		"coding_ledger_revision_items_insert_mutable",
		"coding_ledger_edges_insert_mutable",
		"coding_ledger_routes_insert_mutable",
		"coding_ledger_checks_insert_mutable",
		"coding_ledger_questions_insert_mutable",
		"coding_ledger_questions_change_validate",
		"coding_ledger_findings_history_immutable",
		"ledger revision must carry every prior item forward with a version or tombstone",
		"requirement_item.item_kind IN ('requirement','objective')",
		"stale_content:=target_revision<>current_value+1",
		"requirement_item.item_kind='requirement'",
		"acceptance_item.item_kind='acceptance_criterion'",
		"schema='omnidex.observed-effect-manifest.v1'",
		"declared_effects jsonb NOT NULL",
		"before_state jsonb NOT NULL",
		"after_state jsonb NOT NULL",
		"vcs_diffs jsonb NOT NULL",
		"discrepancies jsonb NOT NULL",
	)
}

func TestAgentExecutionSchemaClosesReviewedAuthorityGaps(t *testing.T) {
	setup := string(SetupSQL())
	maintenanceBytes, err := os.ReadFile("agent_execution_maintenance.sql")
	if err != nil {
		t.Fatal(err)
	}
	maintenance := string(maintenanceBytes)

	for _, source := range []string{setup, maintenance} {
		requireSchemaFragments(t, source,
			"creation_transaction bigint NOT NULL DEFAULT txid_current()",
			"revision_transaction IS DISTINCT FROM txid_current()",
			"FOREIGN KEY (ledger_id,revision,item_id) REFERENCES coding_ledger_revision_items",
			"FOREIGN KEY (attempt_id,ledger_id,revision)",
			"REFERENCES coding_agent_attempts(attempt_id,ledger_id,route_revision) ON DELETE RESTRICT",
			"CHECK (isfinite(expires_at) AND expires_at>started_at)",
			"prior_frozen.state='frozen'",
			"route_approval.owner_turn_id IS NULL",
			"later_executor.attempt_id>check_attempt.attempt_id",
			"OLD.disposition='pending'",
			"attempt_status NOT IN ('question','interrupted') OR queue_status<>'waiting_input'",
			"attempt_task IS DISTINCT FROM NEW.item_id",
		)
		findingStart := strings.Index(source, "CREATE TABLE")
		findingStart += strings.Index(source[findingStart:], "coding_ledger_findings (")
		findingEnd := findingStart + strings.Index(source[findingStart:], ");")
		if findingStart < 0 || findingEnd < findingStart {
			t.Fatal("schema lacks a bounded findings table definition")
		}
		findingTable := source[findingStart:findingEnd]
		if strings.Contains(findingTable, "REFERENCES coding_ledger_item_versions") {
			t.Fatal("findings must target an item projected into the reviewed revision")
		}
	}

	requireSchemaFragments(t, maintenance,
		"Every frozen generation receives durable task history",
		"IF legacy_plan.state IN ('frozen','superseded') AND has_frozen_agent_workload THEN",
	)
	if strings.Contains(maintenance, "SELECT max(final_plan.generation)") {
		t.Fatal("migration must create task history for superseded frozen generations")
	}
}

func TestAgentExecutionSchemaMaintenanceMigratesAndRetiresFlatAuthority(t *testing.T) {
	maintenanceBytes, err := os.ReadFile("agent_execution_maintenance.sql")
	if err != nil {
		t.Fatal(err)
	}
	maintenance := string(maintenanceBytes)
	requireSchemaFragments(t, maintenance,
		"BEGIN;",
		"LOCK TABLE coding_plans, coding_plan_leaves IN ACCESS EXCLUSIVE MODE;",
		"Converted byte-for-byte from a flat coding plan leaf.",
		"legacy_result_schema",
		"legacy_result_relation",
		"legacy_workload_task_id",
		"Converted from the preserved frozen workload acceptance contract.",
		"Converted from the preserved frozen workload required check.",
		"Preserved as an explicit delivery obligation against the migrated objective.",
		"frozen.payload_json->>'phase'='plan_frozen'",
		"CASE WHEN has_frozen_agent_workload THEN 'frozen' ELSE 'proposed' END",
		"IF has_frozen_agent_workload THEN",
		"Tombstoned because the later legacy generation omitted this task or contract child.",
		"Every frozen generation receives durable task history",
		"IF legacy_plan.state IN ('frozen','superseded') AND has_frozen_agent_workload THEN",
		"ALTER TABLE coding_ledgers DISABLE TRIGGER coding_ledger_update_validate;",
		"SET CONSTRAINTS ALL IMMEDIATE;",
		"ALTER TABLE coding_ledgers ENABLE TRIGGER coding_ledger_update_validate;",
		"coding_ledger_revision_carry_forward",
		"prior.revision=revision_number-1",
		"version.revision=revision_number",
		"has_frozen_agent_workload",
		"CASE WHEN legacy_plan.state='canceled' THEN 'cancelled' ELSE 'frozen' END",
		"IF legacy_plan.state IN ('frozen','superseded') AND has_frozen_agent_workload THEN",
		"coding_ledger_questions_change_validate",
		"coding_ledger_findings_history_immutable",
		"coding_agent_tasks_history_immutable",
		"coding_agent_internal_failures_immutable",
		"frozen.payload_json->>'generation'=legacy_plan.generation::text",
		"Preserved dependency from the frozen workload.",
		"Converted from the preserved frozen workload overall required check.",
		"reviewed.payload_json->'delegation'->role_name->>'provider'",
		"reviewed.payload_json->'workload'=frozen.payload_json->'workload'",
		"prior.item_kind IN ('acceptance_criterion','check')",
		"accepted_workload.task=current_workload.task",
		"DROP FUNCTION IF EXISTS require_live_coding_agent_attempt();",
		"FOREIGN KEY (evidence_id,job_id) REFERENCES evidence(id,job_id)",
		"DROP TRIGGER IF EXISTS coding_plan_steps_require_plan_state ON job_steps;",
		"DROP TRIGGER IF EXISTS job_lifecycle_operations_require_coding_plan_result ON job_lifecycle_operations;",
		"ALTER TABLE coding_plan_operation_results RENAME TO retired_coding_plan_operation_results;",
		"ALTER TABLE coding_agent_tasks DISABLE TRIGGER coding_agent_task_transition_validate;",
		"ALTER TABLE coding_agent_tasks ENABLE TRIGGER coding_agent_task_transition_validate;",
		"ALTER TABLE coding_ledger_item_versions DISABLE TRIGGER coding_ledger_item_versions_insert_mutable;",
		"ALTER TABLE coding_ledger_item_versions ENABLE TRIGGER coding_ledger_item_versions_insert_mutable;",
		"ALTER TABLE coding_plan_leaves RENAME TO retired_coding_plan_leaves;",
		"ALTER TABLE coding_plans RENAME TO retired_coding_plans;",
		"COMMIT;",
	)
	if strings.Contains(maintenance, "IF legacy_plan.state IN ('frozen','canceled') THEN") {
		t.Fatal("maintenance must not freeze an incomplete non-agent normalized revision")
	}
	if strings.Contains(maintenance, "FROM coding_agent_settings") {
		t.Fatal("maintenance must use the exact frozen delegation receipt, not mutable current settings")
	}
	if strings.Contains(maintenance, "e.payload_json->>'task_id'=leaf.leaf_id") {
		t.Fatal("maintenance compares accepted workload task identities to unrelated flat leaf identities")
	}
	for _, staleProjection := range []string{
		"item.created_revision=revision_number\n          AND item.item_kind='acceptance_criterion'",
		"item.created_revision=revision_number AND item.item_kind='check'",
	} {
		if strings.Contains(maintenance, staleProjection) {
			t.Fatalf("maintenance omits reused frozen workload items: %q", staleProjection)
		}
	}
	if strings.Index(maintenance, "SET CONSTRAINTS ALL IMMEDIATE;") >
		strings.Index(maintenance, "ALTER TABLE coding_ledgers ENABLE TRIGGER coding_ledger_update_validate;") {
		t.Fatal("maintenance re-enables ledger triggers before flushing deferred carry-forward events")
	}
	for _, destructive := range []string{"DROP TABLE coding_plan", "DELETE FROM evidence", "DELETE FROM coding_plan"} {
		if strings.Contains(maintenance, destructive) {
			t.Fatalf("maintenance destroys preserved authority: %q", destructive)
		}
	}
}

func TestAgentExecutionSchemaPreservesRevisionProgressAndBoundedAuthority(t *testing.T) {
	setup := string(SetupSQL())
	maintenanceBytes, err := os.ReadFile("agent_execution_maintenance.sql")
	if err != nil {
		t.Fatal(err)
	}
	maintenance := string(maintenanceBytes)

	for _, source := range []string{setup, maintenance} {
		requireSchemaFragments(t, source,
			"new coding ledger must start proposed at revision zero",
			"interruption.ledger_id=OLD.id AND interruption.disposition='pending'",
			"task.frozen_revision=NEW.frozen_revision",
			"expires_at timestamptz NOT NULL",
			"CHECK (isfinite(expires_at) AND expires_at>granted_at)",
			"SELECT max(prior_frozen.revision)",
			"prior_frozen.revision<NEW.frozen_revision",
			"prior_frozen.state='frozen'",
			"prior_task.state='accepted'",
			"prior_projection.item_revision=projected_version",
		)
		if strings.Contains(source, "CHECK (expires_at IS NULL OR expires_at>granted_at)") ||
			strings.Contains(source, "CHECK (expires_at>granted_at)") {
			t.Fatal("capability grants must always record a finite bounded lifetime")
		}

		updateVerb := "CREATE FUNCTION "
		taskVerb := "CREATE FUNCTION "
		if source == maintenance {
			updateVerb = "CREATE OR REPLACE FUNCTION "
			taskVerb = "CREATE OR REPLACE FUNCTION "
		}
		updateBody := schemaFunctionBody(t, source, updateVerb+"validate_coding_ledger_update() RETURNS trigger")
		requireSchemaFragments(t, updateBody,
			"coding ledger may enter revising only at a durable task boundary without active attempts or interruptions",
			"task.ledger_id=NEW.id AND task.frozen_revision=NEW.frozen_revision",
		)
		taskBody := schemaFunctionBody(t, source, taskVerb+"validate_coding_agent_task_transition() RETURNS trigger")
		requireSchemaFragments(t, taskBody,
			"NEW.state NOT IN ('ready','accepted')",
			"NEW.state='accepted'",
			"prior_projection.item_revision=projected_version",
		)
	}

	requireSchemaFragments(t, maintenance,
		"generation.feedback AS generation_feedback",
		"CASE WHEN legacy_plan.generation=1",
		"ELSE legacy_plan.generation_feedback",
		"accepted_frozen.payload_json->>'generation'=accepted.payload_json->>'generation'",
		"accepted_workload.task=current_workload.task",
		"CASE WHEN acceptance.carried_accepted THEN 'accepted' ELSE 'ready' END",
		"CASE WHEN has_frozen_agent_workload THEN 'frozen' ELSE 'proposed' END",
		"objective_review:=CASE WHEN has_frozen_agent_workload THEN 'agreed' ELSE 'proposed' END",
	)
	if strings.Contains(maintenance, "accepted_dependency") {
		t.Fatal("migration must create state rows for dependency-blocked frozen tasks")
	}
}

func TestAgentExecutionMaintenanceRetainsFreshSchemaAuthorityGuards(t *testing.T) {
	setup := string(SetupSQL())
	maintenanceBytes, err := os.ReadFile("agent_execution_maintenance.sql")
	if err != nil {
		t.Fatal(err)
	}
	maintenance := string(maintenanceBytes)

	for _, function := range []string{
		"validate_coding_ledger_update() RETURNS trigger",
		"validate_coding_agent_attempt_insert() RETURNS trigger",
		"validate_coding_agent_task_transition() RETURNS trigger",
		"require_live_coding_agent_effect_attempt() RETURNS trigger",
		"require_live_coding_agent_check_attempt() RETURNS trigger",
		"validate_coding_ledger_revision_insert() RETURNS trigger",
		"validate_coding_ledger_content_insert() RETURNS trigger",
		"validate_coding_ledger_finding_insert() RETURNS trigger",
		"validate_coding_agent_grant_change() RETURNS trigger",
		"require_coding_agent_attempt_manifest() RETURNS trigger",
		"validate_coding_agent_interruption() RETURNS trigger",
		"require_coding_ledger_frozen_tasks() RETURNS trigger",
	} {
		setupBody := schemaFunctionBody(t, setup, "CREATE FUNCTION "+function)
		maintenanceBody := schemaFunctionBody(t, maintenance, "CREATE OR REPLACE FUNCTION "+function)
		if setupBody != maintenanceBody {
			t.Fatalf("maintenance definition of %s diverges from the authoritative fresh schema", function)
		}
	}

	for _, source := range []string{setup, maintenance} {
		requireSchemaFragments(t, source,
			"CREATE CONSTRAINT TRIGGER coding_ledger_frozen_tasks",
			"DEFERRABLE INITIALLY DEFERRED FOR EACH ROW",
			"frozen ledger requires a durable state row for every active task",
			"task.frozen_revision=NEW.frozen_revision",
			"task.task_item_id=task_projection.item_id",
			"NEW.status<>'active' AND OLD.role='executor'",
			"resolved interruption answer and resume evidence are immutable",
			"OLD.answer IS DISTINCT FROM NEW.answer",
			"ROW(OLD.resume_evidence_id,OLD.resume_evidence_job_id)",
		)
	}
}

func TestAgentExecutionSchemaSupportsFrozenDependenciesAndExpiryRecovery(t *testing.T) {
	maintenanceBytes, err := os.ReadFile("agent_execution_maintenance.sql")
	if err != nil {
		t.Fatal(err)
	}
	sources := []struct {
		name                    string
		source                  string
		functionDeclarationVerb string
	}{
		{name: "setup", source: string(SetupSQL()), functionDeclarationVerb: "CREATE FUNCTION "},
		{name: "maintenance", source: string(maintenanceBytes), functionDeclarationVerb: "CREATE OR REPLACE FUNCTION "},
	}

	for _, candidate := range sources {
		t.Run(candidate.name, func(t *testing.T) {
			if strings.Contains(candidate.source, "LANGUAGE plpgsql AS $\n") {
				t.Fatal("schema retains an invalid lone dollar-quote delimiter")
			}

			taskBody := schemaFunctionBody(t, candidate.source,
				candidate.functionDeclarationVerb+"validate_coding_agent_task_transition() RETURNS trigger")
			insertEnd := strings.Index(taskBody, "        RETURN NEW;\n    END IF;")
			if insertEnd < 0 {
				t.Fatal("task transition validator lacks its insert branch terminator")
			}
			if strings.Contains(taskBody[:insertEnd], "coding_ledger_edges dependency") {
				t.Fatal("initial durable task rows must represent dependency-blocked frozen tasks")
			}
			requireSchemaFragments(t, taskBody,
				"IF NEW.state='executing' AND EXISTS (",
				"ready task may execute only while every dependency is accepted",
			)

			effectBody := schemaFunctionBody(t, candidate.source,
				candidate.functionDeclarationVerb+"require_live_coding_agent_effect_attempt() RETURNS trigger")
			requireSchemaFragments(t, effectBody,
				"queue_status='active' AND attempt_expiry>clock_timestamp()",
				"TG_TABLE_NAME='coding_agent_effect_manifests'",
				"queue_status<>'active' OR attempt_expiry<=clock_timestamp()",
				"late attempt write rejected for inactive, expired, or terminal ledger attempt",
			)
			if strings.Contains(effectBody, "WHERE attempt.attempt_id=NEW.attempt_id AND queue_attempt.status='active'") {
				t.Fatal("manifest recovery must remain possible after the queue lease stops being active")
			}

			grantBody := schemaFunctionBody(t, candidate.source,
				candidate.functionDeclarationVerb+"validate_coding_agent_grant_change() RETURNS trigger")
			requireSchemaFragments(t, grantBody,
				"NEW.consumed_at IS NOT NULL AND",
				"(NEW.consumed_at>NEW.expires_at OR NEW.expires_at<=clock_timestamp())",
			)
		})
	}
}
