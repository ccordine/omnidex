package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/model"
)

func TestNewRejectsInvalidCodingScopeModeBeforeRuntimeConstruction(t *testing.T) {
	_, err := New(context.Background(), config.Config{
		CodingScopeMode: model.CodingScopeMode("wide"),
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "runtime coding scope authority") {
		t.Fatalf("runtime error=%v", err)
	}
}
