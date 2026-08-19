package roleplay

import "errors"

var (
	ErrSimulationNotConfigured = errors.New("roleplay simulation is not configured")
	ErrSimulationStaleRevision = errors.New("roleplay simulation revision is stale")
	ErrSimulationConflict      = errors.New("roleplay simulation definition conflicts with existing authority")
	ErrSimulationUnknown       = errors.New("roleplay simulation name is unknown")
	ErrSimulationAmbiguous     = errors.New("roleplay simulation name is ambiguous")
	ErrSimulationIllegal       = errors.New("roleplay simulation action is illegal")
)
