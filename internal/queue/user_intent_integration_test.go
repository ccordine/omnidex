package queue

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUserIntentContentPreservesOriginalThroughPersistenceAndReplay(t *testing.T) {
	pool, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Preserve the original owner message.")
	original := "  " + strings.Repeat("Keep this exact line: café 世界.\n", 1200) + "\"quoted\"  "
	request, result := saveIntent(t, repository, command, fixture.created.Revision, original,
		statePatch("/message", sessionintent.Content{Text: "a shortened model copy", Recipient: "planner"}))
	var stored sessionintent.Result
	var raw []byte
	if err := pool.QueryRow(t.Context(), `SELECT result_json FROM channel_user_intents WHERE operation_id=$1`, request.OperationID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []sessionintent.Result{result, stored} {
		var content sessionintent.Content
		if len(candidate.Patch) != 1 || json.Unmarshal(candidate.Patch[0].Value, &content) != nil || content.Text != original || content.Recipient != "planner" {
			t.Fatal("original content was not bound before persistence")
		}
	}
	replay, err := repository.InterpretUserIntent(t.Context(), request, func(context.Context, sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
		t.Fatal("replay invoked interpretation")
		return sessionintent.Result{}, nil, nil
	})
	if err != nil || !reflect.DeepEqual(stored, replay) {
		t.Fatalf("replay changed original content: %v", err)
	}
}

func TestUserIntentApprovalStartsExactRevisionAndReplays(t *testing.T) {
	for _, goal := range []string{"Present a bounded text report.", "Record one independently supplied value."} {
		t.Run(goal, func(t *testing.T) {
			pool, repository := freshUserIntentRepository(t)
			command, fixture := userIntentFixture(t, repository, goal)
			request, result := saveIntent(t, repository, command, fixture.created.Revision, "Looks good, lock that in and go.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"), statePatch("/job_status", "running"))
			if result.RequiresApproval {
				t.Fatal("approval of a displayed plan asked for recursive approval")
			}
			before, _, err := repository.CurrentCodingLedgerForJob(context.Background(), fixture.created.JobID)
			if err != nil || before.State != "proposed" {
				t.Fatal("interpretation mutated ledger")
			}
			receipt, err := repository.ExecuteUserIntent(context.Background(), request.OperationID, command.ChannelID, command.WorkspaceIdentity)
			if err != nil || receipt.Status != model.JobStatusRunning || receipt.Revision != fixture.created.Revision+2 {
				t.Fatalf("receipt=%+v error=%v", receipt, err)
			}
			again, err := repository.ExecuteUserIntent(context.Background(), request.OperationID, command.ChannelID, command.WorkspaceIdentity)
			if err != nil || !reflect.DeepEqual(receipt, again) {
				t.Fatalf("receipt replay changed: %+v %v", again, err)
			}
			calls := 0
			replay, err := repository.InterpretUserIntent(context.Background(), request, func(context.Context, sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
				calls++
				return sessionintent.Result{}, nil, nil
			})
			if err != nil || calls != 0 || !reflect.DeepEqual(result, replay) {
				t.Fatalf("interpretation replay called model: %+v %v", replay, err)
			}
			var exact string
			if err := pool.QueryRow(context.Background(), `SELECT exact_text FROM coding_ledger_owner_turns WHERE ledger_id=$1 AND action='sign_off'`, fixture.created.LedgerID).Scan(&exact); err != nil || exact != request.Text {
				t.Fatalf("lost owner authority %q %v", exact, err)
			}
		})
	}
}
func TestUserIntentRemovalPitchAndSingleApproval(t *testing.T) {
	_, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Keep only the requested local task.")
	request, result := saveIntent(t, repository, command, fixture.created.Revision, "remove that", statePatch("/items/0/disposition", "tombstoned"))
	if !result.RequiresApproval || result.Pitch == "" {
		t.Fatalf("missing action pitch: %+v", result)
	}
	if _, err := repository.ExecuteUserIntent(context.Background(), request.OperationID, command.ChannelID, command.WorkspaceIdentity); err == nil {
		t.Fatal("unapproved proposal executed")
	}
	approval, approved := saveIntent(t, repository, command, fixture.created.Revision, "yes, do that", statePatch("/approval", true))
	if approved.RequiresApproval {
		t.Fatal("approval requested another approval")
	}
	receipt, err := repository.ExecuteUserIntent(context.Background(), approval.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	ledger, _, err := repository.CurrentCodingLedgerForJob(context.Background(), fixture.created.JobID)
	if err != nil || ledger.Ledger.Tasks[0].Disposition != model.LedgerDispositionTombstoned || receipt.Revision != fixture.created.Revision+1 {
		t.Fatalf("removal=%+v error=%v", ledger, err)
	}
}
func TestUserIntentRejectsStaleExecutionAndOtherActor(t *testing.T) {
	_, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Preserve exact approval state.")
	stale, _ := saveIntent(t, repository, command, fixture.created.Revision, "Start this version.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
	first, _ := saveIntent(t, repository, command, fixture.created.Revision, "Approve this version.", statePatch("/items/0/review_state", "agreed"))
	if _, err := repository.ExecuteUserIntent(context.Background(), first.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ExecuteUserIntent(context.Background(), stale.OperationID, command.ChannelID, command.WorkspaceIdentity); !errors.Is(err, ErrUserIntentStale) {
		t.Fatalf("stale execution=%v", err)
	}
	current, _, _ := repository.CurrentCodingLedgerForJob(context.Background(), fixture.created.JobID)
	if current.State != "proposed" {
		t.Fatal("stale start froze changed plan")
	}
	if _, err := repository.ExecuteUserIntent(context.Background(), first.OperationID, command.ChannelID, "client_22222222222222222222222222222222_directory_1_202"); err == nil {
		t.Fatal("another actor replayed owner intent")
	}
}
func TestUserIntentContextKeepsOnlyRecentExchangesAndPendingPitch(t *testing.T) {
	_, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Discuss a concrete selected task.")
	first, result := saveIntent(t, repository, command, fixture.created.Revision, "Remove this task.", statePatch("/items/0/disposition", "tombstoned"))
	state, err := repository.ChannelSessionState(context.Background(), command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := model.NewLifecycleOperationID()
	_, err = repository.InterpretUserIntent(context.Background(), sessionintent.Request{OperationID: id, Text: "which one?", State: state, LedgerRevision: fixture.created.Revision}, func(_ context.Context, c sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
		if len(c.Recent) > 4 || c.Pending == nil || c.Pending.ID != first.OperationID || c.Pending.Pitch != result.Pitch || len(c.Facts) == 0 {
			t.Fatalf("context=%+v", c)
		}
		return sessionintent.Result{OperationID: id, Patch: []userintent.Patch{}, Clarification: "The displayed task."}, json.RawMessage(`[]`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestUserIntentSharedTypedLedgerControlsStillCommit(t *testing.T) {
	_, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Keep typed and conversational approval on the same ledger.")
	mutation := model.LedgerMutation{OperationID: "typed-review", OwnerTurnID: "typed-owner", BaseRevision: fixture.created.Revision, Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: "task", ReviewState: model.LedgerReviewAgreed, Rationale: "Explicit typed owner approval."}}, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"})}
	agreed, err := repository.ApplyCodingLedgerControl(context.Background(), ApplyCodingLedgerMutationCommand{LedgerID: fixture.created.LedgerID, OwnerTurnText: "Approve the shown task.", Mutation: mutation})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := repository.FreezeCodingLedger(context.Background(), FreezeCodingLedgerCommand{LedgerID: fixture.created.LedgerID, Revision: agreed.Revision, OperationID: "typed-freeze", OwnerTurnID: "typed-start-owner", OwnerTurnText: "Start the approved plan."})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteCodingLedgerReview(context.Background(), fixture.created.JobID, fixture.created.LedgerID, frozen.Revision); err != nil {
		t.Fatal(err)
	}
	state, err := repository.ChannelSessionState(context.Background(), command.ChannelID, command.WorkspaceIdentity)
	if err != nil || state.LatestJob.Status != model.JobStatusRunning {
		t.Fatalf("typed controls failed: %+v %v", state, err)
	}
}
func TestUserIntentParserFailurePersistsAndNeverExecutes(t *testing.T) {
	pool, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Preserve authority after a malformed semantic result.")
	state, err := repository.ChannelSessionState(context.Background(), command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := model.NewLifecycleOperationID()
	request := sessionintent.Request{OperationID: id, Text: "yes", State: state, LedgerRevision: fixture.created.Revision}
	failed := errors.New("unavailable opaque semantic choice")
	if _, err := repository.InterpretUserIntent(context.Background(), request, func(context.Context, sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
		return sessionintent.Result{}, json.RawMessage(`[{"model":"fixture","response":"invalid"}]`), failed
	}); !errors.Is(err, failed) {
		t.Fatalf("parser failure=%v", err)
	}
	calls := 0
	if _, err := repository.InterpretUserIntent(context.Background(), request, func(context.Context, sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
		calls++
		return sessionintent.Result{}, nil, nil
	}); err == nil || calls != 0 {
		t.Fatal("failed interpretation retried inference or became valid")
	}
	if _, err := repository.ExecuteUserIntent(context.Background(), id, command.ChannelID, command.WorkspaceIdentity); err == nil {
		t.Fatal("failed parser output executed")
	}
	var stored string
	if err := pool.QueryRow(context.Background(), `SELECT result_json->>'error' FROM channel_user_intents WHERE operation_id=$1`, id).Scan(&stored); err != nil || stored != failed.Error() {
		t.Fatalf("missing failure provenance %q %v", stored, err)
	}
}

func TestUserIntentInterruptionNeverRestartsInferenceOrEffects(t *testing.T) {
	_, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Preserve cancellation authority.")
	request, _ := saveIntent(t, repository, command, fixture.created.Revision, "Approve this plan.", statePatch("/items/0/review_state", "agreed"))
	canceled, err := repository.CancelUserIntent(context.Background(), request)
	if err != nil || !canceled.Interrupted {
		t.Fatalf("cancellation=%+v error=%v", canceled, err)
	}
	replay, err := repository.ExecuteUserIntent(context.Background(), request.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !replay.Interrupted {
		t.Fatalf("revoked action ran: %+v %v", replay, err)
	}
	current, _, err := repository.CurrentCodingLedgerForJob(context.Background(), fixture.created.JobID)
	if err != nil || current.Ledger.Tasks[0].ReviewState != model.LedgerReviewProposed {
		t.Fatal("canceled message changed the ledger")
	}
	id, _ := model.NewLifecycleOperationID()
	request.OperationID = id
	if _, err := repository.CancelUserIntent(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	called := false
	if _, err := repository.InterpretUserIntent(context.Background(), request, func(context.Context, sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
		called = true
		return sessionintent.Result{}, nil, nil
	}); err == nil || called {
		t.Fatal("canceled unseen message restarted inference")
	}
}

// Changed envelopes are rejected against ordinary persisted synthetic state;
// no historical record or authoritative state is patched by this fixture.
func TestUserIntentExactContextBindings(t *testing.T) {
	pool, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Bind current action context.")
	ctx := t.Context()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	bound, err := userIntentContextTx(ctx, tx, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*sessionintent.Context){
		"job":            func(c *sessionintent.Context) { c.JobID++ },
		"generation":     func(c *sessionintent.Context) { c.Generation++ },
		"ledger":         func(c *sessionintent.Context) { c.LedgerID++ },
		"revision":       func(c *sessionintent.Context) { c.Revision++ },
		"plan":           func(c *sessionintent.Context) { c.PlanState = "frozen" },
		"planning":       func(c *sessionintent.Context) { c.Planning = !c.Planning },
		"native prompt":  func(c *sessionintent.Context) { c.Prompt = "old question" },
		"native choices": func(c *sessionintent.Context) { c.NativeChoices = []string{"obsolete"} },
		"native input":   func(c *sessionintent.Context) { id := int64(999); c.InputID = &id },
		"workspace": func(c *sessionintent.Context) {
			c.State.WorkspaceIdentity = "client_22222222222222222222222222222222_directory_1_202"
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			c := bound
			change(&c)
			// A savepoint prevents a deliberately absent job lookup from affecting peers.
			child, err := tx.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer child.Rollback(ctx)
			if _, err := lockUserIntentContextTx(ctx, child, c); err == nil {
				t.Fatal("changed action context accepted")
			}
		})
	}
	current, _, err := repository.CurrentCodingLedgerForJob(ctx, fixture.created.JobID)
	if err != nil || current.Revision != fixture.created.Revision || current.State != "proposed" {
		t.Fatalf("context checks changed ledger: %+v %v", current, err)
	}
}

func TestUserIntentOperationAndStaleIntakeRejectBeforeInference(t *testing.T) {
	_, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Keep exact operation authority.")
	request, _ := saveIntent(t, repository, command, fixture.created.Revision, "Status.", statePatch("/status_visible", true))
	changed := request
	changed.Text = "Start."
	calls := 0
	interpret := func(context.Context, sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
		calls++
		return sessionintent.Result{}, nil, nil
	}
	if _, err := repository.InterpretUserIntent(t.Context(), changed, interpret); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("changed operation: %v", err)
	}
	changed = request
	changed.OperationID, _ = model.NewLifecycleOperationID()
	changed.State.LatestJob.Generation++
	if _, err := repository.InterpretUserIntent(t.Context(), changed, interpret); !errors.Is(err, ErrUserIntentStale) {
		t.Fatalf("stale intake: %v", err)
	}
	if calls != 0 {
		t.Fatal("rejected authority invoked inference")
	}
}

func TestUserIntentRejectsCommittedChannelTurnIdentity(t *testing.T) {
	_, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Present a bounded record.")
	state, err := repository.ChannelSessionState(context.Background(), command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	request := sessionintent.Request{OperationID: command.OperationID, Text: "Cancel the job.", State: state, LedgerRevision: fixture.created.Revision}
	called := false
	_, err = repository.InterpretUserIntent(context.Background(), request, func(context.Context, sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
		called = true
		return sessionintent.Result{}, nil, nil
	})
	if !errors.Is(err, ErrLifecycleOperationConflict) || called {
		t.Fatalf("collision error=%v inference=%v", err, called)
	}
	if _, err := repository.CancelUserIntent(context.Background(), request); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("cancellation reused turn identity: %v", err)
	}
	after, err := repository.ChannelSessionState(context.Background(), command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !state.Equal(after) {
		t.Fatalf("collision changed session: %+v %v", after, err)
	}
}

// Forward effects share the identity lock with semantic intake, including while
// the competing interpretation has not yet persisted its result.
func TestUserIntentForwardSerializesChildIdentity(t *testing.T) {
	pool, repository := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repository, "Keep message and interpretation identities exclusive.")
	delta := statePatch("/message", sessionintent.Content{Text: "Explain this plan.", Recipient: "planner"})
	request, _ := saveIntent(t, repository, command, fixture.created.Revision, "Explain this plan.", delta)
	digest := sha256.Sum256([]byte(string(request.OperationID) + "\x00message"))
	competing := request
	competing.OperationID = model.LifecycleOperationID(fmt.Sprintf("lifecycle_operation_%x", digest))
	competing.Text = "Show current status."
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	go func() {
		_, err := repository.InterpretUserIntent(ctx, competing, func(ctx context.Context, _ sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return sessionintent.Result{}, nil, ctx.Err()
			}
			return sessionintent.Result{OperationID: competing.OperationID, Patch: []userintent.Patch{statePatch("/status_visible", true)}}, json.RawMessage(`[{"model":"fixture"}]`), nil
		})
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("competing interpretation failed: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	blocked, stop := context.WithTimeout(ctx, 250*time.Millisecond)
	_, err := repository.ExecuteUserIntent(blocked, request.OperationID, command.ChannelID, command.WorkspaceIdentity)
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("forward bypassed in-flight identity lock: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	receipt, err := repository.ExecuteUserIntent(ctx, competing.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ExecuteUserIntent(ctx, request.OperationID, command.ChannelID, command.WorkspaceIdentity); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("forward reused committed intent identity: %v", err)
	}
	replay, err := repository.ExecuteUserIntent(ctx, competing.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !reflect.DeepEqual(receipt, replay) {
		t.Fatalf("status receipt replay changed: %+v %v", replay, err)
	}
	var effects int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM lifecycle_operation_registry WHERE operation_id=$1`, competing.OperationID).Scan(&effects); err != nil || effects != 0 {
		t.Fatalf("colliding forward committed effects=%d: %v", effects, err)
	}
	after, err := repository.ChannelSessionState(ctx, command.ChannelID, command.WorkspaceIdentity)
	if err != nil || !request.State.Equal(after) {
		t.Fatalf("collision changed session: %+v %v", after, err)
	}
}
