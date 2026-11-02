package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

type cleanupProvider struct {
	calls    int
	root     string
	job      int64
	accepted []string
	failure  error
}

func (*cleanupProvider) Run(context.Context, codingagent.Assignment, func(codingagent.NativeEvent) error) (codingagent.Result, error) {
	panic("cleanup must not call a model")
}
func (p *cleanupProvider) CleanupWorkflow(_ context.Context, root string, job int64, accepted []string) error {
	p.calls++
	p.root = root
	p.job = job
	p.accepted = append([]string{}, accepted...)
	return p.failure
}

func TestWorkflowCleanupUsesAcceptedScopeWithoutModelCallOrFailureGate(t *testing.T) {
	provider := &cleanupProvider{failure: errors.New("temporary Docker outage")}
	controller := &codingagent.Controller{Root: "/existing/workspace", JobID: 65, Providers: map[string]codingagent.Provider{"native": provider}}
	runtime := &nativeRuntimeV3{}
	runtime.cleanupCodingWorkflow(t.Context(), controller, []string{"accepted-item"})
	if provider.calls != 1 || provider.root != controller.Root || provider.job != 65 || len(provider.accepted) != 1 || provider.accepted[0] != "accepted-item" {
		t.Fatal("cleanup scope differs from the accepted task")
	}
	provider.failure = nil
	runtime.cleanupCodingWorkflow(t.Context(), controller, []string{"accepted-item"})
	if provider.calls != 2 {
		t.Fatal("cleanup failure prevented a later retry")
	}
}
