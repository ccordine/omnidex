package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

type nativeRuntimeV3 struct {
	svc                     *Service
	ctx                     context.Context
	claim                   *model.ClaimedStep
	action                  string
	routing                 ModelRouting
	routingErr              error
	routingOnce             sync.Once
	objectivePathProvenance assemblyline.ArtifactIdentityProvenance
	workspaceFence          *workspacefacts.MutationFence
	workspaceFenceRoot      string
}

func (s *Service) runNativeV3Step(
	ctx context.Context,
	claim *model.ClaimedStep,
	action string,
) error {
	if s == nil || claim == nil {
		return fmt.Errorf("native worker execution requires one claimed step")
	}
	if _, err := s.workspaceScopeForV3Job(claim.Job); err != nil {
		return fmt.Errorf("validate host workspace before action %q: %w", action, err)
	}
	runtime := &nativeRuntimeV3{
		svc: s, ctx: ctx, claim: claim,
		action: action,
	}
	return runtime.run()
}

func (r *nativeRuntimeV3) modelRouting() (ModelRouting, error) {
	if r == nil || r.svc == nil || r.claim == nil {
		return ModelRouting{}, fmt.Errorf("model routing requires runtime authority")
	}
	r.routingOnce.Do(func() {
		r.routing, r.routingErr = modelRoutingFromJobMetadata(r.claim.Job.Metadata)
	})
	return r.routing, r.routingErr
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
