package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/config"
)

func TestNewRejectsMissingDatabaseBeforeRuntimeConstruction(t *testing.T) {
	_, err := New(context.Background(), config.Config{}, nil)
	if err == nil || !strings.Contains(err.Error(), "requires DATABASE_URL") {
		t.Fatalf("runtime error=%v", err)
	}
}
