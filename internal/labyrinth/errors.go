package labyrinth

import "errors"

var (
	ErrAlreadyStarted         = errors.New("symbolic environment already started")
	ErrArtifactSeparation     = errors.New("public and private generated artifacts must remain separate")
	ErrGeneration             = errors.New("deterministic symbolic generation failed")
	ErrInvalidGeneratorConfig = errors.New("invalid deterministic generator configuration")
	ErrInvalidDefinition      = errors.New("invalid symbolic world definition")
	ErrNotStarted             = errors.New("symbolic environment not started")
	ErrObservationLimit       = errors.New("symbolic public observation limit exceeded")
	ErrPrecondition           = errors.New("symbolic action precondition failed")
	ErrPrivateSerialization   = errors.New("private symbolic definition cannot use public JSON serialization")
	ErrReplayConflict         = errors.New("symbolic action replay conflict")
	ErrSurfaceClosed          = errors.New("symbolic environment surface is closed")
	ErrSurfaceLimit           = errors.New("symbolic environment surface limit exceeded")
	ErrSurfaceOperation       = errors.New("symbolic environment surface operation failed")
	ErrSurfacePrecondition    = errors.New("symbolic environment surface precondition failed")
	ErrSolverLimit            = errors.New("symbolic solver state limit exceeded")
	ErrUnsolvable             = errors.New("symbolic world has no legal terminal path")
	ErrTerminal               = errors.New("symbolic environment is terminal")
	ErrTransitionLimit        = errors.New("symbolic environment transition limit exceeded")
	ErrWorldLimit             = errors.New("symbolic world state limit exceeded")
)
