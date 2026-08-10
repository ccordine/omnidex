package cognitiongauntlet

import "fmt"

type liveStalePort string

const (
	liveStalePolicyFinish     liveStalePort = "policy_finish"
	liveStaleReconcile        liveStalePort = "reconciliation"
	liveStaleEnvironmentApply liveStalePort = "environment_apply"
	liveStaleTerminal         liveStalePort = "terminal_progress"
	liveStaleErrorAttempt     string        = "stale_step_attempt"
	liveStaleErrorAuthority   string        = "authority_denied"
)

func liveStalePorts() []liveStalePort {
	return []liveStalePort{
		liveStalePolicyFinish,
		liveStaleReconcile,
		liveStaleEnvironmentApply,
		liveStaleTerminal,
	}
}

func (port liveStalePort) Validate() error {
	switch port {
	case liveStalePolicyFinish, liveStaleReconcile, liveStaleEnvironmentApply, liveStaleTerminal:
		return nil
	default:
		return fmt.Errorf("live stale port %q is not registered", port)
	}
}

func (port liveStalePort) expectedError() string {
	if port == liveStaleEnvironmentApply {
		return liveStaleErrorAuthority
	}
	return liveStaleErrorAttempt
}

func (port liveStalePort) writeClasses() int {
	if port == liveStaleReconcile {
		return 2
	}
	return 1
}
