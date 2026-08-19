package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

type roleplayResearchCitationReceipt struct {
	EvidenceID       int64
	CompletionIndex  int
	CapsuleID        string
	SourceRef        string
	SourceSHA256     string
	ObservedAt       time.Time
	Truncated        bool
	ParagraphIndexes []int
}

// MaterializeRoleplayResearchCompletionTx persists the REAL_WORLD citation
// receipt beside the canonical assistant message. Handled is false only when
// the job has no persisted research binding at all.
func MaterializeRoleplayResearchCompletionTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	command CompleteStepCommand,
	assistantMessageID int64,
) (handled bool, err error) {
	if ctx == nil || tx == nil {
		return false, fmt.Errorf("roleplay research completion requires transaction authority")
	}
	research, found, err := roleplay.FindResearchTurnBindingForJobTx(ctx, tx, job.ID)
	if err != nil || !found {
		return false, err
	}
	active, activeFound, err := roleplay.FindResearchTurnForJobTx(ctx, tx, job.ID)
	if err != nil {
		return true, err
	}
	if !activeFound || active.CapabilityGrantID != research.CapabilityGrantID {
		return true, roleplay.ErrResearchCapabilityDenied
	}
	if job.Pipeline != model.PipelineChat || command.Authority.JobID != job.ID ||
		job.Instruction != "/research "+fmt.Sprintf("%q", research.Question) ||
		command.ContextKey != "objective_result" || assistantMessageID < 1 {
		return true, fmt.Errorf("roleplay research completion differs from exact job and message authority")
	}
	if len(command.RoleplayFacts) != 0 || len(command.RoleplayKnowledgeCharacterIDs) != 0 {
		return true, fmt.Errorf("REAL_WORLD research completion cannot append fictional canon or character knowledge")
	}
	if command.Output == "" || command.Output != strings.TrimSpace(command.Output) {
		return true, fmt.Errorf("roleplay research completion output is blank or not exact")
	}
	citations, err := loadRoleplayResearchCitationReceiptsTx(ctx, tx, command, research)
	if err != nil {
		return true, err
	}
	digest := sha256.Sum256([]byte(command.Output))
	renderedSHA := hex.EncodeToString(digest[:])
	if _, err := tx.Exec(ctx, `
		INSERT INTO roleplay_research_completions (
			operation_id,preparation_id,job_id,source_message_id,rendered_sha256
		) VALUES ($1,$2,$3,$4,$5)
	`, command.OperationID, research.PreparationID, job.ID, assistantMessageID, renderedSHA); err != nil {
		return true, fmt.Errorf("insert roleplay research completion: %w", err)
	}
	for _, citation := range citations {
		paragraphJSON, err := json.Marshal(citation.ParagraphIndexes)
		if err != nil {
			return true, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO roleplay_research_completion_citations (
				operation_id,completion_index,evidence_id,capsule_id,source_ref,
				source_sha256,observed_at,truncated,paragraph_indexes
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
		`, command.OperationID, citation.CompletionIndex, citation.EvidenceID,
			citation.CapsuleID, citation.SourceRef, citation.SourceSHA256,
			citation.ObservedAt, citation.Truncated, string(paragraphJSON)); err != nil {
			return true, fmt.Errorf("insert roleplay research citation %d: %w", citation.CompletionIndex, err)
		}
	}
	return true, nil
}

func loadRoleplayResearchCitationReceiptsTx(
	ctx context.Context,
	tx pgx.Tx,
	command CompleteStepCommand,
	research roleplay.ResearchTurnAuthority,
) ([]roleplayResearchCitationReceipt, error) {
	rows, err := tx.Query(ctx, `
		SELECT item.id,item.completion_evidence_index,item.payload_json
		FROM step_completion_evidence_sets AS authority
		JOIN evidence AS item
		  ON item.completion_operation_id=authority.operation_id
		WHERE authority.operation_id=$1 AND authority.job_id=$2
		ORDER BY item.completion_evidence_index
	`, command.OperationID, command.Authority.JobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]roleplayResearchCitationReceipt, 0, 2)
	for rows.Next() {
		var id int64
		var index int
		var raw []byte
		if err := rows.Scan(&id, &index, &raw); err != nil {
			return nil, err
		}
		if index != len(result) {
			return nil, fmt.Errorf("roleplay research citation indexes are not contiguous")
		}
		var record evidence.Record
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, fmt.Errorf("decode roleplay research citation %d: %w", index, err)
		}
		receipt, err := roleplayResearchCitationReceiptFromRecord(id, index, record, research)
		if err != nil {
			return nil, err
		}
		result = append(result, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result) < 1 || len(result) > 4 {
		return nil, fmt.Errorf("roleplay research completion requires 1..4 exact citations")
	}
	return result, nil
}

func roleplayResearchCitationReceiptFromRecord(
	evidenceID int64,
	index int,
	record evidence.Record,
	research roleplay.ResearchTurnAuthority,
) (roleplayResearchCitationReceipt, error) {
	if record.Kind != evidence.KindObjectiveCitation || record.SourceType != "web_document" ||
		record.SourceRef == "" || !validObjectiveEvidenceSHA(record.Hash) {
		return roleplayResearchCitationReceipt{}, fmt.Errorf("roleplay research citation %d is not exact web evidence", index)
	}
	wantStrings := map[string]string{
		"authority_namespace":                   string(roleplay.AuthorityRealWorld),
		"roleplay_research_preparation_id":      research.PreparationID,
		"roleplay_research_world_id":            research.WorldID,
		"roleplay_research_character_id":        research.CharacterID,
		"roleplay_research_question_sha256":     research.QuestionSHA256,
		"roleplay_research_capability_grant_id": research.CapabilityGrantID,
		"source_sha256":                         record.Hash,
	}
	for key, want := range wantStrings {
		value, ok := record.Metadata[key].(string)
		if !ok || value != want {
			return roleplayResearchCitationReceipt{}, fmt.Errorf(
				"roleplay research citation %d metadata %q differs from authority", index, key,
			)
		}
	}
	capsuleID, ok := record.Metadata["capsule_id"].(string)
	if !ok || capsuleID == "" || len(capsuleID) > 128 {
		return roleplayResearchCitationReceipt{}, fmt.Errorf("roleplay research citation %d has invalid capsule identity", index)
	}
	observedRaw, ok := record.Metadata["source_observed_at"].(string)
	if !ok {
		return roleplayResearchCitationReceipt{}, fmt.Errorf("roleplay research citation %d lacks observation authority", index)
	}
	observed, err := time.Parse(time.RFC3339Nano, observedRaw)
	if err != nil || observed.Location() != time.UTC || observed.Format(time.RFC3339Nano) != observedRaw {
		return roleplayResearchCitationReceipt{}, fmt.Errorf("roleplay research citation %d observation is not canonical UTC", index)
	}
	truncated, ok := record.Metadata["source_truncated"].(bool)
	if !ok {
		return roleplayResearchCitationReceipt{}, fmt.Errorf("roleplay research citation %d lacks truncation authority", index)
	}
	paragraphIndexes, err := objectiveIntegerIndexes(record.Metadata["paragraph_indexes"])
	if err != nil {
		return roleplayResearchCitationReceipt{}, fmt.Errorf("roleplay research citation %d: %w", index, err)
	}
	return roleplayResearchCitationReceipt{
		EvidenceID: evidenceID, CompletionIndex: index, CapsuleID: capsuleID,
		SourceRef: record.SourceRef, SourceSHA256: record.Hash,
		ObservedAt: observed, Truncated: truncated, ParagraphIndexes: paragraphIndexes,
	}, nil
}

func objectiveIntegerIndexes(raw any) ([]int, error) {
	values, ok := raw.([]any)
	if !ok || len(values) < 1 || len(values) > 4 {
		return nil, fmt.Errorf("paragraph indexes require 1..4 exact integers")
	}
	result := make([]int, len(values))
	for index, rawValue := range values {
		value, ok := rawValue.(float64)
		if !ok || value < 0 || value > 3 || value != float64(int(value)) {
			return nil, fmt.Errorf("paragraph indexes require exact integers in 0..3")
		}
		result[index] = int(value)
	}
	if !slices.IsSorted(result) {
		return nil, fmt.Errorf("paragraph indexes must be ascending")
	}
	for index := 1; index < len(result); index++ {
		if result[index] == result[index-1] {
			return nil, fmt.Errorf("paragraph indexes must be unique")
		}
	}
	return result, nil
}

func validObjectiveEvidenceSHA(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}
