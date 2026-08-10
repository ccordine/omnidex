package labyrinth

import "github.com/gryph/omnidex/internal/cognition"

type SolverBounds struct {
	MaxStates        int
	MaxGroundActions int
}

type SolverResult struct {
	Actions        []cognition.ActionRequest
	Cost           int
	LowerBound     int
	ExpandedStates int
	Optimal        bool
}

const MaxSolverGroundActions = 8_192
