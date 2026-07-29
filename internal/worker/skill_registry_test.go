package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/specialists"
)

func TestBootstrapSkillSpecsRejectsRegistryIdentityMismatch(t *testing.T) {
	t.Parallel()

	_, err := bootstrapSkillSpecs(&specialists.Registry{Specs: map[string]specialists.Spec{
		"expected": {ID: "different", Purpose: "Test one boundary."},
	}})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("bootstrapSkillSpecs() error=%v, want identity mismatch", err)
	}
}

func TestServiceDoesNotExposeBootstrapFilesBeforeDatabaseActivation(t *testing.T) {
	t.Parallel()

	service := &Service{bootstrapRegistry: &specialists.Registry{Specs: map[string]specialists.Spec{
		"bootstrap": {ID: "bootstrap", Purpose: "Test one boundary."},
	}}}
	if _, ok := service.skillSpec("bootstrap"); ok {
		t.Fatal("filesystem bootstrap skill became runtime authority before database activation")
	}
}
