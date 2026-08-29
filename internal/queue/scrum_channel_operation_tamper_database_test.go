package queue

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

type stagedScrumStartReceipt struct {
	tx         pgx.Tx
	command    ScrumChannelOperationCommand
	descriptor scrumChannelOperationDescriptor
	card       DBScrumCard
	jobID      int64
}

type forgedScrumReceipt struct {
	effectKind ScrumChannelEffectKind
	effectID   LifecycleOperationID
	jobID      int64
	action     string
}

func TestPostgresScrumChannelOperationRejectsForgedCausalReceipt(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		wantError string
	}{
		{name: "wrong deterministic effect", wantError: "effect identity is not derived"},
		{name: "missing deterministic effect", wantError: "lacks exact lifecycle effect"},
		{name: "wrong job", wantError: "lacks exact project/card job relationship"},
		{name: "wrong action", wantError: "lacks exact job origin"},
		{name: "missing bound message", wantError: "lacks exact bound user message"},
		{name: "missing channel operation ID", wantError: "lacks exact job origin"},
		{name: "non-channel origin with operation ID", wantError: "lacks exact job origin"},
		{name: "wrong registry digest", wantError: "registry payload or digest differs"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repository, pool, ctx := scrumChannelOperationTestRepository(t)
			project, card := newScrumChannelOperationCard(t, repository, "tamper-"+strings.ReplaceAll(testCase.name, " ", "-"))
			request := ScrumChannelOperationRequest{
				OperationID: testLifecycleOperationID(t, "tamper-"+testCase.name, project.ID),
				ProjectID:   project.ID, CardID: card.ID, Message: "Apply only this canonical command.",
			}
			boundMessage := testCase.name != "missing bound message"
			digest := ""
			if testCase.name == "wrong registry digest" {
				digest = strings.Repeat("0", 64)
			}
			staged := stageCanonicalScrumStartReceipt(
				t, repository, ctx, card, request, digest, boundMessage,
			)
			defer func() { _ = staged.tx.Rollback(ctx) }()

			receipt := forgedScrumReceipt{
				effectKind: ScrumChannelStartJob,
				effectID:   request.OperationID,
				jobID:      staged.jobID,
				action:     "started",
			}
			switch testCase.name {
			case "wrong deterministic effect", "missing deterministic effect":
				effectCommand := staged.command
				effectCommand.Effect = ScrumChannelEffect{Kind: ScrumChannelReplanJob, JobID: staged.jobID}
				effectCommand.ResultAction = "replanned"
				receipt.effectKind, receipt.action = ScrumChannelReplanJob, "replanned"
				if testCase.name == "wrong deterministic effect" {
					receipt.effectID = testLifecycleOperationID(t, "unrelated-effect", staged.jobID)
				} else {
					var err error
					receipt.effectID, err = scrumChannelEffectOperationID(effectCommand)
					if err != nil {
						t.Fatal(err)
					}
				}
			case "wrong job":
				metadata, _, err := scrumPlayAuthorityTx(ctx, staged.tx, staged.card)
				if err != nil {
					t.Fatal(err)
				}
				metadata.ChannelOrigin = true
				metadata.ChannelOperationID = string(testLifecycleOperationID(t, "other-channel-command", staged.jobID))
				metadata.ReturnColumn = staged.card.Column
				wrongJob, err := repository.executeScrumChannelEffectTx(
					ctx, staged.tx, staged.command, staged.card, metadata,
				)
				if err != nil {
					t.Fatal(err)
				}
				receipt.jobID = wrongJob.ID
			case "wrong action":
				receipt.action = "feedback"
			case "missing channel operation ID":
				if _, err := staged.tx.Exec(ctx, `
					UPDATE jobs SET metadata=jsonb_set(
					 metadata,'{scrum_channel_operation_id}',to_jsonb($2::text),false
					) WHERE id=$1
				`, staged.jobID, ""); err != nil {
					t.Fatal(err)
				}
			case "non-channel origin with operation ID":
				if _, err := staged.tx.Exec(ctx, `
					UPDATE jobs SET metadata=jsonb_set(
					 metadata,'{scrum_channel_origin}','false'::jsonb,false
					) WHERE id=$1
				`, staged.jobID); err != nil {
					t.Fatal(err)
				}
			}

			insertErr := insertForgedScrumReceipt(ctx, staged, receipt)
			if insertErr == nil {
				insertErr = staged.tx.Commit(ctx)
			} else {
				_ = staged.tx.Rollback(ctx)
			}
			if insertErr == nil || !strings.Contains(insertErr.Error(), testCase.wantError) {
				t.Fatalf("forged receipt error=%v, want %q", insertErr, testCase.wantError)
			}
			assertRejectedScrumReceiptLeftNoState(t, pool, request)
		})
	}
}

func TestPostgresScrumChannelOperationRejectsOrphanRegistryIdentity(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, card := newScrumChannelOperationCard(t, repository, "orphan-registry")
	request := ScrumChannelOperationRequest{
		OperationID: testLifecycleOperationID(t, "orphan-scrum-registry", project.ID),
		ProjectID:   project.ID, CardID: card.ID, Message: "Do not leave an orphan identity.",
	}
	descriptor, err := describeScrumChannelOperation(request)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	created, err := reserveLifecycleOperationIdentityTx(
		ctx, tx, request.OperationID, LifecycleScrumChannel, descriptor.SHA256, descriptor.Payload,
	)
	if err != nil || !created {
		t.Fatalf("reserve orphan identity created=%t error=%v", created, err)
	}
	err = tx.Commit(ctx)
	if err == nil || !strings.Contains(err.Error(), "lacks immutable operation") {
		t.Fatalf("orphan registry commit error=%v", err)
	}
	assertRejectedScrumReceiptLeftNoState(t, pool, request)
}

func stageCanonicalScrumStartReceipt(
	t *testing.T,
	repository *Repository,
	ctx context.Context,
	card DBScrumCard,
	request ScrumChannelOperationRequest,
	digestOverride string,
	appendBoundMessage bool,
) stagedScrumStartReceipt {
	t.Helper()
	command, descriptor, err := normalizeScrumChannelOperation(ScrumChannelOperationCommand{
		Request: request, ExpectedCardUpdatedAt: card.UpdatedAt,
		Effect:       ScrumChannelEffect{Kind: ScrumChannelStartJob, Instruction: request.Message},
		ResultAction: "started",
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback(ctx)
		}
	}()
	if err := lockLifecycleOperationIdentityTx(ctx, tx, request.OperationID); err != nil {
		t.Fatal(err)
	}
	digest := descriptor.SHA256
	if digestOverride != "" {
		digest = digestOverride
	}
	created, err := reserveLifecycleOperationIdentityTx(
		ctx, tx, request.OperationID, LifecycleScrumChannel, digest, descriptor.Payload,
	)
	if err != nil || !created {
		t.Fatalf("reserve staged identity created=%t error=%v", created, err)
	}
	current, err := lockScrumCardTx(ctx, tx, request.ProjectID, request.CardID)
	if err != nil {
		t.Fatal(err)
	}
	metadata, _, err := scrumPlayAuthorityTx(ctx, tx, current)
	if err != nil {
		t.Fatal(err)
	}
	metadata.ChannelOrigin = true
	metadata.ChannelOperationID = string(request.OperationID)
	metadata.ReturnColumn = current.Column
	job, err := repository.executeScrumChannelEffectTx(ctx, tx, command, current, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE scrum_cards SET column_name='in_progress',play_state='running',
		 job_id=$3::text,sync_job_id=$3::text,
		 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2
	`, current.ProjectID, current.ID, strconv.FormatInt(job.ID, 10)); err != nil {
		t.Fatal(err)
	}
	if appendBoundMessage {
		_, err = insertScrumCardMessageTx(ctx, tx, request.ProjectID, request.CardID, ScrumCardMessageAppend{
			ID: "message-" + string(request.OperationID), Role: "user", Content: request.Message,
			OperationID: string(request.OperationID),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	rollback = false
	return stagedScrumStartReceipt{
		tx: tx, command: command, descriptor: descriptor, card: current, jobID: job.ID,
	}
}

func insertForgedScrumReceipt(
	ctx context.Context,
	staged stagedScrumStartReceipt,
	receipt forgedScrumReceipt,
) error {
	_, err := staged.tx.Exec(ctx, `
		INSERT INTO scrum_channel_operations(
		 operation_id,project_id,card_id,effect_kind,effect_operation_id,job_id,result_action
		) VALUES($1,$2,$3,$4,$5,$6,$7)
	`, staged.descriptor.Request.OperationID, staged.descriptor.Request.ProjectID,
		staged.descriptor.Request.CardID, receipt.effectKind, receipt.effectID,
		receipt.jobID, receipt.action)
	return err
}

func assertRejectedScrumReceiptLeftNoState(
	t *testing.T,
	pool interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	request ScrumChannelOperationRequest,
) {
	t.Helper()
	var operations, registry, messages, jobs int
	if err := pool.QueryRow(t.Context(), `
		SELECT
		 (SELECT COUNT(*) FROM scrum_channel_operations WHERE operation_id=$1),
		 (SELECT COUNT(*) FROM lifecycle_operation_registry WHERE operation_id=$1),
		 (SELECT COUNT(*) FROM scrum_card_messages WHERE operation_id=$1),
		 (SELECT COUNT(*) FROM jobs WHERE project_id=$2)
	`, request.OperationID, request.ProjectID).Scan(&operations, &registry, &messages, &jobs); err != nil {
		t.Fatal(err)
	}
	if operations != 0 || registry != 0 || messages != 0 || jobs != 0 {
		t.Fatalf(
			"rejected receipt left operations=%d registry=%d messages=%d jobs=%d",
			operations, registry, messages, jobs,
		)
	}
}
