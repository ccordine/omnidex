package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

var ErrUnsupportedPipeline = errors.New("unsupported job pipeline")

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
		model.PipelineDataQuery,
		model.PipelineDataExplore,
		model.PipelineProjectDebugger,
		model.PipelineScrumCardLLM,
		model.PipelineScrum:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w %q", ErrUnsupportedPipeline, normalized)
	}
}

func isDataSourceQueryJob(metadataJSON []byte) bool {
	if len(metadataJSON) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(metadataJSON, &payload); err != nil {
		return false
	}
	return strings.TrimSpace(stringFromMetadata(payload["source"])) == "omni-data-source"
}

func stringFromMetadata(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func stepsForPipeline(pipeline string) []stepSeed {
	switch normalizePipeline(pipeline) {
	case model.PipelineAssistant, model.PipelineChat, model.PipelineStory:
		return v3ConversationSteps()
	case model.PipelineCoding:
		return []stepSeed{{action: "v3_coding", sortIndex: 5}}
	case model.PipelineDataQuery:
		return []stepSeed{{action: "data_source_query", sortIndex: 1}}
	case model.PipelineDataExplore:
		return []stepSeed{{action: "data_source_explore", sortIndex: 1}}
	case model.PipelineProjectDebugger:
		return []stepSeed{{action: "project_debugger", sortIndex: 1}}
	case model.PipelineScrumCardLLM:
		return []stepSeed{{action: "scrum_card_llm", sortIndex: 1}}
	default:
		return nil
	}
}
