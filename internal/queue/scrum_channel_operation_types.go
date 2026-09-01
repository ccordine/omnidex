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
}

type ScrumChannelOperationCommand struct {
	Request               ScrumChannelOperationRequest
	ExpectedCardUpdatedAt time.Time
	Effect                ScrumChannelEffect
	ResultAction          string
}

type ScrumChannelCardUpdate struct {
	Messages          []ScrumCardMessageAppend
	Column            string
	JobID             string
	PlayState         string
	QueueOrder        int
}

type ScrumChannelCardBuilder func(DBScrumCard, model.Job) (ScrumChannelCardUpdate, error)

type ScrumChannelOperationResult struct {
	OperationID  LifecycleOperationID
	Card         DBScrumCard
	Messages     []ScrumCardMessage
	MessageStart int64
	MessageTotal int64
	PreviousCard DBScrumCard
	Job          model.Job
	Action       string
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
	if request.CardID == "" || request.CardID != strings.TrimSpace(request.CardID) {
		return scrumChannelOperationDescriptor{}, fmt.Errorf("Scrum channel operation requires one canonical card ID")
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, request.Message); err != nil {
		return scrumChannelOperationDescriptor{}, fmt.Errorf("Scrum channel message: %w", err)
	}
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
	if command.ResultAction != strings.TrimSpace(command.ResultAction) {
		return ScrumChannelOperationCommand{}, scrumChannelOperationDescriptor{}, fmt.Errorf("Scrum channel result action is not canonical")
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
		if effect.Instruction != command.Request.Message {
			return fmt.Errorf("start-job Scrum channel instruction must equal the exact request message")
		}
	case ScrumChannelReplanJob:
		if effect.JobID <= 0 || command.ResultAction != "replanned" {
			return fmt.Errorf("replan Scrum channel operation requires a job and action replanned")
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
	return effect.Instruction != ""
}
