package queue

import (
	"context"
	"encoding/json"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
	"reflect"
	"testing"
)

func TestUserIntentPatchRollbackAndExactDelta(t *testing.T) {
	pool, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Apply exactly one approved review state.")
	before, _, err := repo.CurrentCodingLedgerForJob(ctx, fixture.created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := saveIntent(t, repo, command, before.Revision, "Approve this item.", statePatch("/items/0/review_state", "agreed"))
	// Fail the last write, after the ledger mutation, in this disposable schema.
	_, err = pool.Exec(ctx, `CREATE FUNCTION fail_intent_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture receipt failure'; END $$;
 CREATE TRIGGER fail_receipt BEFORE UPDATE ON channel_user_intents FOR EACH ROW EXECUTE FUNCTION fail_intent_receipt()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ExecuteUserIntent(ctx, request.OperationID, command.ChannelID, command.WorkspaceIdentity); err == nil {
		t.Fatal("failed receipt committed ledger")
	}
	rolled, _, err := repo.CurrentCodingLedgerForJob(ctx, fixture.created.JobID)
	if err != nil || !reflect.DeepEqual(before, rolled) {
		t.Fatal("transaction failed to roll back", err)
	}
	var receipt []byte
	if err := pool.QueryRow(ctx, `SELECT receipt_json FROM channel_user_intents WHERE operation_id=$1`, request.OperationID).Scan(&receipt); err != nil || receipt != nil {
		t.Fatal("partial receipt survived", err)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_receipt ON channel_user_intents; DROP FUNCTION fail_intent_receipt()`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExecuteUserIntent(ctx, request.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
		t.Fatal(err)
	}
	after, _, err := repo.CurrentCodingLedgerForJob(ctx, fixture.created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	wanted := before.Ledger.Tasks[0]
	wanted.ReviewState = model.LedgerReviewAgreed
	if !reflect.DeepEqual(wanted, after.Ledger.Tasks[0]) {
		t.Fatal("item changed outside proposed review delta")
	}
	if after.State != before.State || after.JobID != before.JobID || after.Revision != before.Revision+1 {
		t.Fatal("unexpected workflow changes")
	}
}

func TestUserIntentPatchUntrustedRepositoryAndStoredData(t *testing.T) {
	pool, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Preserve protected fields.")
	state, err := repo.ChannelSessionState(ctx, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := repo.CurrentCodingLedgerForJob(ctx, fixture.created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []userintent.Patch{statePatch("/authority/job_id", 999), statePatch("/authority/generation", 99), statePatch("/authority/revision", 9), statePatch("/items/0/text", "replace scope"), statePatch("/items/0/id", "foreign"), statePatch("/authority/state/workspace_identity", "foreign")} {
		id, _ := model.NewLifecycleOperationID()
		request := sessionintent.Request{OperationID: id, State: state, Text: "Proposed data.", LedgerRevision: fixture.created.Revision}
		_, err := repo.InterpretUserIntent(ctx, request, func(context.Context, sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
			return sessionintent.Result{OperationID: id, Patch: []userintent.Patch{p}}, json.RawMessage(`[]`), nil
		})
		if err == nil {
			t.Fatalf("untrusted patch accepted: %s", p.Path)
		}
	}
	request, _ := saveIntent(t, repo, command, before.Revision, "Status.", statePatch("/status_visible", true))
	raw, _ := json.Marshal([]userintent.Patch{statePatch("/authority/job_id", 999)})
	if _, err := pool.Exec(ctx, `UPDATE channel_user_intents SET result_json=jsonb_set(result_json,'{patch}',$2) WHERE operation_id=$1`, request.OperationID, raw); err == nil {
		t.Fatal("stored proposed provenance was mutable")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	saved, err := readUserIntentTx(ctx, tx, request.OperationID, command.ChannelID, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	saved.result.Patch = []userintent.Patch{statePatch("/authority/job_id", 999)}
	if err := validateStoredUserDelta(saved.current, saved); err == nil {
		t.Fatal("execution validation trusted proposed JSON")
	}
	after, _, err := repo.CurrentCodingLedgerForJob(ctx, fixture.created.JobID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("rejected proposals changed ledger", err)
	}
}

func TestUserIntentConfirmationCannotApproveReplacement(t *testing.T) {
	_, repo := freshUserIntentRepository(t)
	command, fixture := userIntentFixture(t, repo, "Confirm only the exact proposed delta.")
	first, _ := saveIntent(t, repo, command, fixture.created.Revision, "Remove it.", statePatch("/items/0/disposition", "tombstoned"))
	confirmation, _ := saveIntent(t, repo, command, fixture.created.Revision, "Yes.", statePatch("/approval", true))
	replacement, _ := saveIntent(t, repo, command, fixture.created.Revision, "Mark it for discussion instead.", statePatch("/items/0/review_state", "needs_discussion"))
	if replacement.OperationID == first.OperationID {
		t.Fatal("fixture identity collision")
	}
	if _, err := repo.ExecuteUserIntent(t.Context(), confirmation.OperationID, command.ChannelID, command.WorkspaceIdentity); err == nil {
		t.Fatal("old confirmation approved replacement")
	}
	ledger, _, err := repo.CurrentCodingLedgerForJob(t.Context(), fixture.created.JobID)
	if err != nil || ledger.Revision != fixture.created.Revision || ledger.Ledger.Tasks[0].Disposition != model.LedgerDispositionActive {
		t.Fatal("stale confirmation changed ledger", err)
	}
	decline, _ := saveIntent(t, repo, command, fixture.created.Revision, "No.", statePatch("/approval", false))
	if _, err := repo.ExecuteUserIntent(t.Context(), decline.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ExecuteUserIntent(t.Context(), replacement.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
		t.Fatal(err)
	} // immutable declined receipt
}
