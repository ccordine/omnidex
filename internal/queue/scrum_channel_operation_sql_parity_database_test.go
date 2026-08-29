package queue

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func TestPostgresScrumChannelCommandMatchesGoBytesDigestAndBounds(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	bundle := loadMigrationBundleThroughPrefix(t, "090")
	if err := repository.EnsureSchema(t.Context(), bundle); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(t.Context(), bundle); err != nil {
		t.Fatalf("retry operation receipt migration bundle: %v", err)
	}
	assertAppliedMigrationCount(t, pool, "090_scrum_channel_operation_receipts.sql", 1)
	operationID, err := NewLifecycleOperationID("scrum-channel-sql-parity")
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range []string{
		"<script>&\u2028\u2029\"\\\n",
		`\u003cscript\u003e\u0026\u2028`,
		strings.Repeat("x", model.MaxFreeFormTurnBytes),
		"\ufeff",
	} {
		descriptor, err := describeScrumChannelOperation(ScrumChannelOperationRequest{
			OperationID: operationID, ProjectID: 4, CardID: "card-4", Message: message,
		})
		if err != nil {
			t.Fatal(err)
		}
		var rendered, digest string
		var valid bool
		if err := pool.QueryRow(t.Context(), `
			SELECT scrum_channel_command_text($1::jsonb),
			       scrum_channel_command_sha256($1::jsonb),
			       scrum_valid_channel_command($1::jsonb)
		`, string(descriptor.Payload)).Scan(&rendered, &digest, &valid); err != nil {
			t.Fatal(err)
		}
		if !valid || rendered != string(descriptor.Payload) || digest != descriptor.SHA256 {
			t.Fatalf("SQL command parity valid=%t rendered=%q want=%q digest=%q want=%q",
				valid, rendered, descriptor.Payload, digest, descriptor.SHA256)
		}
	}

	assertPostgresScrumCommandValidity(t, pool, operationID, strings.Repeat("x", model.MaxFreeFormTurnBytes+1), false)
	unicodeWhiteSpace := []rune{
		'\u0009', '\u000a', '\u000b', '\u000c', '\u000d', '\u0020', '\u0085', '\u00a0',
		'\u1680', '\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006',
		'\u2007', '\u2008', '\u2009', '\u200a', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000',
	}
	for _, whitespace := range unicodeWhiteSpace {
		t.Run(fmt.Sprintf("U+%04X", whitespace), func(t *testing.T) {
			assertPostgresScrumCommandValidity(t, pool, operationID, string(whitespace), false)
		})
	}
}

func TestPostgresScrumOperationFunctionsIgnorePGTempShadows(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "090")); err != nil {
		t.Fatal(err)
	}
	project, card := newScrumChannelOperationCard(t, repository, "operation-shadow")
	request := ScrumChannelOperationRequest{
		OperationID: testLifecycleOperationID(t, "operation-shadow", project.ID),
		ProjectID:   project.ID, CardID: card.ID, Message: "Use only runtime-schema authority.",
	}
	command, descriptor, err := normalizeScrumChannelOperation(ScrumChannelOperationCommand{
		Request: request, ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect:       ScrumChannelEffect{Kind: ScrumChannelStartJob, Instruction: request.Message},
		ResultAction: "started",
	})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	var runtimeSchema string
	if err := conn.QueryRow(t.Context(), `SELECT current_schema()`).Scan(&runtimeSchema); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	job := stageScrumStartAuthorityForShadowTest(t, tx, card, command, descriptor)
	if _, err := tx.Exec(t.Context(), `
		CREATE TEMP TABLE lifecycle_operation_registry(operation_id text,kind text,command_sha256 text,command_payload jsonb);
		CREATE TEMP TABLE scrum_cards(project_id bigint,id text);
		CREATE TEMP TABLE scrum_card_messages(project_id bigint,card_id text,operation_id text);
		CREATE TEMP TABLE jobs(id bigint,project_id bigint,pipeline text,metadata jsonb,instruction text);
		CREATE TEMP TABLE job_lifecycle_operations(operation_id text,kind text,job_id bigint,command_payload jsonb);
		CREATE TEMP TABLE scrum_channel_operations(operation_id text);
	`); err != nil {
		t.Fatal(err)
	}
	runtimeOperations := pgx.Identifier{runtimeSchema, "scrum_channel_operations"}.Sanitize()
	if _, err := tx.Exec(t.Context(), `INSERT INTO `+runtimeOperations+`(
		operation_id,project_id,card_id,effect_kind,effect_operation_id,job_id,result_action
	) VALUES($1,$2,$3,'start_job',$1,$4,'started')`,
		request.OperationID, project.ID, card.ID, job.ID,
	); err != nil {
		t.Fatalf("pinned operation trigger resolved pg_temp shadow: %v", err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatalf("commit pinned operation receipt: %v", err)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT COUNT(*) FROM scrum_channel_operations WHERE operation_id=$1`, request.OperationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("runtime operation receipts=%d want=1", count)
	}
}

func stageScrumStartAuthorityForShadowTest(
	t *testing.T,
	tx pgx.Tx,
	card DBScrumCard,
	command ScrumChannelOperationCommand,
	descriptor scrumChannelOperationDescriptor,
) model.Job {
	t.Helper()
	ctx := t.Context()
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.Request.OperationID); err != nil {
		t.Fatal(err)
	}
	created, err := reserveLifecycleOperationIdentityTx(
		ctx, tx, descriptor.Request.OperationID, LifecycleScrumChannel, descriptor.SHA256, descriptor.Payload,
	)
	if err != nil || !created {
		t.Fatalf("reserve shadow-test operation created=%t error=%v", created, err)
	}
	current, err := lockScrumCardTx(ctx, tx, card.ProjectID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	metadata, _, err := scrumPlayAuthorityTx(ctx, tx, current)
	if err != nil {
		t.Fatal(err)
	}
	metadata.ChannelOrigin = true
	metadata.ChannelOperationID = string(command.Request.OperationID)
	metadata.ReturnColumn = current.Column
	if err := metadata.Validate(); err != nil {
		t.Fatal(err)
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	job := seedPreInlineExecutionMigrationJobTx(
		t, ctx, tx, command.Request.Message, model.PipelineScrum, "v3_coding", metadataJSON,
	).Job
	if _, err := tx.Exec(ctx, `
		UPDATE scrum_cards SET column_name='in_progress',play_state='running',
		 job_id=$3::text,sync_job_id=$3::text,
		 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2
	`, card.ProjectID, card.ID, strconv.FormatInt(job.ID, 10)); err != nil {
		t.Fatal(err)
	}
	if _, err := insertScrumCardMessageTx(ctx, tx, card.ProjectID, card.ID, ScrumCardMessageAppend{
		ID: "message-" + string(command.Request.OperationID), Role: "user",
		Content: command.Request.Message, OperationID: string(command.Request.OperationID),
	}); err != nil {
		t.Fatal(err)
	}
	return job
}

func assertPostgresScrumCommandValidity(
	t *testing.T,
	pool scrumChannelQueryRower,
	operationID LifecycleOperationID,
	message string,
	want bool,
) {
	t.Helper()
	payload, err := json.Marshal(ScrumChannelOperationRequest{
		OperationID: operationID, ProjectID: 4, CardID: "card-4", Message: message,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got bool
	if err := pool.QueryRow(t.Context(), `SELECT scrum_valid_channel_command($1::jsonb)`, string(payload)).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("message bytes=%d validity=%t want=%t", len(message), got, want)
	}
}
