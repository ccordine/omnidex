package host

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
	"github.com/jackc/pgx/v5"
)

func TestPostgresHostReconstructsRegisteredTransferSurfaces(t *testing.T) {
	for _, surface := range []string{
		labyrinth.FilesystemSurfaceVersionV1,
		labyrinth.RecordSurfaceVersionV1,
	} {
		t.Run(surface, func(t *testing.T) {
			fixture := newDurableFixture(t)
			environment, err := NewSurfaceEnvironment(
				fixture.Store, fixture.Episode, fixture.resolver(),
				func(_ context.Context, actor cognition.AttemptRef) error {
					if actor != fixture.Actor {
						return cognition.ErrAuthorityDenied
					}
					return nil
				},
				func(_ context.Context, _ pgx.Tx, actor cognition.AttemptRef) error {
					if actor != fixture.Actor {
						return cognition.ErrAuthorityDenied
					}
					return nil
				},
				surface,
			)
			if err != nil {
				t.Fatal(err)
			}
			current, err := environment.Start(t.Context(), fixture.Scenario.Ref())
			if err != nil {
				t.Fatal(err)
			}
			observations := append([]cognition.Observation(nil), current.Observations...)
			for index, witness := range fixture.Witness {
				schema, exists := fixture.Scenario.Catalog().Schema(witness.Request.Kind)
				if !exists {
					t.Fatalf("witness schema %q is absent", witness.Request.Kind)
				}
				var evidence []cognition.EvidenceRef
				if schema.EvidencePolicy == cognition.EvidenceRequired {
					evidence = make([]cognition.EvidenceRef, len(observations))
					for position, observation := range observations {
						evidence[position] = observation.EvidenceRef()
					}
				}
				action, err := cognition.NewRegisteredAction(
					cognition.ActionID("surface-action-"+string(rune('a'+index))),
					fixture.Actor, schema, witness.Request, evidence,
				)
				if err != nil {
					t.Fatal(err)
				}
				current, err = environment.Apply(t.Context(), fixture.Episode, current.Current, action)
				if err != nil {
					t.Fatalf("surface action %d: %v", index, err)
				}
				observations = append(observations, current.Observations...)
			}
			if !current.Terminal {
				t.Fatal("registered transfer surface did not reach its exact terminal state")
			}
		})
	}
}

func TestHostRejectsUnregisteredSurfaceWithoutFallback(t *testing.T) {
	if _, err := registeredKernelFactory("shell-if-confused.v1"); err == nil {
		t.Fatal("unregistered host surface fell back to symbolic execution")
	}
}
