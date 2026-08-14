package queue

import (
	"strings"
	"testing"
)

func TestPostgresScrumChannelOperationRejectsCrossProjectAndCardEffects(t *testing.T) {
	for _, scope := range []string{"project", "card"} {
		t.Run(scope, func(t *testing.T) {
			repository, pool, ctx := scrumChannelOperationTestRepository(t)
			project, target := newScrumChannelOperationCard(t, repository, "scope-target-"+scope)
			var wrongCard DBScrumCard
			if scope == "project" {
				_, wrongCard = newScrumChannelOperationCard(t, repository, "scope-wrong-project")
			} else {
				var err error
				wrongCard, err = repository.CreateScrumCard(
					ctx, project.ID, "wrong-scope-card", "Wrong scope", "", "assigned", nil, nil,
				)
				if err != nil {
					t.Fatal(err)
				}
			}
			wrongJob := enqueueScrumChannelJobForTest(t, repository, wrongCard, "Wrong scoped job.")
			request := ScrumChannelOperationRequest{
				OperationID: testLifecycleOperationID(t, "scope-forge-"+scope, project.ID),
				ProjectID:   project.ID, CardID: target.ID, Message: "Reject a cross-scope effect.",
			}
			effectCommand := ScrumChannelOperationCommand{
				Request: request, ExpectedCardUpdatedAt: target.UpdatedAt,
				Effect:       ScrumChannelEffect{Kind: ScrumChannelReplanJob, JobID: wrongJob.ID},
				ResultAction: "replanned",
			}
			effectID, err := scrumChannelEffectOperationID(effectCommand)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repository.ReplanJob(ctx, ReplanJobCommand{
				OperationID: effectID, JobID: wrongJob.ID, Feedback: request.Message,
			}); err != nil {
				t.Fatal(err)
			}
			staged := stageCanonicalScrumStartReceipt(t, repository, ctx, target, request, "", true)
			defer func() { _ = staged.tx.Rollback(ctx) }()
			err = insertForgedScrumReceipt(ctx, staged, forgedScrumReceipt{
				effectKind: ScrumChannelReplanJob, effectID: effectID,
				jobID: wrongJob.ID, action: "replanned",
			})
			if err == nil || !strings.Contains(err.Error(), "lacks exact project/card job relationship") {
				t.Fatalf("cross-%s effect error=%v", scope, err)
			}
			_ = staged.tx.Rollback(ctx)
			var receipts, identities, messages int
			if err := pool.QueryRow(ctx, `
				SELECT
				 (SELECT COUNT(*) FROM scrum_channel_operations WHERE operation_id=$1),
				 (SELECT COUNT(*) FROM lifecycle_operation_registry WHERE operation_id=$1),
				 (SELECT COUNT(*) FROM scrum_card_messages WHERE operation_id=$1)
			`, request.OperationID).Scan(&receipts, &identities, &messages); err != nil {
				t.Fatal(err)
			}
			if receipts != 0 || identities != 0 || messages != 0 {
				t.Fatalf("cross-%s rejection left receipt/identity/message=%d/%d/%d",
					scope, receipts, identities, messages)
			}
		})
	}
}
