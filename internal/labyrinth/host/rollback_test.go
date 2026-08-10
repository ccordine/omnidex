package host

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func TestPostgresReceiptFailureDiscardsCandidateMutation(t *testing.T) {
	fixture := newDurableFixture(t)
	environment := fixture.environment(t, func(actor cognition.AttemptRef) bool { return actor == fixture.Actor })
	started, err := environment.Start(context.Background(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	action := fixture.action(t, 0, "force-db-rollback", fixture.Actor)
	functionName := fmt.Sprintf("reject_receipt_%d", fixture.Actor.JobID)
	triggerName := fmt.Sprintf("reject_receipt_trigger_%d", fixture.Actor.JobID)
	functionIdentifier := pgx.Identifier{"labyrinth_host", functionName}.Sanitize()
	triggerIdentifier := pgx.Identifier{triggerName}.Sanitize()
	createFunction := fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$
		BEGIN
			IF NEW.action_id = 'force-db-rollback' THEN
				RAISE EXCEPTION 'forced durable receipt failure';
			END IF;
			RETURN NEW;
		END
		$body$`, functionIdentifier)
	if _, err := fixture.Pool.Exec(context.Background(), createFunction); err != nil {
		t.Fatal(err)
	}
	createTrigger := fmt.Sprintf(`
		CREATE TRIGGER %s BEFORE INSERT ON labyrinth_host.action_receipts
		FOR EACH ROW EXECUTE FUNCTION %s()`, triggerIdentifier, functionIdentifier)
	if _, err := fixture.Pool.Exec(context.Background(), createTrigger); err != nil {
		t.Fatal(err)
	}
	dropTrigger := fmt.Sprintf(`DROP TRIGGER %s ON labyrinth_host.action_receipts`, triggerIdentifier)
	dropFunction := fmt.Sprintf(`DROP FUNCTION %s()`, functionIdentifier)
	dropped := false
	t.Cleanup(func() {
		if !dropped {
			_, _ = fixture.Pool.Exec(context.Background(), dropTrigger)
			_, _ = fixture.Pool.Exec(context.Background(), dropFunction)
		}
	})
	if _, err := environment.Apply(context.Background(), fixture.Episode, started.Current, action); err == nil {
		t.Fatal("forced database receipt failure unexpectedly succeeded")
	}
	if _, err := fixture.Store.Action(context.Background(), fixture.Episode, action.ID); !errors.Is(err, ErrReceiptNotFound) {
		t.Fatalf("rolled-back action receipt error = %v", err)
	}
	head, err := fixture.Store.Episode(context.Background(), fixture.Episode)
	if err != nil {
		t.Fatal(err)
	}
	if head.Current != started.Current {
		t.Fatalf("failed commit advanced durable head: %#v", head.Current)
	}
	if _, err := fixture.Pool.Exec(context.Background(), dropTrigger); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Pool.Exec(context.Background(), dropFunction); err != nil {
		t.Fatal(err)
	}
	dropped = true
	transition, err := environment.Apply(context.Background(), fixture.Episode, started.Current, action)
	if err != nil {
		t.Fatalf("same action after database recovery: %v", err)
	}
	if transition.Current.Number != 2 {
		t.Fatalf("candidate state leaked across rollback: revision=%d", transition.Current.Number)
	}
}
