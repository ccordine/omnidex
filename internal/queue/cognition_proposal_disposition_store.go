package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func insertCognitionProposalDispositionTx(
	ctx context.Context, tx pgx.Tx, value cognitionProposalDisposition,
) error {
	if err := value.validate(); err != nil {
		return err
	}
	raw, rawSHA, err := cognitionJSON(value)
	if err != nil {
		return err
	}
	identityRaw, identitySHA, err := cognitionJSON(value.identity())
	if err != nil || identitySHA != value.SHA256 {
		return fmt.Errorf("%w: proposal disposition identity changed", ErrCognitionConflict)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_proposal_dispositions (
			disposition_id,disposition_sha256,episode_id,ledger_id,reconciliation_id,
			action_id,candidate_entry_id,proposal_kind,proposal_index,source_descriptor_id,
			source_descriptor_sha256,outcome,proof_uri,proof_version,proof_sha256,
			result_entry_id,command_id,command_sha256,identity_json,identity_json_sha256,
			descriptor_json,descriptor_json_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
	`, value.ID, value.SHA256, value.EpisodeID, value.LedgerID, value.ReconciliationID,
		value.ActionID, value.CandidateEntryID, value.ProposalKind, value.ProposalIndex,
		value.SourceDescriptorID, value.SourceDescriptorSHA, value.Outcome,
		value.ProofRef.URI, value.ProofRef.Version, value.ProofRef.Hash,
		nullableTaskText(string(value.ResultEntryID)), value.CommandID, value.CommandSHA256,
		string(identityRaw), identitySHA, string(raw), rawSHA)
	if err != nil {
		return fmt.Errorf("insert cognition proposal disposition: %w", err)
	}
	return nil
}

func requireCognitionProposalDispositionReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record CognitionActionRecord,
	outcome cognitionProposalOutcome,
	proof taskstate.Ref,
) error {
	link, found, err := loadCognitionActionGraphMaterializationTx(ctx, tx, record)
	if err != nil {
		return err
	}
	var raw []byte
	var rawSHA string
	err = tx.QueryRow(ctx, `
		SELECT descriptor_json,descriptor_json_sha256
		FROM cognition_proposal_dispositions WHERE reconciliation_id=$1
	`, record.ReconciliationID).Scan(&raw, &rawSHA)
	if !found && errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if !found {
		return fmt.Errorf("%w: action without proposal gained a disposition", ErrCognitionConflict)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: proposal action lost its disposition", ErrCognitionConflict)
	}
	if err != nil || cognitionPayloadSHA(raw) != rawSHA {
		return fmt.Errorf("%w: proposal disposition persistence changed", ErrCognitionConflict)
	}
	var value cognitionProposalDisposition
	if json.Unmarshal(raw, &value) != nil || value.validate() != nil {
		return fmt.Errorf("%w: proposal disposition is invalid", ErrCognitionConflict)
	}
	want := value
	want.ID, want.SHA256, want.CommandID, want.CommandSHA256, want.ResultEntryID = "", "", "", "", ""
	_ = want
	if value.EpisodeID != record.EpisodeID || value.ReconciliationID != record.ReconciliationID ||
		value.ActionID != record.Action.ID || value.CandidateEntryID != link.Candidate ||
		value.ProposalKind != link.Kind || value.Outcome != outcome ||
		value.ProofRef != proof {
		return fmt.Errorf("%w: proposal disposition replay changed authority", ErrCognitionConflict)
	}
	descriptorID, descriptorSHA, proposalIndex, descriptorErr := link.descriptor()
	if descriptorErr != nil || value.SourceDescriptorID != descriptorID ||
		value.SourceDescriptorSHA != descriptorSHA || value.ProposalIndex != proposalIndex {
		return fmt.Errorf("%w: proposal disposition replay changed descriptor", ErrCognitionConflict)
	}
	canonical, _, encodeErr := cognitionJSON(value)
	if encodeErr != nil || !bytes.Equal(canonical, raw) {
		return fmt.Errorf("%w: proposal disposition is not canonical", ErrCognitionConflict)
	}
	return nil
}

func cognitionActionFailureProof(failure cognition.ActionFailure) (taskstate.Ref, error) {
	_, digest, err := cognitionJSON(failure)
	if err != nil {
		return taskstate.Ref{}, err
	}
	return taskstate.Ref{URI: "cognition:action-failure/" + string(failure.ActionID),
		Version: string(failure.Code), Hash: digest, Relation: taskstate.RefVerifies}, nil
}

func cognitionTransitionProof(transition cognition.Transition) (taskstate.Ref, error) {
	_, digest, err := cognitionJSON(transition)
	if err != nil {
		return taskstate.Ref{}, err
	}
	return taskstate.Ref{URI: "cognition:transition/" + string(transition.ActionID),
		Version: transition.Current.SHA256, Hash: digest, Relation: taskstate.RefVerifies}, nil
}
