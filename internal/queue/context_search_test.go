package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestContextSearchAcceptsOneExactFourKiBInstruction(t *testing.T) {
	exact := " " + strings.Repeat("x", model.MaxFreeFormTurnBytes-1)
	if err := validateContextSearchRequest([]string{exact}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestContextSearchRejectsAlternateQueryLists(t *testing.T) {
	if err := validateContextSearchRequest([]string{"first", "second"}, 1); err == nil {
		t.Fatal("multiple context search queries were accepted")
	}
}
