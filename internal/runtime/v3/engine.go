package v3

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/artifacts"
)

const (
	StageIntentParse     = "intent_parse"
	StageCapabilityAudit = "capability_audit"
	StageWorkspace       = "workspace_research"
	StageMemory          = "memory_retrieval"
	StagePlanning        = "planning"
	StageExternal        = "external_research"
	StageAnalysis        = "analysis"
	StageResponseDraft   = "response_draft"
	StageVerification    = "verification"
	StageFinalize        = "finalize"
)

type ArtifactWriter interface {
	WriteArtifact(ctx context.Context, artifact artifacts.Envelope) error
}

type Engine struct {
	Writer ArtifactWriter
}

type RunInput struct {
	JobID       int64
	StepID      int64
	Instruction string
	Pipeline    string
}

func (e *Engine) Bootstrap(ctx context.Context, in RunInput) error {
	if e == nil {
		return fmt.Errorf("v3 engine is nil")
	}
	if e.Writer == nil {
		return fmt.Errorf("v3 engine writer is nil")
	}
	intent, err := artifacts.MarshalPayload(artifacts.KindIntent, "1", artifacts.IntentArtifact{
		UserGoal: in.Instruction,
		Mode:     in.Pipeline,
	})
	if err != nil {
		return err
	}
	intent.JobID = in.JobID
	intent.StepID = in.StepID
	return e.Writer.WriteArtifact(ctx, intent)
}
