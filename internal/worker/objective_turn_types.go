package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/webresearch"
)

const maxObjectiveOutputBytes = 32 * 1024

const (
	maxObjectiveEvidenceIDBytes         = 128
	maxObjectiveEvidenceTextBytes       = 2 * 1024
	maxObjectiveEvidenceSourceTypeBytes = 64
	maxObjectiveEvidenceSourceRefBytes  = 512
)

type turnAuthority struct {
	JobID                           int64
	Pipeline                        string
	Instruction                     string
	SHA256                          string
	DataSourceID                    model.DataSourceID
	DelegatedDataAuthorityID        string
	ChannelID                       model.ChannelID
	ChannelMode                     model.ChannelMode
	RoleplayViewpointCharacterID    model.RoleplayCharacterID
	RoleplaySimulationPreparationID string
	RoleplayWorldID                 string
	RoleplaySceneID                 string
	RoleplaySceneRevision           int64
	RoleplayInputKind               roleplay.SimulationTurnInputKind
	RoleplayParticipantCharacterIDs []model.RoleplayCharacterID
	RoleplayNarrativeFingerprint    string
	RoleplayContext                 *roleplay.NarrativeSimulationProjection
	Context                         assemblyline.ObjectiveContext
}

type objectiveStationReceipt struct {
	Calls int
}

type objectiveKindStation interface {
	Classify(context.Context, assemblyline.ConversationObjectiveKindInput) (
		assemblyline.ConversationObjectiveKindDecision, objectiveStationReceipt, error,
	)
}

type conversationCandidateSet struct {
	Turns            []assemblyline.ConversationContextTurn
	AssistantResults []assemblyline.ConversationSelectedAssistantResult
}

type objectiveConversationCandidateProvider interface {
	Candidates(context.Context, model.Job) (conversationCandidateSet, error)
}

type objectiveContextSelectionStation interface {
	Select(context.Context, assemblyline.ConversationContextSelectionInput) (
		assemblyline.ConversationContextSelectionDecision, objectiveStationReceipt, error,
	)
}

type objectiveMemoryContextCandidateSet struct {
	Replan     *assemblyline.ObjectiveReplanAuthority
	Candidates []assemblyline.MemoryContextCandidate
}

type objectiveMemoryContextCandidateProvider interface {
	MemoryCandidates(context.Context, model.Job) (objectiveMemoryContextCandidateSet, error)
}

type objectiveMemoryContextSelectionStation interface {
	SelectMemory(context.Context, assemblyline.MemoryContextSelectionInput) (
		assemblyline.MemoryContextSelectionDecision, objectiveStationReceipt, error,
	)
}

type objectiveAnswerStation interface {
	Answer(context.Context, assemblyline.GroundedAnswerInput) (
		assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error,
	)
}

type objectiveConversationStation interface {
	Respond(context.Context, assemblyline.ConversationResponseInput) (
		assemblyline.ConversationResponseDecision, objectiveStationReceipt, error,
	)
}

type objectiveRoleplayCanonStation interface {
	ExtractCanon(context.Context, assemblyline.RoleplayCanonExtractionInput) (
		assemblyline.RoleplayCanonExtractionDecision, objectiveStationReceipt, error,
	)
}

type objectiveRoleplayGroundedStation interface {
	RespondGrounded(context.Context, assemblyline.RoleplayGroundedResponseInput) (
		assemblyline.RoleplayGroundedResponseDecision, objectiveStationReceipt, error,
	)
}

type objectiveWorkflows struct {
	WorkspaceMutation  func(context.Context, turnAuthority) (string, error)
	RepositoryRead     func(context.Context, turnAuthority) (objectiveEvidenceAcquisition, error)
	ExternalAnswer     func(context.Context, turnAuthority) (objectiveExternalAnswer, error)
	DatabaseRead       func(context.Context, turnAuthority, string) (objectiveEvidenceAcquisition, error)
	RoleplaySimulation func(context.Context, string, int64) (roleplay.SimulationTurnAuthority, roleplay.NarrativeSimulationProjection, error)
	RoleplayCanon      objectiveRoleplayCanonStation
	RoleplayResearch   func(context.Context, turnAuthority) (objectiveRoleplayResearchAnswer, error)
	ObjectiveAdvisory  objectiveAdvisoryRunner
}

type objectiveEvidenceAcquisition struct {
	Evidence             []objectiveEvidence
	ModelCalls           int
	RepositoryCallLedger objectiveRepositoryAcquisitionCallLedger
}

type objectiveExternalAnswer struct {
	Text           string
	Rendered       string
	RenderedSHA256 string
	Paragraphs     []webresearch.GroundedParagraph
	Evidence       []objectiveEvidence
	EvidenceIDs    []string
	ModelCalls     int
}

type objectiveRoleplayResearchAnswer struct {
	Research       roleplay.ResearchTurnAuthority
	Text           string
	Rendered       string
	RenderedSHA256 string
	Paragraphs     []webresearch.GroundedParagraph
	Evidence       []objectiveEvidence
	EvidenceIDs    []string
	ModelCalls     int
}

type objectiveEvidence struct {
	Capsule       assemblyline.GroundedEvidenceCapsule
	SourceType    string
	SourceRef     string
	SHA256        string
	SourceSHA256  string
	ParagraphMask uint8
	ObservedAt    time.Time
	Truncated     bool
}

type objectiveTurnResult struct {
	ObjectiveID                   string
	RequirementID                 string
	InstructionSHA256             string
	Kind                          assemblyline.ConversationObjectiveKind
	Citations                     []objectiveEvidence
	Output                        string
	CitationsRendered             bool
	ModelCalls                    int
	Advisory                      objectiveadvisory.Report
	RoleplayFacts                 []string
	RoleplayKnowledgeCharacterIDs []model.RoleplayCharacterID
	RoleplayResearch              *roleplay.ResearchTurnAuthority
	Complete                      bool
}

func newObjectiveEvidence(
	id, text, sourceType, sourceRef string,
) (objectiveEvidence, error) {
	if err := validateObjectiveEvidenceLine("ID", id, maxObjectiveEvidenceIDBytes); err != nil {
		return objectiveEvidence{}, err
	}
	if strings.TrimSpace(text) == "" || len(text) > maxObjectiveEvidenceTextBytes ||
		!utf8.ValidString(text) || strings.ContainsRune(text, '\x00') {
		return objectiveEvidence{}, fmt.Errorf("objective evidence text is blank, oversized, invalid UTF-8, or contains NUL")
	}
	if err := validateObjectiveEvidenceLine(
		"source type", sourceType, maxObjectiveEvidenceSourceTypeBytes,
	); err != nil {
		return objectiveEvidence{}, err
	}
	if err := validateObjectiveEvidenceLine(
		"source reference", sourceRef, maxObjectiveEvidenceSourceRefBytes,
	); err != nil {
		return objectiveEvidence{}, err
	}
	digest := sha256.Sum256([]byte(text))
	return objectiveEvidence{
		Capsule:    assemblyline.GroundedEvidenceCapsule{ID: id, Text: text},
		SourceType: sourceType, SourceRef: sourceRef,
		SHA256: hex.EncodeToString(digest[:]), SourceSHA256: hex.EncodeToString(digest[:]),
	}, nil
}

func validObjectiveSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func validateObjectiveEvidenceLine(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") ||
		len(value) > maximum || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("objective evidence %s must be one bounded trimmed UTF-8 line", label)
	}
	return nil
}

func newTurnAuthority(job model.Job) (turnAuthority, error) {
	if job.ID < 1 {
		return turnAuthority{}, fmt.Errorf("conversation turn requires a positive job ID")
	}
	pipeline := job.Pipeline
	switch pipeline {
	case model.PipelineChat:
	default:
		return turnAuthority{}, fmt.Errorf("conversation turn pipeline %q is unsupported", pipeline)
	}
	if strings.TrimSpace(job.Instruction) == "" {
		return turnAuthority{}, fmt.Errorf("conversation turn requires one non-blank exact instruction")
	}
	if !utf8.ValidString(job.Instruction) || strings.ContainsRune(job.Instruction, '\x00') {
		return turnAuthority{}, fmt.Errorf("conversation turn instruction is invalid UTF-8 or contains NUL")
	}
	digest := sha256.Sum256([]byte(job.Instruction))
	var metadata struct {
		ChannelID                       model.ChannelID                  `json:"channel_id"`
		DataSourceID                    model.DataSourceID               `json:"data_source_id"`
		DelegatedDataAuthorityID        string                           `json:"delegated_data_authority_id"`
		ChannelMode                     model.ChannelMode                `json:"channel_mode"`
		RoleplayViewpointCharacterID    model.RoleplayCharacterID        `json:"roleplay_viewpoint_character_id"`
		RoleplaySimulationPreparationID string                           `json:"roleplay_simulation_preparation_id"`
		RoleplayWorldID                 string                           `json:"roleplay_world_id"`
		RoleplaySceneID                 string                           `json:"roleplay_scene_id"`
		RoleplaySceneRevision           int64                            `json:"roleplay_scene_revision"`
		RoleplayInputKind               roleplay.SimulationTurnInputKind `json:"roleplay_input_kind"`
		RoleplayParticipantCharacterIDs []model.RoleplayCharacterID      `json:"roleplay_participant_character_ids"`
		RoleplayNarrativeFingerprint    string                           `json:"roleplay_narrative_fingerprint"`
	}
	if len(job.Metadata) == 0 {
		return turnAuthority{}, fmt.Errorf("conversation turn requires exact channel mode authority")
	}
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		return turnAuthority{}, fmt.Errorf("decode conversation turn authority: %w", err)
	}
	if err := metadata.ChannelMode.Validate(); err != nil {
		return turnAuthority{}, fmt.Errorf("conversation turn mode authority: %w", err)
	}
	if err := metadata.ChannelID.Validate(); err != nil {
		return turnAuthority{}, fmt.Errorf("conversation turn channel authority: %w", err)
	}
	if metadata.DataSourceID != "" {
		if err := metadata.DataSourceID.Validate(); err != nil {
			return turnAuthority{}, fmt.Errorf("conversation turn data-source authority: %w", err)
		}
	}
	if metadata.DelegatedDataAuthorityID != "" {
		if metadata.DataSourceID == "" {
			return turnAuthority{}, fmt.Errorf("conversation turn delegated authority requires a data source")
		}
		if err := datasource.ValidateDelegatedAuthorityID(metadata.DelegatedDataAuthorityID); err != nil {
			return turnAuthority{}, fmt.Errorf("conversation turn delegated data authority: %w", err)
		}
	}
	switch metadata.ChannelMode {
	case model.ChannelModeAssistant:
		if metadata.RoleplayViewpointCharacterID != "" || metadata.RoleplaySimulationPreparationID != "" ||
			metadata.RoleplayWorldID != "" || metadata.RoleplaySceneID != "" ||
			metadata.RoleplaySceneRevision != 0 || metadata.RoleplayInputKind != "" ||
			metadata.RoleplayParticipantCharacterIDs != nil || metadata.RoleplayNarrativeFingerprint != "" {
			return turnAuthority{}, fmt.Errorf("assistant conversation cannot carry fictional simulation authority")
		}
	case model.ChannelModeRoleplay:
		if metadata.DataSourceID != "" {
			return turnAuthority{}, fmt.Errorf("roleplay conversation cannot carry real-world data-source authority")
		}
		if err := metadata.RoleplayViewpointCharacterID.Validate(); err != nil {
			return turnAuthority{}, fmt.Errorf("conversation turn roleplay viewpoint: %w", err)
		}
		if metadata.RoleplaySimulationPreparationID == "" || metadata.RoleplayWorldID == "" ||
			metadata.RoleplaySceneID == "" || metadata.RoleplaySceneRevision < 1 ||
			metadata.RoleplayNarrativeFingerprint == "" || len(metadata.RoleplayParticipantCharacterIDs) < 1 {
			return turnAuthority{}, fmt.Errorf("conversation turn requires exact simulation preparation authority")
		}
		if metadata.RoleplayInputKind != roleplay.SimulationTurnProse &&
			metadata.RoleplayInputKind != roleplay.SimulationTurnAction &&
			metadata.RoleplayInputKind != roleplay.SimulationTurnExternalCommand {
			return turnAuthority{}, fmt.Errorf("conversation turn roleplay input kind is invalid")
		}
		seenParticipants := make(map[model.RoleplayCharacterID]struct{}, len(metadata.RoleplayParticipantCharacterIDs))
		activeFound := false
		for index, participantID := range metadata.RoleplayParticipantCharacterIDs {
			if err := participantID.Validate(); err != nil {
				return turnAuthority{}, fmt.Errorf("conversation turn roleplay participant %d: %w", index, err)
			}
			if _, duplicate := seenParticipants[participantID]; duplicate {
				return turnAuthority{}, fmt.Errorf("conversation turn roleplay participant %q is duplicated", participantID)
			}
			seenParticipants[participantID] = struct{}{}
			activeFound = activeFound || participantID == metadata.RoleplayViewpointCharacterID
		}
		if !activeFound {
			return turnAuthority{}, fmt.Errorf("conversation turn roleplay viewpoint is not a prepared participant")
		}
	}
	authority := turnAuthority{
		JobID: job.ID, Pipeline: pipeline, Instruction: job.Instruction,
		SHA256: hex.EncodeToString(digest[:]), DataSourceID: metadata.DataSourceID,
		DelegatedDataAuthorityID:        metadata.DelegatedDataAuthorityID,
		ChannelID:                       metadata.ChannelID,
		ChannelMode:                     metadata.ChannelMode,
		RoleplayViewpointCharacterID:    metadata.RoleplayViewpointCharacterID,
		RoleplaySimulationPreparationID: metadata.RoleplaySimulationPreparationID,
		RoleplayWorldID:                 metadata.RoleplayWorldID, RoleplaySceneID: metadata.RoleplaySceneID,
		RoleplaySceneRevision: metadata.RoleplaySceneRevision, RoleplayInputKind: metadata.RoleplayInputKind,
		RoleplayParticipantCharacterIDs: append([]model.RoleplayCharacterID(nil), metadata.RoleplayParticipantCharacterIDs...),
		RoleplayNarrativeFingerprint:    metadata.RoleplayNarrativeFingerprint,
	}
	if err := authority.Context.Validate(); err != nil {
		return turnAuthority{}, err
	}
	return authority, nil
}

func objectiveTurnID(authority turnAuthority, kind assemblyline.ConversationObjectiveKind) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%d\x00%s\x00%s\x00%s", authority.JobID, authority.Pipeline, authority.SHA256, kind)
	_, _ = fmt.Fprintf(hash, "\x00data-source\x00%s", authority.DataSourceID)
	_, _ = fmt.Fprintf(hash, "\x00delegated-data-authority\x00%s", authority.DelegatedDataAuthorityID)
	_, _ = fmt.Fprintf(hash, "\x00channel\x00%s\x00channel-mode\x00%s\x00viewpoint\x00%s",
		authority.ChannelID, authority.ChannelMode, authority.RoleplayViewpointCharacterID)
	if authority.RoleplayContext != nil {
		_, _ = fmt.Fprintf(hash, "\x00roleplay-context\x00%s", authority.RoleplayNarrativeFingerprint)
	}
	for _, selected := range authority.Context.UserAuthorities {
		_, _ = fmt.Fprintf(hash, "\x00user\x00%d\x00%x", selected.MessageID, sha256.Sum256([]byte(selected.Content)))
	}
	for _, selected := range authority.Context.AssistantResults {
		_, _ = fmt.Fprintf(hash, "\x00assistant\x00%d\x00%d\x00%d\x00%x",
			selected.UserMessageID, selected.MessageID, selected.JobID, sha256.Sum256([]byte(selected.Content)))
	}
	for _, selected := range authority.Context.MemoryAuthorities {
		_, _ = fmt.Fprintf(hash, "\x00memory\x00%d\x00%s\x00%s",
			selected.MemoryID, selected.Kind, selected.ContentSHA256)
	}
	if replan := authority.Context.ReplanAuthority; replan != nil {
		_, _ = fmt.Fprintf(hash, "\x00replan\x00%d\x00%d\x00%s",
			replan.JobID, replan.Generation, replan.FeedbackSHA256)
	}
	return "objective-" + hex.EncodeToString(hash.Sum(nil))
}

func objectiveRequirementID(objectiveID string) string {
	digest := sha256.Sum256([]byte("requirement\x00" + objectiveID))
	return "requirement-" + hex.EncodeToString(digest[:])
}
