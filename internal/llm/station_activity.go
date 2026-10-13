package llm

import (
	"context"

	"github.com/gryph/omnidex/internal/model"
)

// ExactStationActivityClient exposes public observations from the same exact
// request. The observer cannot alter its prompt, result or execution authority.
type ExactStationActivityClient interface {
	GeneratePreparedExactWithActivity(context.Context, PreparedModel, func(model.AgentActivity) error) (PreparedGeneration, error)
}
