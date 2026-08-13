package specialistworkflow

import (
	"cmp"
	"fmt"
	"slices"
)

const maxRegistryRegistrations = 128

type Registry struct {
	registrations []Registration
}

func NewRegistry(registrations []Registration) (Registry, error) {
	if len(registrations) == 0 {
		return Registry{}, ErrEmptyRegistry
	}
	if len(registrations) > maxRegistryRegistrations {
		return Registry{}, fmt.Errorf(
			"%w: %d registrations exceed the %d-entry hard limit",
			ErrInvalidRegistration, len(registrations), maxRegistryRegistrations,
		)
	}
	owned := make([]Registration, 0, len(registrations))
	for index, registration := range registrations {
		if err := registration.validate(); err != nil {
			return Registry{}, fmt.Errorf("registration %d: %w", index, err)
		}
		for previous := range owned {
			if owned[previous].workflow == registration.workflow {
				return Registry{}, fmt.Errorf(
					"%w: workflow %q", ErrDuplicateWorkflow, registration.workflow,
				)
			}
			if owned[previous].capability == registration.capability {
				return Registry{}, fmt.Errorf(
					"%w: capability %q", ErrAmbiguousCapability, registration.capability,
				)
			}
		}
		owned = append(owned, registration)
	}
	slices.SortFunc(owned, func(left, right Registration) int {
		if order := cmp.Compare(left.capability, right.capability); order != 0 {
			return order
		}
		if order := cmp.Compare(left.workflow, right.workflow); order != 0 {
			return order
		}
		return cmp.Compare(left.version, right.version)
	})
	return Registry{registrations: owned}, nil
}

func (registry Registry) Resolve(capability CapabilityID) (Registration, error) {
	if len(registry.registrations) == 0 {
		return Registration{}, ErrEmptyRegistry
	}
	if err := validateIdentity("capability", string(capability), maxIdentityBytes); err != nil {
		return Registration{}, err
	}
	var resolved Registration
	matches := 0
	for _, registration := range registry.registrations {
		if registration.capability != capability {
			continue
		}
		resolved = registration
		matches++
	}
	switch matches {
	case 0:
		return Registration{}, fmt.Errorf("%w: capability %q", ErrWorkflowNotFound, capability)
	case 1:
		return resolved, nil
	default:
		return Registration{}, fmt.Errorf("%w: capability %q", ErrAmbiguousCapability, capability)
	}
}
