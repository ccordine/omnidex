package queue

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

var (
	ErrUnsupportedPipeline      = errors.New("unsupported job pipeline")
	ErrChannelTransportRequired = errors.New("free-form jobs require the channel message transport")
)

func normalizePipeline(pipeline string) string {
	return strings.ToLower(strings.TrimSpace(pipeline))
}

func validatePipeline(pipeline string) (string, error) {
	normalized := normalizePipeline(pipeline)
	switch normalized {
	case model.PipelineAssistant,
		model.PipelineChat,
		model.PipelineCoding,
		model.PipelineStory,
		model.PipelineScrum:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w %q", ErrUnsupportedPipeline, normalized)
	}
}

func validatePublicEnqueuePipeline(pipeline string) (string, error) {
	normalized, err := validatePipeline(pipeline)
	if err != nil {
		return "", err
	}
	switch normalized {
	case model.PipelineCoding, model.PipelineScrum:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: pipeline %q", ErrChannelTransportRequired, normalized)
	}
}

func stepsForPipeline(pipeline string) []stepSeed {
	switch normalizePipeline(pipeline) {
	case model.PipelineAssistant, model.PipelineChat, model.PipelineStory:
		return conversationObjectiveSteps()
	case model.PipelineCoding:
		return []stepSeed{{action: "v3_coding", sortIndex: 5}}
	default:
		return nil
	}
}
