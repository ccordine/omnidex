package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
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
	JobID       int64
	Pipeline    string
	Instruction string
	SHA256      string
	Context     assemblyline.ObjectiveContext
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

type objectiveWorkflows struct {
	WorkspaceMutation func(context.Context, turnAuthority) (string, error)
	RepositoryRead    func(context.Context, turnAuthority) (objectiveEvidenceAcquisition, error)
	ExternalAnswer    func(context.Context, turnAuthority) (objectiveExternalAnswer, error)
	ObjectiveAdvisory objectiveAdvisoryRunner
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
	ObjectiveID       string
	RequirementID     string
	InstructionSHA256 string
	Kind              assemblyline.ConversationObjectiveKind
	Citations         []objectiveEvidence
	Output            string
	CitationsRendered bool
	ModelCalls        int
	Advisory          objectiveadvisory.Report
	Complete          bool
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
	authority := turnAuthority{
		JobID: job.ID, Pipeline: pipeline, Instruction: job.Instruction,
		SHA256: hex.EncodeToString(digest[:]),
	}
	if err := authority.Context.Validate(); err != nil {
		return turnAuthority{}, err
	}
	return authority, nil
}

func objectiveTurnID(authority turnAuthority, kind assemblyline.ConversationObjectiveKind) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%d\x00%s\x00%s\x00%s", authority.JobID, authority.Pipeline, authority.SHA256, kind)
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
