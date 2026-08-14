package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestWorkerAcceptsOnlyCurrentExecutablePipelines(t *testing.T) {
	for _, pipeline := range []string{model.PipelineChat, model.PipelineCoding, model.PipelineScrum} {
		if err := requireExecutablePipeline(pipeline); err != nil {
			t.Fatalf("current pipeline %q error=%v", pipeline, err)
		}
	}
	for _, pipeline := range []string{"assistant", "story", "agent", "unknown", "CHAT", " chat"} {
		if err := requireExecutablePipeline(pipeline); err == nil || !strings.Contains(err.Error(), "unsupported executable pipeline") {
			t.Fatalf("retired pipeline %q error=%v", pipeline, err)
		}
	}
}

func TestConversationTurnAuthorityIsChatOnly(t *testing.T) {
	for _, pipeline := range []string{"assistant", "story", "agent", "CHAT", " chat"} {
		if _, err := newTurnAuthority(model.Job{ID: 1, Pipeline: pipeline, Instruction: "historical"}); err == nil {
			t.Fatalf("retired pipeline %q received conversation turn authority", pipeline)
		}
	}
}
