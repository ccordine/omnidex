package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	runtimecoding "github.com/gryph/omnidex/internal/runtime/coding"
)

type codingWorkflowRunner interface {
	Run(context.Context, runtimecoding.Request) (runtimecoding.Result, error)
}

type stepCompleteFunc func(context.Context, int64, string, string, string) error

type nativeV3StepRunner func(context.Context, *model.ClaimedStep, map[string]string, string) error

type agentRuntimeStepRunner func(context.Context, *model.ClaimedStep, map[string]string, string) error

func (s *Service) runCodingWorkflowStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	engine := s.codingEngine
	if engine == nil {
		engine = runtimecoding.NewDeterministicEngine()
	}

	result, err := engine.Run(ctx, runtimecoding.Request{
		Goal:      claim.Job.Instruction,
		Contexts:  contexts,
		Workspace: codingWorkspaceForJob(claim.Job),
	})
	if err != nil {
		return err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal coding workflow result: %w", err)
	}
	output := strings.TrimSpace(result.Summary)
	if output == "" {
		output = string(resultJSON)
	}
	completeStep := s.completeStep
	if completeStep == nil {
		if s.repo == nil {
			return fmt.Errorf("coding workflow step completer is nil")
		}
		completeStep = s.repo.CompleteStep
	}
	return completeStep(ctx, claim.Step.ID, output, "coding_workflow", string(resultJSON))
}

func codingWorkspaceForJob(job model.Job) string {
	if cwd := clientCWDForJob(job); strings.TrimSpace(cwd) != "" {
		return cwd
	}
	return metadataString(job.Metadata, "workspace")
}

func isCodingJob(job model.Job) bool {
	return strings.EqualFold(strings.TrimSpace(job.Pipeline), model.PipelineCoding)
}
