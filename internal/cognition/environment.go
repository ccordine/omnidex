package cognition

import "context"

// Environment is the complete production boundary between cognition and a
// world. Implementations expose state only through returned observations.
type Environment interface {
	Start(ctx context.Context, scenario ScenarioRef) (Transition, error)
	Apply(
		ctx context.Context,
		episode EpisodeRef,
		expected WorldRevision,
		action RegisteredAction,
	) (Transition, error)
}
