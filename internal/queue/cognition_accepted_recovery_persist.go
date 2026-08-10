package queue

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/jackc/pgx/v5"
)

func insertAcceptedCognitionRecoveryTx(
	ctx context.Context,
	tx pgx.Tx,
	recovery cognitionruntime.AcceptedDecisionRecovery,
) error {
	authorityJSON, authoritySHA, err := acceptedCognitionRecoveryAuthorityJSON(recovery)
	if err != nil {
		return err
	}
	prepared := recovery.Prepared
	decisionSHA, err := cognitionruntime.DecisionSHA256(recovery.Decision)
	if err != nil {
		return err
	}
	source := recovery.SourceActor
	current := recovery.Binding.Attempt
	revision := prepared.Snapshot.CurrentRevision()
	projection := prepared.Snapshot.ContextProjection()
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_accepted_decision_recoveries (
			recovery_id,recovery_sha256,episode_id,job_id,generation,step_id,
			source_policy_call_id,source_attempt,source_worker_id,recovery_attempt,recovery_worker_id,
			snapshot_sha256,expected_revision,expected_revision_sha256,graph_version,graph_sha256,
			projection_id,obligation_node_id,decision_sha256,action_schema_id,
			action_schema_version,action_schema_sha256,authority_json,authority_json_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24)
	`, recovery.ID, recovery.SHA256, recovery.Binding.Episode.ID, current.JobID,
		current.Generation, current.StepID, recovery.SourcePolicyCallID, int64(source.Attempt),
		source.WorkerID, int64(current.Attempt), current.WorkerID, prepared.Snapshot.SHA256(),
		int64(revision.Number), revision.SHA256, int64(prepared.GraphVersion),
		prepared.ObligationGraph.SHA256, projection.ID, recovery.Decision.ObligationID,
		decisionSHA, recovery.ActionSchema.ID, recovery.ActionSchema.Version,
		recovery.ActionSchema.SHA256, string(authorityJSON), authoritySHA)
	if err != nil {
		return fmt.Errorf("insert accepted cognition recovery: %w", err)
	}
	return nil
}

func validateAcceptedCognitionRecoveryReplay(
	stored acceptedCognitionRecoveryRecord,
	recovery cognitionruntime.AcceptedDecisionRecovery,
) error {
	authorityJSON, authoritySHA, err := acceptedCognitionRecoveryAuthorityJSON(recovery)
	if err != nil {
		return err
	}
	if stored.ID != recovery.ID || stored.SHA256 != recovery.SHA256 ||
		stored.SourcePolicyCallID != recovery.SourcePolicyCallID ||
		stored.AuthorityJSONSHA256 != authoritySHA || !bytes.Equal(stored.AuthorityJSON, authorityJSON) {
		return fmt.Errorf("%w: accepted decision recovery replay changed authority", ErrCognitionConflict)
	}
	return nil
}

func acceptedCognitionRecoveryAuthorityJSON(
	recovery cognitionruntime.AcceptedDecisionRecovery,
) ([]byte, string, error) {
	decisionSHA, err := cognitionruntime.DecisionSHA256(recovery.Decision)
	if err != nil {
		return nil, "", err
	}
	return cognitionJSON(struct {
		Schema, ID, SHA256, PolicyCallID, SnapshotSHA256, GraphSHA256, DecisionSHA256 string
		Binding                                                                       cognitionruntime.Binding
		SourceActor                                                                   cognition.AttemptRef
		GraphVersion                                                                  uint64
		Projection                                                                    cognition.ContextProjectionRef
		ActionSchema                                                                  cognition.ActionSchemaRef
	}{
		cognitionruntime.AcceptedDecisionRecoverySchemaV1, recovery.ID, recovery.SHA256,
		recovery.SourcePolicyCallID, recovery.Prepared.Snapshot.SHA256(),
		recovery.Prepared.ObligationGraph.SHA256, decisionSHA, recovery.Binding,
		recovery.SourceActor, recovery.Prepared.GraphVersion,
		recovery.Prepared.Snapshot.ContextProjection(), recovery.ActionSchema.Ref(),
	})
}

func cloneAcceptedCognitionRecovery(
	recovery cognitionruntime.AcceptedDecisionRecovery,
) cognitionruntime.AcceptedDecisionRecovery {
	copy := recovery
	copy.Prepared.ObligationGraph = recovery.Prepared.ObligationGraph.Clone()
	copy.Prepared.CompletionEvidenceRefs = append(
		[]cognition.EvidenceRef{}, recovery.Prepared.CompletionEvidenceRefs...,
	)
	copy.Decision = recovery.Decision.Clone()
	copy.ActionSchema = recovery.ActionSchema.Clone()
	if recovery.ExistingReconciliation != nil {
		replay := cognitionruntime.ReconciliationReplay{
			Command: recovery.ExistingReconciliation.Command.Clone(),
			Receipt: recovery.ExistingReconciliation.Receipt.Clone(),
		}
		copy.ExistingReconciliation = &replay
	}
	return copy
}
