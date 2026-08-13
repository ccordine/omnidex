package specialistworkflow_test

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/specialistworkflow"
)

func TestRegistryResolvesOneExactWorkflowAndOwnsItsInput(t *testing.T) {
	registration, err := specialistworkflow.NewRegistration(
		"rendered.observation", "browser.observer", "1",
	)
	if err != nil {
		t.Fatal(err)
	}
	registrations := []specialistworkflow.Registration{registration}
	registry, err := specialistworkflow.NewRegistry(registrations)
	if err != nil {
		t.Fatal(err)
	}
	registrations[0], _ = specialistworkflow.NewRegistration("changed", "changed", "2")

	resolved, err := registry.Resolve("rendered.observation")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Capability() != "rendered.observation" ||
		resolved.Workflow() != "browser.observer" || resolved.Version() != "1" {
		t.Fatalf("resolved registration drifted: %#v", resolved)
	}
}

func TestRegistryRejectsAmbiguousAndDuplicateAuthority(t *testing.T) {
	first, _ := specialistworkflow.NewRegistration("observe", "browser", "1")
	secondProducer, _ := specialistworkflow.NewRegistration("observe", "compiler", "1")
	if _, err := specialistworkflow.NewRegistry([]specialistworkflow.Registration{
		first, secondProducer,
	}); !errors.Is(err, specialistworkflow.ErrAmbiguousCapability) {
		t.Fatalf("ambiguous capability error=%v", err)
	}

	secondUse, _ := specialistworkflow.NewRegistration("compile", "browser", "1")
	if _, err := specialistworkflow.NewRegistry([]specialistworkflow.Registration{
		first, secondUse,
	}); !errors.Is(err, specialistworkflow.ErrDuplicateWorkflow) {
		t.Fatalf("duplicate workflow error=%v", err)
	}
}

func TestRegistryAndRegistrationFailLoudlyForInvalidIdentity(t *testing.T) {
	for name, values := range []struct {
		capability specialistworkflow.CapabilityID
		workflow   specialistworkflow.WorkflowID
		version    string
	}{
		{"", "workflow", "1"},
		{" capability", "workflow", "1"},
		{"capability", "work flow", "1"},
		{"capability", "workflow", ""},
		{"capability", "workflow", " 1"},
	} {
		if _, err := specialistworkflow.NewRegistration(
			values.capability, values.workflow, values.version,
		); !errors.Is(err, specialistworkflow.ErrInvalidRegistration) {
			t.Fatalf("case %d invalid registration error=%v", name, err)
		}
	}
	if _, err := specialistworkflow.NewRegistry(nil); !errors.Is(err, specialistworkflow.ErrEmptyRegistry) {
		t.Fatalf("empty registry error=%v", err)
	}
	if _, err := (specialistworkflow.Registry{}).Resolve("observe"); !errors.Is(err, specialistworkflow.ErrEmptyRegistry) {
		t.Fatalf("zero-value registry error=%v", err)
	}
	registration, _ := specialistworkflow.NewRegistration("observe", "browser", "1")
	registry, err := specialistworkflow.NewRegistry([]specialistworkflow.Registration{registration})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("compile"); !errors.Is(err, specialistworkflow.ErrWorkflowNotFound) {
		t.Fatalf("missing workflow error=%v", err)
	}
}

func TestRegistryRejectsOversizedInventoryBeforeOwnershipCopy(t *testing.T) {
	registrations := make([]specialistworkflow.Registration, 129)
	if _, err := specialistworkflow.NewRegistry(registrations); !errors.Is(
		err, specialistworkflow.ErrInvalidRegistration,
	) {
		t.Fatalf("oversized registry error=%v", err)
	}
}
