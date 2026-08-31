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
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
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
	ModelInstruction                string
	ModelRedactedInstruction        string
	ModelArtifactIdentities         []assemblyline.ArtifactIdentity
	ModelArtifactPaths              []string
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
	RoleplayGenerationConfig        *roleplay.CharacterGenerationConfig
	RoleplayResponders              []roleplay.SimulationResponderRoute
	RoleplayUserTurn                *roleplay.UserTurnAuthority
	RoleplayIdentity                *assemblyline.RoleplayResponseIdentity
	RoleplayEarlierResponses        []roleplayRoundResponseAuthority
	Context                         assemblyline.ObjectiveContext
}

type roleplayRoundResponseAuthority struct {
	Position      int
	CharacterID   model.RoleplayCharacterID
	CharacterName string
	Text          string
}

type objectiveStationReceipt struct {
	Calls  int
	Reused bool
}

func validateObjectiveStationReceipt(label string, receipt objectiveStationReceipt) error {
	if receipt.Reused {
		if receipt.Calls != 0 {
			return fmt.Errorf("%s reuse reported %d provider calls", label, receipt.Calls)
		}
		return nil
	}
	if receipt.Calls != exactSemanticLeafCalls {
		return fmt.Errorf(
			"%s reported %d calls; one exact semantic leaf requires exactly %d",
			label, receipt.Calls, exactSemanticLeafCalls,
		)
	}
	return nil
}

func validateObjectiveBoundedStationReceipt(
	label string,
	receipt objectiveStationReceipt,
	maximumCalls int,
) error {
	if maximumCalls < exactSemanticLeafCalls {
		return fmt.Errorf(
			"%s has invalid maximum call budget %d", label, maximumCalls,
		)
	}
	if receipt.Reused {
		if receipt.Calls != 0 {
			return fmt.Errorf("%s reuse reported %d provider calls", label, receipt.Calls)
		}
		return nil
	}
	if receipt.Calls < exactSemanticLeafCalls || receipt.Calls > maximumCalls {
		return fmt.Errorf(
			"%s reported %d calls outside the 1..%d bounded leaf budget",
			label, receipt.Calls, maximumCalls,
		)
	}
	return nil
}

type objectiveKindStation interface {
	Classify(context.Context, assemblyline.ConversationObjectiveKindInput) (
		assemblyline.ConversationObjectiveKindDecision, objectiveStationReceipt, error,
	)
}

type objectiveContextCandidateSource interface {
	ContextSearchAvailability(
		context.Context,
		model.Job,
		turnAuthority,
		*roleplay.SimulationTurnAuthority,
		*roleplay.NarrativeSimulationProjection,
	) (contextcompiler.SearchAvailability, error)
	ContextCandidates(
		context.Context,
		model.Job,
		turnAuthority,
		*roleplay.SimulationTurnAuthority,
		*roleplay.NarrativeSimulationProjection,
		[]string,
	) (contextcompiler.CandidateSet, error)
}

type objectiveContextSieveStations interface {
	contextcompiler.RelevanceStation
	contextcompiler.MinificationStation
}

type objectiveAnswerStation interface {
	Answer(context.Context, assemblyline.GroundedAnswerInput) (
		assemblyline.GroundedAnswerDecision, objectiveStationReceipt, error,
	)
}

type objectiveConversationStation interface {
	Respond(context.Context, assemblyline.ConversationResponseInput, string) (
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
	ResolveModelPathProvenance func() (assemblyline.ArtifactIdentityProvenance, error)
	WorkspaceReplanContext func(context.Context, model.Job) (assemblyline.ObjectiveContext, error)
	WorkspaceMutation     func(context.Context, turnAuthority) (string, error)
	DatabaseRead          func(context.Context, turnAuthority, string) (objectiveEvidenceAcquisition, error)
	RoleplaySimulation    func(context.Context, string, int64) (roleplay.SimulationTurnAuthority, roleplay.NarrativeSimulationProjection, error)
	RoleplayCanon         objectiveRoleplayCanonStation
	RoleplayCanonDelta    func(context.Context, string, []string) ([]string, error)
	RoleplayOngoingAction objectiveRoleplayOngoingActionStation
	RoleplayResearch      func(context.Context, turnAuthority) (objectiveRoleplayResearchAnswer, error)
}

type objectiveEvidenceAcquisition struct {
	Evidence           []objectiveEvidence
	ModelCalls         int
	DatabaseCallLedger objectiveDatabaseAcquisitionCallLedger
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
	WebCallLedger  webresearch.SemanticCallLedger
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
	ObjectiveID               string
	RequirementID             string
	InstructionSHA256         string
	Kind                      assemblyline.ConversationObjectiveKind
	Citations                 []objectiveEvidence
	Output                    string
	CitationsRendered         bool
	ModelCalls                int
	RoleplayResponses         []queue.RoleplayResponseCompletion
	RoleplayUserCanon         *queue.RoleplayUserCanonCompletion
	RoleplayUserOngoingAction *queue.RoleplayUserOngoingActionCompletion
	RoleplayResearch          *roleplay.ResearchTurnAuthority
	Complete                  bool
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
		ChannelID                       model.ChannelID                     `json:"channel_id"`
		DataSourceID                    model.DataSourceID                  `json:"data_source_id"`
		DelegatedDataAuthorityID        string                              `json:"delegated_data_authority_id"`
		ChannelMode                     model.ChannelMode                   `json:"channel_mode"`
		RoleplayViewpointCharacterID    model.RoleplayCharacterID           `json:"roleplay_viewpoint_character_id"`
		RoleplaySimulationPreparationID string                              `json:"roleplay_simulation_preparation_id"`
		RoleplayWorldID                 string                              `json:"roleplay_world_id"`
		RoleplaySceneID                 string                              `json:"roleplay_scene_id"`
		RoleplaySceneRevision           int64                               `json:"roleplay_scene_revision"`
		RoleplayInputKind               roleplay.SimulationTurnInputKind    `json:"roleplay_input_kind"`
		RoleplayParticipantCharacterIDs []model.RoleplayCharacterID         `json:"roleplay_participant_character_ids"`
		RoleplayNarrativeFingerprint    string                              `json:"roleplay_narrative_fingerprint"`
		RoleplayGenerationConfig        *roleplay.CharacterGenerationConfig `json:"roleplay_generation_config"`
		RoleplayResponders              []roleplay.SimulationResponderRoute `json:"roleplay_responders"`
		RoleplayUserTurn                *roleplay.UserTurnAuthority         `json:"roleplay_user_turn"`
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
			metadata.RoleplayParticipantCharacterIDs != nil || metadata.RoleplayNarrativeFingerprint != "" ||
			metadata.RoleplayGenerationConfig != nil || metadata.RoleplayResponders != nil ||
			metadata.RoleplayUserTurn != nil {
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
		if metadata.RoleplayGenerationConfig == nil {
			return turnAuthority{}, fmt.Errorf("conversation turn requires frozen character generation authority")
		}
		if len(metadata.RoleplayResponders) < 1 || len(metadata.RoleplayResponders) > roleplay.MaxSceneParticipants {
			return turnAuthority{}, fmt.Errorf("conversation turn requires a bounded ordered roleplay response round")
		}
		for index, responder := range metadata.RoleplayResponders {
			if responder.Position != index {
				return turnAuthority{}, fmt.Errorf("conversation turn roleplay responder order is invalid")
			}
			if err := model.RoleplayCharacterID(responder.CharacterID).Validate(); err != nil {
				return turnAuthority{}, fmt.Errorf("conversation turn roleplay responder %d: %w", index, err)
			}
			if err := responder.GenerationConfig.Validate(); err != nil {
				return turnAuthority{}, fmt.Errorf("conversation turn roleplay responder %d generation: %w", index, err)
			}
			if responder.NarrativeFingerprint == "" {
				return turnAuthority{}, fmt.Errorf("conversation turn roleplay responder %d has no narrative fingerprint", index)
			}
		}
		if metadata.RoleplayResponders[0].CharacterID != string(metadata.RoleplayViewpointCharacterID) ||
			metadata.RoleplayResponders[0].GenerationConfig != *metadata.RoleplayGenerationConfig ||
			metadata.RoleplayResponders[0].NarrativeFingerprint != metadata.RoleplayNarrativeFingerprint {
			return turnAuthority{}, fmt.Errorf("conversation turn primary roleplay responder differs from its response round")
		}
		if err := metadata.RoleplayGenerationConfig.Validate(); err != nil {
			return turnAuthority{}, fmt.Errorf("conversation turn roleplay generation authority: %w", err)
		}
		if metadata.RoleplayUserTurn == nil {
			return turnAuthority{}, fmt.Errorf("conversation turn requires frozen user persona and contribution authority")
		}
		if err := metadata.RoleplayUserTurn.Validate(); err != nil {
			return turnAuthority{}, fmt.Errorf("conversation turn roleplay user authority: %w", err)
		}
		if metadata.RoleplayUserTurn.ExactText != job.Instruction {
			return turnAuthority{}, fmt.Errorf("conversation turn roleplay user authority changed exact instruction bytes")
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
		RoleplayGenerationConfig:        metadata.RoleplayGenerationConfig,
		RoleplayResponders:              append([]roleplay.SimulationResponderRoute(nil), metadata.RoleplayResponders...),
		RoleplayUserTurn:                metadata.RoleplayUserTurn,
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
	if authority.RoleplayIdentity != nil {
		_, _ = fmt.Fprintf(hash, "\x00roleplay-context\x00%s", authority.RoleplayNarrativeFingerprint)
	}
	for _, capsule := range authority.Context.Capsules {
		_, _ = fmt.Fprintf(hash, "\x00minified-context\x00%s", capsule.ContentSHA256)
		for _, source := range capsule.Sources {
			_, _ = fmt.Fprintf(hash, "\x00source\x00%s\x00%s\x00%s",
				source.Namespace, source.CandidateID, source.ContentSHA256)
		}
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
