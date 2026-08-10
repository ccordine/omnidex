package host

import (
	"context"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitiontransport"
)

func TestDurableHostRunsBehindCognitionTransport(t *testing.T) {
	fixture := newDurableFixture(t)
	environment := fixture.environment(t, func(actor cognition.AttemptRef) bool { return actor == fixture.Actor })
	authenticator, err := cognitiontransport.NewBearerAuthenticator("durable-host-token")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := cognitiontransport.NewHandler(environment, environment, authenticator)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := cognitiontransport.NewClient(server.URL, "durable-host-token", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.Start(context.Background(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	action := fixture.action(t, 0, "transport-action", fixture.Actor)
	transition, err := client.Apply(context.Background(), fixture.Episode, started.Current, action)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := fixture.Store.Action(context.Background(), fixture.Episode, action.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Transition == nil || !reflect.DeepEqual(*stored.Transition, transition) {
		t.Fatalf("transport response is not the durable receipt: %#v", stored)
	}
}
