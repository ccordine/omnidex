package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const scrumChannelOperationSchema = "omnidex.scrum-channel-operation.v1"

type ScrumChannelEffectKind string

const (
	ScrumChannelStartJob       ScrumChannelEffectKind = "start_job"
	ScrumChannelReplanJob      ScrumChannelEffectKind = "replan_job"
	ScrumChannelSubmitFeedback ScrumChannelEffectKind = "submit_feedback"
)

type ScrumChannelOperationRequest struct {
	OperationID LifecycleOperationID `json:"operation_id"`
	ProjectID   int64                `json:"project_id"`
	CardID      string               `json:"card_id"`
	Message     string               `json:"message"`
}

type ScrumChannelEffect struct {
	Kind        ScrumChannelEffectKind
	JobID       int64
	Instruction string
	Pipeline    string
	Metadata    json.RawMessage
}

type ScrumChannelOperationCommand struct {
	Request               ScrumChannelOperationRequest
	ExpectedCardUpdatedAt time.Time
	Effect                ScrumChannelEffect
	ResultAction          string
	ResultAgent           string
}

type ScrumChannelCardUpdate struct {
	Chat       json.RawMessage
	Column     string
	JobID      string
	ConsoleLog string
	PlayState  string
	QueueOrder int
}

type ScrumChannelCardBuilder func(DBScrumCard, model.Job) (ScrumChannelCardUpdate, error)

type ScrumChannelOperationResult struct {
	Card         DBScrumCard
	PreviousCard DBScrumCard
	Job          model.Job
	Action       string
	Agent        string
	Applied      bool
}

type scrumChannelOperationDescriptor struct {
	Request ScrumChannelOperationRequest
	SHA256  string
	Payload []byte
}

func describeScrumChannelOperation(request ScrumChannelOperationRequest) (scrumChannelOperationDescriptor, error) {
	if _, err := ParseLifecycleOperationID(string(request.OperationID)); err != nil {
		return scrumChannelOperationDescriptor{}, err
	}
	if request.ProjectID <= 0 {
		return scrumChannelOperationDescriptor{}, fmt.Errorf("Scrum channel operation requires a positive project ID")
	}
	request.CardID = strings.TrimSpace(request.CardID)
	if request.CardID == "" {
		return scrumChannelOperationDescriptor{}, fmt.Errorf("Scrum channel operation requires a card ID")
	}
	message, _, err := validateLifecycleFeedback(request.Message, "Scrum channel message")
	if err != nil {
		return scrumChannelOperationDescriptor{}, err
	}
	request.Message = message
	payload, err := json.Marshal(request)
	if err != nil {
		return scrumChannelOperationDescriptor{}, fmt.Errorf("encode Scrum channel operation: %w", err)
	}
	return scrumChannelOperationDescriptor{
		Request: request,
		SHA256:  lifecycleIdentityDigest(scrumChannelOperationSchema, string(payload)),
		Payload: payload,
	}, nil
}

func normalizeScrumChannelOperation(command ScrumChannelOperationCommand) (ScrumChannelOperationCommand, scrumChannelOperationDescriptor, error) {
	descriptor, err := describeScrumChannelOperation(command.Request)
	if err != nil {
		return ScrumChannelOperationCommand{}, scrumChannelOperationDescriptor{}, err
	}
	command.Request = descriptor.Request
	if command.ExpectedCardUpdatedAt.IsZero() {
		return ScrumChannelOperationCommand{}, scrumChannelOperationDescriptor{}, fmt.Errorf("Scrum channel operation requires the observed card version")
	}
	command.ResultAction = strings.TrimSpace(command.ResultAction)
	command.ResultAgent = strings.TrimSpace(command.ResultAgent)
	if command.ResultAgent == "" {
		return ScrumChannelOperationCommand{}, scrumChannelOperationDescriptor{}, fmt.Errorf("Scrum channel operation requires a result agent")
	}
	if err := validateScrumChannelEffect(&command); err != nil {
		return ScrumChannelOperationCommand{}, scrumChannelOperationDescriptor{}, err
	}
	return command, descriptor, nil
}

func validateScrumChannelEffect(command *ScrumChannelOperationCommand) error {
	effect := &command.Effect
	switch effect.Kind {
	case ScrumChannelStartJob:
		if effect.JobID != 0 || command.ResultAction != "started" {
			return fmt.Errorf("start-job Scrum channel operation requires action started and no existing job")
		}
		instruction, _, err := validateLifecycleFeedback(effect.Instruction, "Scrum channel instruction")
		if err != nil {
			return err
		}
		effect.Instruction = instruction
		pipeline, err := validatePipeline(effect.Pipeline)
		if err != nil {
			return err
		}
		effect.Pipeline = pipeline
		if len(effect.Metadata) == 0 || !json.Valid(effect.Metadata) || effect.Metadata[0] != '{' {
			return fmt.Errorf("start-job Scrum channel operation requires JSON object metadata")
		}
	case ScrumChannelReplanJob:
		if effect.JobID <= 0 || (command.ResultAction != "steered" && command.ResultAction != "revised") {
			return fmt.Errorf("replan Scrum channel operation requires a job and action steered or revised")
		}
		if hasScrumChannelStartFields(*effect) {
			return fmt.Errorf("replan Scrum channel operation forbids start-job fields")
		}
	case ScrumChannelSubmitFeedback:
		if effect.JobID <= 0 || command.ResultAction != "feedback" {
			return fmt.Errorf("feedback Scrum channel operation requires a job and action feedback")
		}
		if hasScrumChannelStartFields(*effect) {
			return fmt.Errorf("feedback Scrum channel operation forbids start-job fields")
		}
	default:
		return fmt.Errorf("Scrum channel effect kind %q is not registered", effect.Kind)
	}
	return nil
}

func hasScrumChannelStartFields(effect ScrumChannelEffect) bool {
	return effect.Instruction != "" || effect.Pipeline != "" || len(effect.Metadata) != 0
}
