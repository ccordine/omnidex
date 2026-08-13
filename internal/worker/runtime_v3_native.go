package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
)

type nativeRuntimeV3 struct {
	svc      *Service
	ctx      context.Context
	claim    *model.ClaimedStep
	action   string
	contexts map[string]string
	routing  ModelRouting
}

func (s *Service) runNativeV3Step(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string, action string) error {
	routing, err := modelRoutingFromJobMetadata(claim.Job.Metadata, s.models)
	if err != nil {
		return err
	}
	runtime := &nativeRuntimeV3{
		svc: s, ctx: ctx, claim: claim,
		action: action, contexts: contexts, routing: routing,
	}
	return runtime.run()
}

func (r *nativeRuntimeV3) run() error {
	switch r.action {
	case "objective_resolve":
		return r.runObjectiveResolve()
	case "v3_coding":
		return r.runDirectCodingAction()
	default:
		return fmt.Errorf("worker action %q is not registered", r.action)
	}
}
