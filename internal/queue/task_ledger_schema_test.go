package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
)

func TestTaskLedgerMigrationDefinesOneJobOwnedNormalizedAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/027_task_ledger.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS task_ledgers",
		"CREATE OR REPLACE FUNCTION task_ledger_text_is_exact(value TEXT)",
		"CREATE OR REPLACE FUNCTION task_ledger_uri_is_valid(value TEXT)",
		"U&'\\0009\\000A\\000B\\000C\\000D\\0020\\0085\\00A0",
		"id TEXT PRIMARY KEY CHECK (id ~ '^ledger_[0-9a-f]{64}$')",
		"CREATE TABLE IF NOT EXISTS task_nodes",
		"CREATE TABLE IF NOT EXISTS task_node_verification_refs",
		"CREATE TABLE IF NOT EXISTS task_node_edges",
		"CREATE TABLE IF NOT EXISTS task_entries",
		"CREATE TABLE IF NOT EXISTS task_entry_refs",
		"CREATE TABLE IF NOT EXISTS task_events",
		"run_id UUID NOT NULL UNIQUE REFERENCES omni_runs(id) ON DELETE RESTRICT",
		"owner_type = 'job'",
		"owner_id = job_id",
		"closed_at TIMESTAMPTZ",
		"status = 'active' AND closed_at IS NULL",
		"'active', 'closed', 'failed', 'canceled'",
		"'accepted_decision', 'question', 'failure', 'checkpoint', 'note', 'feedback'",
		"feedback_purpose TEXT",
		"feedback_purpose IN ('replan', 'interrupt', 'input_response')",
		"uri TEXT NOT NULL CHECK (task_ledger_uri_is_valid(uri))",
		"assigned_step_id BIGINT",
		"idx_task_nodes_ledger_assigned_step",
		"CHECK (objective_id IS NULL OR objective_id <> id)",
		"status_reason TEXT NOT NULL DEFAULT ''",
		"disposition_reason TEXT NOT NULL DEFAULT ''",
		"disposition_by TEXT",
		"disposition_by IS NULL OR disposition_by IN ('user', 'code')",
		"jsonb_typeof(acceptance_criteria) IS NOT DISTINCT FROM 'array'",
		"jsonb_typeof(metadata) IS NOT DISTINCT FROM 'object'",
		"taskstate enforces the exact 65,536-byte compact canonical JSON contract",
		"octet_length(metadata::text) <= 131072",
		"jsonb_typeof(payload) IS NOT DISTINCT FROM 'object'",
		"created_version BIGINT NOT NULL CHECK (created_version > 0)",
		"position INT NOT NULL CHECK (position >= 0)",
		"updated_version BIGINT NOT NULL CHECK (updated_version >= created_version)",
		"idx_task_entries_one_replacement",
		"source_entry_id TEXT",
		"acceptance_policy TEXT",
		"accepted_by TEXT",
		"idx_task_entries_one_acceptance",
		"ledger_version BIGINT NOT NULL CHECK (ledger_version > 0)",
		"UNIQUE (ledger_id, ledger_version)",
		"command_id TEXT NOT NULL CHECK (command_id ~ '^command_[0-9a-f]{64}$')",
		"command_sha256 TEXT NOT NULL CHECK (command_sha256 ~ '^[0-9a-f]{64}$')",
		"content_sha256 = encode(digest(content, 'sha256'), 'hex')",
		"CHECK ((payload ->> 'ledger_id') IS NOT DISTINCT FROM ledger_id)",
		"CHECK ((payload ->> 'command_sha256') IS NOT DISTINCT FROM command_sha256)",
		"CHECK ((payload ->> 'event_kind') IS NOT DISTINCT FROM event_kind)",
		"step_id IS NULL AND NOT (payload ? 'step_id')",
		"command_kind = 'add_node' AND event_kind = 'node_added'",
		"command_kind = 'close_ledger' AND event_kind = 'ledger_closed'",
		"UNIQUE (ledger_id, command_id)",
		"UNIQUE (ledger_id, from_node_id, to_node_id, kind)",
		"UNIQUE (job_id, run_id)",
		"prevent_task_event_mutation",
		"task events are immutable",
		"BEFORE UPDATE OR DELETE ON task_events",
		"BEFORE TRUNCATE ON task_events",
		"FOR EACH STATEMENT EXECUTE FUNCTION prevent_task_event_mutation()",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("task ledger migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"working_set",
		"context_projection",
		"context_projections",
		"provenance jsonb",
	} {
		if strings.Contains(strings.ToLower(source), forbidden) {
			t.Fatalf("task ledger migration introduced forbidden schema %q", forbidden)
		}
	}
}

func TestTaskLedgerMigrationAcceptsEveryTaskStateValue(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/027_task_ledger.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	values := []string{
		string(taskstate.OwnerJob),
		string(taskstate.LedgerActive), string(taskstate.LedgerClosed),
		string(taskstate.LedgerFailed), string(taskstate.LedgerCanceled),
		string(taskstate.AuthorityUser), string(taskstate.AuthorityCode),
		string(taskstate.AuthorityToolEvidence), string(taskstate.AuthorityModelProposal),
		string(taskstate.AuthorityAcceptedModelDecision),
		string(taskstate.NodeGoal), string(taskstate.NodeObjective), string(taskstate.NodeTask),
		string(taskstate.NodeCheckpoint), string(taskstate.NodeChangeGroup),
		string(taskstate.NodePending), string(taskstate.NodeReady), string(taskstate.NodeActive),
		string(taskstate.NodeBlocked), string(taskstate.NodeDone), string(taskstate.NodeFailed),
		string(taskstate.NodeCanceled),
		string(taskstate.EdgeDependsOn), string(taskstate.EdgeBlocks),
		string(taskstate.EdgeDecomposes), string(taskstate.EdgeVerifies),
		string(taskstate.EntryConstraint), string(taskstate.EntryFact),
		string(taskstate.EntryObservation), string(taskstate.EntryHypothesis),
		string(taskstate.EntryDecisionCandidate), string(taskstate.EntryAcceptedDecision),
		string(taskstate.EntryQuestion), string(taskstate.EntryFailure),
		string(taskstate.EntryCheckpoint), string(taskstate.EntryNote), string(taskstate.EntryFeedback),
		string(taskstate.FeedbackReplan), string(taskstate.FeedbackInterrupt),
		string(taskstate.FeedbackInputResponse),
		string(taskstate.EntryActive), string(taskstate.EntryResolved),
		string(taskstate.EntryRejected), string(taskstate.EntrySuperseded),
		string(taskstate.RefEvidence), string(taskstate.RefSource), string(taskstate.RefSupports),
		string(taskstate.RefContradicts), string(taskstate.RefConcerns),
		string(taskstate.RefVerifies), string(taskstate.RefSupersedes),
		string(taskstate.CommandAddNode), string(taskstate.CommandAddEdge),
		string(taskstate.CommandAddEntry), string(taskstate.CommandRejectEntry),
		string(taskstate.CommandResolveEntry), string(taskstate.CommandSupersedeEntry),
		string(taskstate.CommandAcceptDecision), string(taskstate.CommandPromoteReady),
		string(taskstate.CommandAssignStep), string(taskstate.CommandTransitionNode),
		string(taskstate.CommandCloseLedger),
		string(taskstate.EventNodeAdded), string(taskstate.EventEdgeAdded),
		string(taskstate.EventEntryAdded), string(taskstate.EventEntryRejected),
		string(taskstate.EventEntryResolved), string(taskstate.EventEntrySuperseded),
		string(taskstate.EventDecisionAccepted), string(taskstate.EventNodesReadied),
		string(taskstate.EventNodeStepAssigned), string(taskstate.EventNodeTransitioned),
		string(taskstate.EventLedgerClosed),
	}
	for _, value := range values {
		if !strings.Contains(source, "'"+value+"'") {
			t.Errorf("task ledger migration rejects taskstate value %q", value)
		}
	}
}

func TestTaskLedgerMigrationUsesCompositeOwnershipReferences(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/027_task_ledger.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"FOREIGN KEY (ledger_id, job_id)",
		"REFERENCES task_ledgers(id, job_id)",
		"FOREIGN KEY (ledger_id, parent_id)",
		"FOREIGN KEY (ledger_id, objective_id)",
		"FOREIGN KEY (job_id, assigned_step_id)",
		"REFERENCES task_nodes(ledger_id, id)",
		"FOREIGN KEY (ledger_id, from_node_id)",
		"FOREIGN KEY (ledger_id, node_id)",
		"FOREIGN KEY (ledger_id, to_node_id)",
		"FOREIGN KEY (ledger_id, scope_node_id)",
		"FOREIGN KEY (ledger_id, supersedes_id)",
		"FOREIGN KEY (ledger_id, entry_id)",
		"REFERENCES task_entries(ledger_id, id)",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("task ledger ownership constraint omitted %q", required)
		}
	}
	if strings.Contains(source, "CREATE UNIQUE INDEX IF NOT EXISTS idx_task_nodes_ledger_objective") {
		t.Fatal("task nodes incorrectly limit an objective to one child")
	}
}

func TestTaskLedgerQueueUsesOneNormalizedAuthorityAndBoundedHistory(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("task_ledger*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToUpper(string(raw)), "ON CONFLICT") {
			t.Fatalf("%s introduced a task-ledger upsert fallback", name)
		}
		if strings.Contains(string(raw), "CreateTaskLedger(") {
			t.Fatalf("%s exposes obsolete manual task ledger creation", name)
		}
	}
	storeRaw, err := os.ReadFile("task_ledger_store.go")
	if err != nil {
		t.Fatal(err)
	}
	store := string(storeRaw)
	if strings.Contains(store, "FROM task_events") {
		t.Fatal("normal task ledger restoration reads immutable event history")
	}
	for _, required := range []string{
		"TaskLedger(ctx context.Context, jobID int64) (taskstate.MaterializedState, error)",
		"maxTaskLedgerNodes    = taskstate.MaxLedgerNodes",
		"maxTaskLedgerNodeRefs = taskstate.MaxLedgerNodeVerificationRefs",
		"maxTaskLedgerEdges    = taskstate.MaxLedgerEdges",
		"maxTaskLedgerEntries  = taskstate.MaxLedgerEntries",
		"maxTaskLedgerRefs     = taskstate.MaxLedgerEntryRefs",
		"LIMIT $2",
	} {
		if !strings.Contains(store, required) {
			t.Fatalf("bounded normalized task ledger store omitted %q", required)
		}
	}
	jobLock := strings.Index(store, `jobQuery :=`)
	ledgerLock := strings.Index(store, `ledgerQuery :=`)
	if jobLock < 0 || ledgerLock < 0 || jobLock >= ledgerLock {
		t.Fatal("task ledger lock order is not jobs then task_ledgers")
	}
	eventsRaw, err := os.ReadFile("task_ledger_events.go")
	if err != nil {
		t.Fatal(err)
	}
	events := string(eventsRaw)
	for _, required := range []string{"maxTaskEventPageSize = 100", "id>$3", "ORDER BY id ASC", "LIMIT $4"} {
		if !strings.Contains(events, required) {
			t.Fatalf("bounded task event history omitted %q", required)
		}
	}
	applyRaw, err := os.ReadFile("task_ledger_apply.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(applyRaw), "taskstate.ValidateMaterializedState(state)") {
		t.Fatal("task command persistence does not validate the bounded post-apply materialized state")
	}
	nodeRefsRaw, err := os.ReadFile("task_ledger_node_refs.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(nodeRefsRaw), "LIMIT $2") {
		t.Fatal("normalized node verification references are not hard bounded")
	}
	recordsRaw, err := os.ReadFile("task_ledger_records.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(recordsRaw), "LIMIT $2") != 3 {
		t.Fatal("normalized edge, entry, and entry-reference reads are not all hard bounded")
	}
}
