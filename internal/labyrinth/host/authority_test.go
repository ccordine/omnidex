package host

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func TestActionRequestIdentityExcludesActorButBindsMeaning(t *testing.T) {
	fixture := newDurableFixture(t)
	action := fixture.action(t, 0, "identity-action", fixture.Actor)
	replacement := action.Clone()
	replacement.Actor.Attempt++
	replacement.Actor.WorkerID = "replacement-worker"
	first, err := actionRequestSHA256(action)
	if err != nil {
		t.Fatal(err)
	}
	second, err := actionRequestSHA256(replacement)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("replacement actor changed semantic action identity")
	}
	changed := fixture.action(t, 1, action.ID, fixture.Actor)
	third, err := actionRequestSHA256(changed)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("changed action content retained semantic action identity")
	}
}

func TestPublicReceiptsCannotRepresentOracleState(t *testing.T) {
	receipt := EpisodeReceipt{
		Episode:  cognition.EpisodeRef{ID: "public-episode"},
		Scenario: cognition.ScenarioRef{ID: "public-scenario", SHA256: strings.Repeat("a", 64)},
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"oracle", "witness", "seed", "definition"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Fatalf("public receipt contains forbidden field %q: %s", forbidden, raw)
		}
	}
}

func TestUnauthorizedActorCannotReplayCommittedReceipt(t *testing.T) {
	fixture := newDurableFixture(t)
	environment := fixture.environment(t, func(actor cognition.AttemptRef) bool { return actor == fixture.Actor })
	started, err := environment.Start(context.Background(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	action := fixture.action(t, 0, "actor-fenced-action", fixture.Actor)
	if _, err := environment.Apply(context.Background(), fixture.Episode, started.Current, action); err != nil {
		t.Fatal(err)
	}
	stale := action.Clone()
	stale.Actor.WorkerID = "stale-worker"
	_, err = environment.Apply(context.Background(), fixture.Episode, started.Current, stale)
	if !errors.Is(err, cognition.ErrAuthorityDenied) {
		t.Fatalf("stale replay error = %v, want authority denial", err)
	}
	if errors.Is(err, labyrinth.ErrReplayConflict) {
		t.Fatalf("authority was checked after receipt content: %v", err)
	}
}
