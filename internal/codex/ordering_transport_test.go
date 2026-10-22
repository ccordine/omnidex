package codex

import (
	"context"
	"testing"
)

func TestOrderingUsesScopedInstructionIsolatedParser(t *testing.T) {
	fixture, _, cancel := parserFixture(t, true)
	defer cancel()
	native, err := New(fixture.command, fixture.directory)
	if err != nil {
		t.Fatal(err)
	}
	transport, cleanup, err := native.OrderingClient(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	parser, ok := transport.(*Client)
	if !ok || parser.parser == nil || native.parser != nil {
		t.Fatal("ordering shares native transport state")
	}
	prepared := parserRequest(t, parser, "one ordering question")
	if _, err := transport.GeneratePreparedExact(t.Context(), prepared); err != nil {
		t.Fatal(err)
	}
	cleanup()
	if !parser.parser.closed {
		t.Fatal("ordering process not retired")
	}
}
