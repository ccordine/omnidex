package queue

import (
	"context"

	"github.com/gryph/omnidex/internal/model"
)

// InterruptJob performs the same immutable generation cutover as an explicit
// replan. Reusing the running step ID would let the retired worker and its
// replacement share authority during the control-poll race.
func (r *Repository) InterruptJob(ctx context.Context, command ReplanJobCommand) (model.Job, error) {
	return r.ReplanJob(ctx, command)
}
