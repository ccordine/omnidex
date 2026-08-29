package queue

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrum"
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
	if pipeline != normalized {
		return "", fmt.Errorf("%w %q: pipeline must be canonical", ErrUnsupportedPipeline, pipeline)
	}
	switch normalized {
	case model.PipelineChat,
		model.PipelineCoding,
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
	case model.PipelineCoding:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: pipeline %q", ErrChannelTransportRequired, normalized)
	}
}

func stepsForPipeline(pipeline string) []stepSeed {
	switch pipeline {
	case model.PipelineChat:
		return conversationObjectiveSteps()
	case model.PipelineCoding:
		return []stepSeed{{action: "v3_coding", sortIndex: 5}}
	default:
		return nil
	}
}

func stepsForJob(pipeline, instruction string, metadataJSON []byte) ([]stepSeed, error) {
	var err error
	pipeline, err = validatePipeline(pipeline)
	if err != nil {
		return nil, err
	}
	metadata := decodeMetadataObject(metadataJSON)
	if err := ValidateJobMetadataAuthority(metadata); err != nil {
		return nil, err
	}
	if pipeline == model.PipelineScrum {
		if _, err := scrum.DecodeStoredJobMetadata(metadataJSON); err != nil {
			return nil, err
		}
		return []stepSeed{{action: "v3_coding", sortIndex: 5}}, nil
	}
	if source, present := metadata["source"]; present && source == scrum.JobMetadataSource {
		return nil, fmt.Errorf("Scrum metadata source requires the typed Scrum enqueue boundary")
	}
	if pipeline == model.PipelineCoding {
		return stepsForPipeline(model.PipelineCoding), nil
	}
	switch pipeline {
	case model.PipelineChat:
		return conversationObjectiveSteps(), nil
	}
	return stepsForPipeline(pipeline), nil
}

func conversationObjectiveSteps() []stepSeed {
	return []stepSeed{{action: "objective_resolve", sortIndex: 5}}
}
