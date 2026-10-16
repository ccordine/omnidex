package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/projectroot"
)

type codingAgentEnvironment struct {
	runtime        *nativeRuntimeV3
	root, identity string
}

func (environment *codingAgentEnvironment) Attest(ctx context.Context) error {
	if err := environment.runtime.svc.repo.RequireActiveStepAttempt(ctx, environment.runtime.claim.Authority); err != nil {
		return err
	}
	identity, err := projectroot.DirectoryIdentity(environment.root)
	if err != nil || identity != environment.identity {
		return fmt.Errorf("native agent workspace identity changed: %w", err)
	}
	return environment.runtime.workspaceFence.Reattest(environment.root)
}
