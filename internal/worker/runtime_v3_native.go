package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

type nativeRuntimeV3 struct {
	svc                     *Service
	ctx                     context.Context
	claim                   *model.ClaimedStep
	action                  string
	contexts                map[string]string
	routing                 ModelRouting
	routingErr              error
	routingOnce             sync.Once
	objectivePathProvenance assemblyline.ArtifactIdentityProvenance
}

func (s *Service) runNativeV3Step(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string, action string) error {
	runtime := &nativeRuntimeV3{
		svc: s, ctx: ctx, claim: claim,
		action: action, contexts: contexts,
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
