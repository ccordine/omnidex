package queue

import (
	"fmt"
	"math"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
)

// VerifyCognitionInitialObligationTraceAuthority reuses the sole durable
// initial-graph derivation. The caller supplies the independently derived root
// contract; a self-consistent replacement graph or command identity is not
// sufficient.
func VerifyCognitionInitialObligationTraceAuthority(
	episodeID cognition.EpisodeID,
	actor cognition.AttemptRef,
	root cognition.ObligationSpec,
	recordID string,
	graph cognition.ObligationGraphSnapshot,
) error {
	if (cognition.EpisodeRef{ID: episodeID}).Validate() != nil ||
		actor.Validate() != nil || actor.Attempt > math.MaxInt64 ||
		graph.Validate() != nil {
		return fmt.Errorf("%w: initial cognition graph verifier input is invalid", ErrCognitionConflict)
	}
	want, descriptor, err := initialCognitionObligationGraph(CognitionEpisodeStart{
		Authority: model.StepAttemptAuthority{
			JobID: actor.JobID, Generation: actor.Generation, StepID: actor.StepID,
			Attempt: int64(actor.Attempt), WorkerID: actor.WorkerID,
		},
		EpisodeID: episodeID,
		Root:      root,
	})
	if err != nil || descriptor.ID != recordID || !reflect.DeepEqual(want, graph) {
		return fmt.Errorf("%w: initial cognition graph differs from exact derivation", ErrCognitionConflict)
	}
	return nil
}
