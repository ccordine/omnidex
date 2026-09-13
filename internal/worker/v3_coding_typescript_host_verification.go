package worker

import (
	"errors"

	"github.com/gryph/omnidex/internal/queue"
)

func (session *directCodingSession) verifyAuthoritativeTypeScriptWorkspace(program directCodingProgram, assembly directCodingAssembly) (resultErr error) {
	if err := session.runtime.svc.requireWorkspaceScopeForV3Job(session.runtime.ctx, session.runtime.claim.Job, session.root); err != nil {
		return err
	}
	if err := validateDirectCodingProgramAssembly(program, assembly); err != nil {
		return err
	}
	if err := validateDirectCodingAssembly(session.runtime.ctx, session.runtime.workspaceFence, assembly); err != nil {
		return err
	}
	workspace, err := openDirectCodingTypeScriptStageWorkspace(session, program, queue.VerificationHostInstall)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = errors.Join(resultErr, workspace.Close(), validateDirectCodingAssembly(session.runtime.ctx, session.runtime.workspaceFence, assembly))
	}()
	return workspace.Verify(&program, queue.VerificationHostFinal, directCodingFullTypeScriptStageCommands())
}
