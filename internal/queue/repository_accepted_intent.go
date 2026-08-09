package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

var ErrIntentArtifactRequiresAcceptedWriter = errors.New(
	"intent artifacts require the accepted intent writer",
)

func (r *Repository) WriteAcceptedIntentArtifact(
	ctx context.Context,
	envelope artifacts.Envelope,
) error {
	intent, err := validateAcceptedIntentEnvelope(envelope)
	if err != nil {
		return err
	}
	if err := validateTaskLedgerRequest(r, ctx, envelope.JobID); err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin accepted intent artifact write: %w", err)
	}
	defer tx.Rollback(ctx)
	generation, err := requireAcceptedIntentStepTx(ctx, tx, envelope.JobID, envelope.StepID)
	if err != nil {
		return err
	}
	existing, found, err := loadAcceptedIntentProjectionTx(ctx, tx, envelope.JobID, envelope.Payload)
	if err != nil {
		return err
	}
	if found {
		if existing.StepID != envelope.StepID || existing.JobGeneration != generation {
			return fmt.Errorf(
				"%w: accepted intent projection belongs to step %d generation %d",
				ErrStaleJobGeneration, existing.StepID, existing.JobGeneration,
			)
		}
		if err := verifyAcceptedIntentProjectionTx(ctx, tx, existing, intent); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, envelope.JobID, true)
	if err != nil {
		return err
	}
	if header.Generation != generation || header.Status != taskstate.LedgerActive {
		return fmt.Errorf(
			"%w: accepted intent observed generation %d with ledger generation %d status %q",
			ErrStaleJobGeneration, generation, header.Generation, header.Status,
		)
	}
	artifactID, payloadSHA, err := insertAcceptedIntentArtifactTx(ctx, tx, envelope)
	if err != nil {
		return err
	}
	projection, commands, err := buildAcceptedIntentProjection(acceptedIntentProjection{
		ArtifactID: artifactID, JobID: envelope.JobID, StepID: envelope.StepID,
		JobGeneration: generation, LedgerID: header.ID,
		PayloadSHA256: payloadSHA, LedgerStart: header.Version,
	}, intent)
	if err != nil {
		return err
	}
	for _, command := range commands {
		event, err := applyQueueOwnedTaskCommandTx(ctx, tx, envelope.JobID, generation, command)
		if err != nil {
			return fmt.Errorf("project accepted intent into task ledger: %w", err)
		}
		if event.Kind == taskstate.EventNodesReadied &&
			(len(event.NodeIDs) != 1 || event.NodeIDs[0] != projection.ObjectiveNodeID) {
			return fmt.Errorf(
				"%w: intent projection would ready nodes outside its objective",
				taskstate.ErrInvalidState,
			)
		}
	}
	if err := insertAcceptedIntentProjectionTx(ctx, tx, projection); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit accepted intent artifact: %w", err)
	}
	return nil
}

func validateAcceptedIntentEnvelope(
	envelope artifacts.Envelope,
) (artifacts.IntentArtifact, error) {
	var intent artifacts.IntentArtifact
	if err := envelope.Validate(); err != nil {
		return intent, err
	}
	if envelope.JobID <= 0 || envelope.StepID <= 0 {
		return intent, fmt.Errorf("accepted intent artifact requires positive job and step IDs")
	}
	if envelope.Kind != artifacts.KindIntent || envelope.Version != "1" {
		return intent, fmt.Errorf(
			"accepted intent writer requires intent version 1, received %q version %q",
			envelope.Kind, envelope.Version,
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&intent); err != nil {
		return intent, fmt.Errorf("decode accepted intent artifact: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return intent, fmt.Errorf("decode accepted intent artifact: trailing JSON value")
		}
		return intent, fmt.Errorf("decode accepted intent artifact trailing data: %w", err)
	}
	if err := intent.Validate(); err != nil {
		return intent, err
	}
	if len(intent.UnresolvedReferences) != 0 {
		return intent, fmt.Errorf("accepted intent artifact contains unresolved references")
	}
	canonical, err := json.Marshal(intent)
	if err != nil {
		return intent, fmt.Errorf("canonicalize accepted intent artifact: %w", err)
	}
	if !bytes.Equal(canonical, envelope.Payload) {
		return intent, fmt.Errorf("accepted intent artifact payload must use canonical typed encoding")
	}
	return intent, nil
}

func requireAcceptedIntentStepTx(
	ctx context.Context,
	tx pgx.Tx,
	jobID, stepID int64,
) (int64, error) {
	if err := requireRunningCurrentStepTx(ctx, tx, jobID, stepID); err != nil {
		return 0, err
	}
	var action string
	var generation int64
	if err := tx.QueryRow(ctx, `
		SELECT action, generation FROM job_steps
		WHERE job_id=$1 AND id=$2
	`, jobID, stepID).Scan(&action, &generation); err != nil {
		return 0, fmt.Errorf("read accepted intent step: %w", err)
	}
	if action != "v3_intent_parse" {
		return 0, fmt.Errorf(
			"accepted intent artifact requires v3_intent_parse step, received %q",
			action,
		)
	}
	return generation, nil
}

func verifyAcceptedIntentProjectionTx(
	ctx context.Context,
	tx pgx.Tx,
	persisted acceptedIntentProjection,
	intent artifacts.IntentArtifact,
) error {
	expected, _, err := buildAcceptedIntentProjection(acceptedIntentProjection{
		ArtifactID: persisted.ArtifactID, JobID: persisted.JobID,
		StepID: persisted.StepID, JobGeneration: persisted.JobGeneration,
		LedgerID: persisted.LedgerID, PayloadSHA256: persisted.PayloadSHA256,
		LedgerStart: persisted.LedgerStart,
	}, intent)
	if err != nil {
		return err
	}
	if expected.ObjectiveNodeID != persisted.ObjectiveNodeID ||
		expected.LedgerEnd != persisted.LedgerEnd ||
		!sameAcceptedIntentItems(expected.Items, persisted.Items) {
		return fmt.Errorf(
			"%w: accepted intent projection manifest is inconsistent",
			taskstate.ErrInvalidState,
		)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, persisted.JobID, true)
	if err != nil {
		return err
	}
	ledger, err := restoreTaskLedgerTx(ctx, tx, header)
	if err != nil {
		return err
	}
	objective, ok := ledger.Node(persisted.ObjectiveNodeID)
	if !ok || objective.Kind != taskstate.NodeObjective ||
		objective.Status != taskstate.NodeActive || objective.AssignedStepID != nil ||
		objective.CreatedStepID == nil || *objective.CreatedStepID != persisted.StepID ||
		objective.Title != intent.Objectives[0].Description ||
		objective.Priority != intent.Objectives[0].Priority ||
		!slices.Equal(objective.AcceptanceCriteria, intent.Objectives[0].AcceptanceCriteria) {
		return fmt.Errorf("%w: accepted intent objective authority is inconsistent", taskstate.ErrInvalidState)
	}
	for _, item := range persisted.Items {
		if item.Kind == "objective" {
			continue
		}
		entry, ok := ledger.Entry(item.EntryID)
		if !ok || entry.Status != taskstate.EntryActive ||
			entry.ScopeNodeID != persisted.ObjectiveNodeID ||
			entry.CreatedStepID == nil || *entry.CreatedStepID != persisted.StepID ||
			len(entry.Refs) != 1 || entry.Refs[0] != item.sourceRef() {
			return fmt.Errorf("%w: accepted intent %s authority is inconsistent", taskstate.ErrInvalidState, item.Kind)
		}
		switch item.Kind {
		case "constraint":
			if item.Ordinal >= len(intent.Constraints) || entry.Kind != taskstate.EntryConstraint ||
				entry.Authority != taskstate.AuthorityCode || entry.Content != intent.Constraints[item.Ordinal] {
				return fmt.Errorf("%w: accepted intent constraint authority is inconsistent", taskstate.ErrInvalidState)
			}
		case "ambiguity":
			if item.Ordinal >= len(intent.Ambiguities) || entry.Kind != taskstate.EntryQuestion ||
				entry.Authority != taskstate.AuthorityModelProposal || entry.Content != intent.Ambiguities[item.Ordinal] {
				return fmt.Errorf("%w: accepted intent ambiguity authority is inconsistent", taskstate.ErrInvalidState)
			}
		default:
			return fmt.Errorf("%w: accepted intent projection has item kind %q", taskstate.ErrInvalidState, item.Kind)
		}
	}
	return nil
}

func sameAcceptedIntentItems(
	left, right []acceptedIntentProjectionItem,
) bool {
	key := func(item acceptedIntentProjectionItem) string {
		return fmt.Sprintf("%s:%09d", item.Kind, item.Ordinal)
	}
	leftCopy := append([]acceptedIntentProjectionItem(nil), left...)
	rightCopy := append([]acceptedIntentProjectionItem(nil), right...)
	slices.SortFunc(leftCopy, func(a, b acceptedIntentProjectionItem) int {
		return cmpText(key(a), key(b))
	})
	slices.SortFunc(rightCopy, func(a, b acceptedIntentProjectionItem) int {
		return cmpText(key(a), key(b))
	})
	return slices.Equal(leftCopy, rightCopy)
}

func cmpText(left, right string) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
