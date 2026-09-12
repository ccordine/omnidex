package worker

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func objectiveModelEvidence(
	items []objectiveEvidence,
) ([]assemblyline.GroundedEvidenceCapsule, error) {
	projected := make([]assemblyline.GroundedEvidenceCapsule, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		if err := validateObjectiveEvidence(item); err != nil {
			return nil, err
		}
		if _, duplicate := seen[item.Capsule.ID]; duplicate {
			return nil, fmt.Errorf("objective evidence ID %q is duplicated", item.Capsule.ID)
		}
		seen[item.Capsule.ID] = struct{}{}
		projected[index] = item.Capsule
	}
	return projected, nil
}

func selectObjectiveCitations(
	available []objectiveEvidence,
	ids []string,
) ([]objectiveEvidence, error) {
	byID := make(map[string]objectiveEvidence, len(available))
	for _, item := range available {
		if err := validateObjectiveEvidence(item); err != nil {
			return nil, err
		}
		if _, duplicate := byID[item.Capsule.ID]; duplicate {
			return nil, fmt.Errorf("objective evidence ID %q is duplicated", item.Capsule.ID)
		}
		byID[item.Capsule.ID] = item
	}
	selected := make([]objectiveEvidence, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		item, exists := byID[id]
		if !exists {
			return nil, fmt.Errorf("objective cited unavailable evidence ID %q", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("objective cited evidence ID %q more than once", id)
		}
		seen[id] = struct{}{}
		selected = append(selected, item)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("grounded objective requires at least one cited evidence capsule")
	}
	return selected, nil
}

func validateObjectiveEvidence(item objectiveEvidence) error {
	_, err := newObjectiveEvidence(
		item.Capsule.ID, item.Capsule.Text, item.SourceType, item.SourceRef,
	)
	if err != nil {
		return err
	}
	if item.ParagraphMask&^uint8(0x0f) != 0 {
		return fmt.Errorf("objective evidence %q exceeds paragraph binding bounds", item.Capsule.ID)
	}
	if item.SourceType == "web_document" {
		if item.WebEvidenceID < 1 || item.WebEvidenceIndex < 0 || item.WebEvidenceIndex >= 32 {
			return fmt.Errorf("objective web evidence %q requires a recorded acquisition and source index", item.Capsule.ID)
		}
		if item.ObservedAt.IsZero() || item.ObservedAt.Location() != time.UTC {
			return fmt.Errorf("objective web evidence %q requires exact UTC observation authority", item.Capsule.ID)
		}
	} else if item.SourceType == "postgres_query" {
		if item.DatabaseEvidenceID < 1 || item.DatabaseRowStart < 0 ||
			item.DatabaseRowEnd < item.DatabaseRowStart || item.DatabaseRowEnd > datasource.MaxIntentRows {
			return fmt.Errorf("objective database evidence %q requires a recorded execution and row range, not a source hash", item.Capsule.ID)
		}
		if item.ObservedAt.IsZero() || item.ObservedAt.Location() != time.UTC || item.Truncated {
			return fmt.Errorf("objective database evidence %q requires exact UTC acquisition authority without truncation", item.Capsule.ID)
		}
	} else if !item.ObservedAt.IsZero() || item.Truncated {
		return fmt.Errorf("objective evidence %q carries unsupported freshness authority", item.Capsule.ID)
	}
	if item.SourceType != "postgres_query" && (item.DatabaseEvidenceID != 0 || item.DatabaseRowStart != 0 || item.DatabaseRowEnd != 0) {
		return fmt.Errorf("objective evidence %q carries unrelated database fields", item.Capsule.ID)
	}
	if item.SourceType != "web_document" && (item.WebEvidenceID != 0 || item.WebEvidenceIndex != 0) {
		return fmt.Errorf("objective evidence %q carries unrelated web fields", item.Capsule.ID)
	}
	return nil
}

func prepareObjectiveTurnCompletion(
	result objectiveTurnResult,
) (string, []evidence.Record, error) {
	output, err := renderObjectiveTurnOutput(result)
	if err != nil {
		return "", nil, err
	}
	records := make([]evidence.Record, len(result.Citations))
	for index, citation := range result.Citations {
		record, recordErr := objectiveCitationRecord(result, citation)
		if recordErr != nil {
			return "", nil, recordErr
		}
		records[index] = record
	}
	return output, records, nil
}

func objectiveCitationRecord(
	result objectiveTurnResult,
	citation objectiveEvidence,
) (evidence.Record, error) {
	if err := validateObjectiveTurnResult(result); err != nil {
		return evidence.Record{}, err
	}
	if err := validateObjectiveEvidence(citation); err != nil {
		return evidence.Record{}, err
	}
	requirementAuthorityBindings := []string{result.RequirementID}
	paragraphIndexes := make([]int, 0, 4)
	if citation.ParagraphMask != 0 {
		requirementAuthorityBindings = requirementAuthorityBindings[:0]
		for paragraphIndex := 0; paragraphIndex < 4; paragraphIndex++ {
			if citation.ParagraphMask&(1<<paragraphIndex) == 0 {
				continue
			}
			paragraphIndexes = append(paragraphIndexes, paragraphIndex)
			requirementAuthorityBindings = append(
				requirementAuthorityBindings,
				result.RequirementID+"#paragraph-"+strconv.Itoa(paragraphIndex+1),
			)
		}
		sort.Strings(requirementAuthorityBindings)
	}
	metadata := map[string]any{
		"capsule_id":   citation.Capsule.ID,
		"objective_id": result.ObjectiveID, "objective_kind": string(result.Kind),
		"requirement_id": result.RequirementID,
	}
	if len(paragraphIndexes) > 0 {
		metadata["paragraph_indexes"] = paragraphIndexes
	}
	if citation.SourceType == "web_document" {
		metadata["web_evidence_id"] = strconv.FormatInt(citation.WebEvidenceID, 10)
		metadata["web_evidence_index"] = strconv.Itoa(citation.WebEvidenceIndex)
		metadata["source_observed_at"] = citation.ObservedAt.Format(time.RFC3339Nano)
		metadata["source_truncated"] = citation.Truncated
		if result.RoleplayResearch != nil {
			research := result.RoleplayResearch
			metadata["authority_namespace"] = string(roleplay.AuthorityRealWorld)
			metadata["roleplay_research_preparation_id"] = research.PreparationID
			metadata["roleplay_research_world_id"] = research.WorldID
			metadata["roleplay_research_character_id"] = research.CharacterID
			metadata["roleplay_research_capability_grant_id"] = research.CapabilityGrantID
		}
	} else if citation.SourceType == "postgres_query" {
		metadata["source_acquired_at"] = citation.ObservedAt.Format(time.RFC3339Nano)
		metadata["database_evidence_id"] = strconv.FormatInt(citation.DatabaseEvidenceID, 10)
		metadata["database_row_start"] = strconv.Itoa(citation.DatabaseRowStart)
		metadata["database_row_end"] = strconv.Itoa(citation.DatabaseRowEnd)
	}
	return evidence.Record{
		Kind: evidence.KindObjectiveCitation, SourceType: citation.SourceType,
		SourceRef: citation.SourceRef, Excerpt: citation.Capsule.Text,
		Summary: fmt.Sprintf(
			"Objective %s cited evidence capsule %s.", result.ObjectiveID, citation.Capsule.ID,
		),
		Confidence:                   1,
		RequirementAuthorityBindings: requirementAuthorityBindings,
		Metadata:                     metadata,
	}, nil
}

func renderObjectiveTurnOutput(result objectiveTurnResult) (string, error) {
	if err := validateObjectiveTurnResult(result); err != nil {
		return "", err
	}
	if len(result.Citations) == 0 {
		return result.Output, nil
	}
	if result.CitationsRendered {
		return result.Output, nil
	}
	lines := []string{result.Output, "", "Sources:"}
	for _, citation := range result.Citations {
		if err := validateObjectiveEvidence(citation); err != nil {
			return "", err
		}
		lines = append(lines, fmt.Sprintf(
			"- [%s] %s:%s", citation.Capsule.ID,
			citation.SourceType, citation.SourceRef,
		))
	}
	output := strings.Join(lines, "\n")
	if len(output) > maxObjectiveOutputBytes {
		return "", fmt.Errorf("objective completion output exceeds %d bytes", maxObjectiveOutputBytes)
	}
	return output, nil
}

func validateObjectiveTurnResult(result objectiveTurnResult) error {
	if !result.Complete || strings.TrimSpace(result.Output) == "" ||
		!utf8.ValidString(result.Output) ||
		strings.ContainsRune(result.Output, '\x00') {
		return fmt.Errorf("objective result is incomplete or has invalid output")
	}
	if len(result.RoleplayResponses) == 0 {
		if len(result.Output) > maxObjectiveOutputBytes {
			return fmt.Errorf("objective result is incomplete or has invalid output")
		}
	} else {
		if err := queue.ValidateRoleplayResponseRound(result.RoleplayResponses); err != nil {
			return fmt.Errorf("objective roleplay response round: %w", err)
		}
		if result.Output != queue.RenderRoleplayResponseRound(result.RoleplayResponses) {
			return fmt.Errorf("objective output differs from its exact ordered roleplay response round")
		}
	}
	if result.ObjectiveID == "" || result.ObjectiveID != strings.TrimSpace(result.ObjectiveID) ||
		result.RequirementID == "" || result.RequirementID != strings.TrimSpace(result.RequirementID) {
		return fmt.Errorf("objective result requires exact objective and requirement IDs")
	}
	if result.RoleplayResearch != nil {
		if err := result.RoleplayResearch.Validate(); err != nil {
			return fmt.Errorf("objective roleplay research authority: %w", err)
		}
		if result.Kind != assemblyline.ObjectiveKindExternalAnswer {
			return fmt.Errorf("roleplay research result has objective kind %q", result.Kind)
		}
		if result.ModelCalls < 0 {
			return fmt.Errorf(
				"roleplay research result has negative model-call evidence %d",
				result.ModelCalls,
			)
		}
		if len(result.RoleplayResponses) != 0 || result.RoleplayUserCanon != nil ||
			result.RoleplayUserOngoingAction != nil {
			return fmt.Errorf("roleplay research result attempted to persist fictional canon")
		}
	}
	switch result.Kind {
	case assemblyline.ObjectiveKindExternalAnswer, assemblyline.ObjectiveKindDatabaseRead:
		if len(result.Citations) == 0 {
			return fmt.Errorf("grounded objective result requires cited evidence")
		}
	case assemblyline.ObjectiveKindAnswer,
		assemblyline.ObjectiveKindStory,
		assemblyline.ObjectiveKindWorkspaceMutation:
		if len(result.Citations) != 0 {
			return fmt.Errorf("objective kind %q cannot carry grounded citations", result.Kind)
		}
	default:
		return fmt.Errorf("objective result kind %q is unsupported", result.Kind)
	}
	if result.Kind == assemblyline.ObjectiveKindExternalAnswer {
		if !result.CitationsRendered {
			return fmt.Errorf("external objective lost code-rendered citations")
		}
		for _, citation := range result.Citations {
			if citation.ParagraphMask == 0 {
				return fmt.Errorf("external citation %q lost paragraph authority", citation.Capsule.ID)
			}
			if result.RoleplayResearch != nil && citation.SourceType != "web_document" {
				return fmt.Errorf("roleplay research citation %q is not real-world web evidence", citation.Capsule.ID)
			}
		}
	} else if result.CitationsRendered {
		return fmt.Errorf("objective kind %q cannot carry pre-rendered citations", result.Kind)
	}
	return nil
}
