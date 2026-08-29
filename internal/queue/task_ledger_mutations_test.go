package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresTaskLedgerRoundTripsEveryMutationAndEventPage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}

	marker := fmt.Sprintf("task-ledger-mutations-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	var stepID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps WHERE job_id=$1 ORDER BY sort_index, id LIMIT 1
	`, job.ID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}

	events := make([]taskstate.Event, 0, 15)
	apply := func(command taskstate.Command, want taskstate.EventKind) {
		t.Helper()
		event, err := repository.ApplyTaskCommand(ctx, job.ID, 1, command)
		if err != nil {
			t.Fatalf("apply %s command: %v", want, err)
		}
		if event.Kind != want {
			t.Fatalf("command event kind=%q, want %q", event.Kind, want)
		}
		events = append(events, event)
	}
	empty := taskstate.EmptyJSONObject()
	expandedMetadata := taskLedgerMutationExpandedMetadata(t)
	criteria := []string{"The bounded behavior is verified."}
	apply(taskstate.AddNodeCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "add-task"), ExpectedVersion: initialTaskLedgerVersion,
		Actor: taskstate.AuthorityCode, ID: "task-one", Kind: taskstate.NodeTask,
		Title: "Task one", Priority: 20, CreatedStepID: &stepID,
		AcceptanceCriteria: criteria, Metadata: expandedMetadata,
	}, taskstate.EventNodeAdded)
	apply(taskstate.AddNodeCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "add-checkpoint"), ExpectedVersion: initialTaskLedgerVersion + 1,
		Actor: taskstate.AuthorityCode, ID: "checkpoint-one", Kind: taskstate.NodeCheckpoint,
		Title: "Checkpoint one", Priority: 10,
		AcceptanceCriteria: []string{}, Metadata: empty,
	}, taskstate.EventNodeAdded)
	apply(taskstate.AddEdgeCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "add-edge"), ExpectedVersion: initialTaskLedgerVersion + 2,
		Actor: taskstate.AuthorityCode, ID: "edge-verifies", Kind: taskstate.EdgeVerifies,
		From: "checkpoint-one", To: "task-one",
	}, taskstate.EventEdgeAdded)
	assertTaskLedgerMutationPhase(t, ctx, repository, job.ID, initialTaskLedgerVersion+3, func(state taskstate.MaterializedState) {
		if len(state.Nodes) != 3 || len(state.Edges) != 1 || len(state.Entries) != 1 {
			t.Fatalf("node/edge phase state=%+v", state)
		}
	})

	refSource := taskLedgerMutationRef("source://mutation/input", taskstate.RefSource, "a")
	refEvidence := taskLedgerMutationRef("evidence://mutation/resolution", taskstate.RefEvidence, "b")
	refContradiction := taskLedgerMutationRef("evidence://mutation/contradiction", taskstate.RefContradicts, "c")
	apply(taskstate.AddEntryCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "add-rejected"), ExpectedVersion: initialTaskLedgerVersion + 3,
		Actor: taskstate.AuthorityCode, ID: "entry-rejected", Kind: taskstate.EntryNote,
		Content: "Reject this bounded note.", Metadata: empty, Refs: []taskstate.Ref{},
	}, taskstate.EventEntryAdded)
	apply(taskstate.RejectEntryCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "reject-entry"), ExpectedVersion: initialTaskLedgerVersion + 4,
		Actor: taskstate.AuthorityCode, EntryID: "entry-rejected", Reason: "The evidence invalidated it.",
		Refs: []taskstate.Ref{refContradiction},
	}, taskstate.EventEntryRejected)
	apply(taskstate.AddEntryCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "add-resolved"), ExpectedVersion: initialTaskLedgerVersion + 5,
		Actor: taskstate.AuthorityCode, ID: "entry-resolved", Kind: taskstate.EntryQuestion,
		Content: "Can the persisted state answer this?", Metadata: empty, Refs: []taskstate.Ref{refSource},
	}, taskstate.EventEntryAdded)
	apply(taskstate.ResolveEntryCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "resolve-entry"), ExpectedVersion: initialTaskLedgerVersion + 6,
		Actor: taskstate.AuthorityCode, EntryID: "entry-resolved", Reason: "The verification evidence answers it.",
		Refs: []taskstate.Ref{refEvidence},
	}, taskstate.EventEntryResolved)
	for index, id := range []taskstate.EntryID{"entry-old", "entry-replacement"} {
		apply(taskstate.AddEntryCommand{
			CommandID:       taskLedgerMutationCommandID(t, marker, "add-supersession-"+string(id)),
			ExpectedVersion: initialTaskLedgerVersion + uint64(7+index), Actor: taskstate.AuthorityCode,
			ID: id, Kind: taskstate.EntryNote, Content: "Supersession content " + string(id),
			Metadata: empty, Refs: []taskstate.Ref{},
		}, taskstate.EventEntryAdded)
	}
	apply(taskstate.SupersedeEntryCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "supersede-entry"), ExpectedVersion: initialTaskLedgerVersion + 9,
		Actor: taskstate.AuthorityCode, EntryID: "entry-old", ReplacementID: "entry-replacement",
		Reason: "The replacement is authoritative.",
	}, taskstate.EventEntrySuperseded)
	assertTaskLedgerMutationPhase(t, ctx, repository, job.ID, initialTaskLedgerVersion+10, func(state taskstate.MaterializedState) {
		entries := taskLedgerMutationEntries(state.Entries)
		if len(entries) != 5 || entries["entry-rejected"].Status != taskstate.EntryRejected ||
			entries["entry-rejected"].DispositionBy != taskstate.AuthorityCode ||
			!reflect.DeepEqual(entries["entry-rejected"].Refs, []taskstate.Ref{refContradiction}) ||
			entries["entry-resolved"].Status != taskstate.EntryResolved ||
			entries["entry-resolved"].DispositionBy != taskstate.AuthorityCode ||
			!reflect.DeepEqual(entries["entry-resolved"].Refs, []taskstate.Ref{refSource, refEvidence}) ||
			entries["entry-old"].SupersededBy != "entry-replacement" ||
			entries["entry-old"].DispositionBy != taskstate.AuthorityCode ||
			entries["entry-replacement"].SupersedesID != "entry-old" {
			t.Fatalf("entry mutation phase entries=%+v", state.Entries)
		}
	})

	apply(taskstate.PromoteReadyNodesCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "promote-ready"), ExpectedVersion: initialTaskLedgerVersion + 10,
		Actor: taskstate.AuthorityCode,
	}, taskstate.EventNodesReadied)
	apply(taskstate.AssignNodeStepCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "assign-step"), ExpectedVersion: initialTaskLedgerVersion + 11,
		Actor: taskstate.AuthorityCode, NodeID: "task-one", StepID: stepID,
	}, taskstate.EventNodeStepAssigned)
	apply(taskstate.TransitionNodeCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "activate-node"), ExpectedVersion: initialTaskLedgerVersion + 12,
		Actor: taskstate.AuthorityCode, NodeID: "task-one", To: taskstate.NodeActive,
		VerificationRefs: []taskstate.Ref{},
	}, taskstate.EventNodeTransitioned)
	completionRef := taskLedgerMutationRef("evidence://mutation/completion", taskstate.RefVerifies, "c")
	apply(taskstate.TransitionNodeCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "complete-node"), ExpectedVersion: initialTaskLedgerVersion + 13,
		Actor: taskstate.AuthorityCode, NodeID: "task-one", To: taskstate.NodeDone,
		CompletedStepID: &stepID, VerificationRefs: []taskstate.Ref{completionRef},
	}, taskstate.EventNodeTransitioned)
	assertTaskLedgerMutationPhase(t, ctx, repository, job.ID, initialTaskLedgerVersion+14, func(state taskstate.MaterializedState) {
		nodes := taskLedgerMutationNodes(state.Nodes)
		task := nodes["task-one"]
		if task.Status != taskstate.NodeDone || task.AssignedStepID == nil || *task.AssignedStepID != stepID ||
			task.CompletedStepID == nil || *task.CompletedStepID != stepID ||
			!reflect.DeepEqual(task.Metadata.Bytes(), expandedMetadata.Bytes()) ||
			!reflect.DeepEqual(task.VerificationRefs, []taskstate.Ref{completionRef}) ||
			nodes["checkpoint-one"].Status != taskstate.NodeReady {
			t.Fatalf("node mutation phase nodes=%+v", state.Nodes)
		}
	})
	if result, err := pool.Exec(ctx, `
		UPDATE jobs SET status=$2, completed_at=NOW(), updated_at=NOW() WHERE id=$1
	`, job.ID, model.JobStatusFailed); err != nil || result.RowsAffected() != 1 {
		t.Fatalf("prepare matching terminal job state: rows=%d error=%v", result.RowsAffected(), err)
	}
	apply(taskstate.CloseLedgerCommand{
		CommandID: taskLedgerMutationCommandID(t, marker, "close-ledger"), ExpectedVersion: initialTaskLedgerVersion + 14,
		Actor: taskstate.AuthorityCode, Status: taskstate.LedgerFailed,
		StepID: &stepID, Reason: "The remaining checkpoint failed explicitly.",
	}, taskstate.EventLedgerClosed)
	assertTaskLedgerMutationPhase(t, ctx, repository, job.ID, initialTaskLedgerVersion+15, func(state taskstate.MaterializedState) {
		if state.Status != taskstate.LedgerFailed {
			t.Fatalf("closed ledger status=%q", state.Status)
		}
	})

	seedEvents, err := repository.ListTaskEvents(ctx, job.ID, 0, int(initialTaskLedgerVersion))
	if err != nil || len(seedEvents) != int(initialTaskLedgerVersion) {
		t.Fatalf("initial task event page=%+v error=%v", seedEvents, err)
	}
	cursor := seedEvents[len(seedEvents)-1].ID
	for index, want := range events {
		page, err := repository.ListTaskEvents(ctx, job.ID, cursor, 1)
		if err != nil {
			t.Fatalf("event page %d: %v", index, err)
		}
		if len(page) != 1 || page[0].ID <= cursor ||
			taskLedgerTestEventJSON(t, page[0].Event) != taskLedgerTestEventJSON(t, want) {
			t.Fatalf("event page %d=%+v, want %+v after %d", index, page, want, cursor)
		}
		cursor = page[0].ID
	}
	page, err := repository.ListTaskEvents(ctx, job.ID, cursor, 1)
	if err != nil || len(page) != 0 {
		t.Fatalf("event page after final cursor=%+v error=%v", page, err)
	}
	assertTaskLedgerDatabaseCounts(
		t, ctx, pool, ledger.ID,
		int64(initialTaskLedgerVersion+15), 3, int64(initialTaskLedgerVersion+15),
	)
}

func taskLedgerMutationCommandID(t *testing.T, marker, label string) taskstate.CommandID {
	t.Helper()
	id, err := taskstate.NewCommandID(marker, label)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func taskLedgerMutationRef(uri string, relation taskstate.RefRelation, digit string) taskstate.Ref {
	return taskstate.Ref{URI: uri, Version: "1", Hash: strings.Repeat(digit, 64), Relation: relation}
}

func taskLedgerMutationExpandedMetadata(t *testing.T) taskstate.JSONObject {
	t.Helper()
	fields := make(map[string]int, 6000)
	for index := 0; index < 6000; index++ {
		fields[fmt.Sprintf("k%d", index)] = 0
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := taskstate.NewJSONObject(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata.Bytes()) >= taskstate.MaxJSONObjectBytes {
		t.Fatalf("expanded metadata fixture is not within the canonical bound: %d", len(metadata.Bytes()))
	}
	return metadata
}

func assertTaskLedgerMutationPhase(
	t *testing.T, ctx context.Context, repository *Repository, jobID int64, wantVersion uint64,
	assert func(taskstate.MaterializedState),
) {
	t.Helper()
	ledger, err := repository.TaskLedger(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	state := ledger
	if state.Version != wantVersion {
		t.Fatalf("normalized ledger version=%d, want %d", state.Version, wantVersion)
	}
	assert(state)
}

func taskLedgerMutationEntries(entries []taskstate.Entry) map[taskstate.EntryID]taskstate.Entry {
	result := make(map[taskstate.EntryID]taskstate.Entry, len(entries))
	for _, entry := range entries {
		result[entry.ID] = entry
	}
	return result
}

func taskLedgerMutationNodes(nodes []taskstate.Node) map[taskstate.NodeID]taskstate.Node {
	result := make(map[taskstate.NodeID]taskstate.Node, len(nodes))
	for _, node := range nodes {
		result[node.ID] = node
	}
	return result
}
