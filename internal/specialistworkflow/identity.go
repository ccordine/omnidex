package specialistworkflow

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxIdentityBytes = 128
	maxVersionBytes  = 64
)

var (
	ErrInvalidRegistration    = errors.New("invalid specialist workflow registration")
	ErrEmptyRegistry          = errors.New("specialist workflow registry is empty")
	ErrAmbiguousCapability    = errors.New("specialist capability has multiple workflow authorities")
	ErrDuplicateWorkflow      = errors.New("specialist workflow is registered more than once")
	ErrWorkflowNotFound       = errors.New("specialist capability has no registered workflow")
	ErrInvalidContract        = errors.New("invalid specialist workflow contract")
	ErrInvalidAttemptBudget   = errors.New("invalid specialist workflow attempt budget")
	ErrAttemptBudgetExhausted = errors.New("specialist workflow attempt budget exhausted")
	ErrNilContext             = errors.New("specialist workflow context is nil")
	ErrInvalidBoundedValue    = errors.New("invalid specialist workflow bounded value")
)

type CapabilityID string

type WorkflowID string

type Registration struct {
	capability CapabilityID
	workflow   WorkflowID
	version    string
}

func NewRegistration(
	capability CapabilityID,
	workflow WorkflowID,
	version string,
) (Registration, error) {
	registration := Registration{
		capability: capability,
		workflow:   workflow,
		version:    version,
	}
	if err := registration.validate(); err != nil {
		return Registration{}, err
	}
	return registration, nil
}

func (registration Registration) Capability() CapabilityID {
	return registration.capability
}

func (registration Registration) Workflow() WorkflowID {
	return registration.workflow
}

func (registration Registration) Version() string {
	return registration.version
}

func (registration Registration) validate() error {
	if err := validateIdentity("capability", string(registration.capability), maxIdentityBytes); err != nil {
		return err
	}
	if err := validateIdentity("workflow", string(registration.workflow), maxIdentityBytes); err != nil {
		return err
	}
	if err := validateIdentity("version", registration.version, maxVersionBytes); err != nil {
		return err
	}
	return nil
}

func validateIdentity(name, value string, maximum int) error {
	if value == "" {
		return fmt.Errorf("%w: %s is empty", ErrInvalidRegistration, name)
	}
	if len(value) > maximum {
		return fmt.Errorf("%w: %s exceeds %d bytes", ErrInvalidRegistration, name, maximum)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalidRegistration, name)
	}
	if strings.IndexFunc(value, func(character rune) bool {
		return unicode.IsSpace(character) || unicode.IsControl(character)
	}) >= 0 {
		return fmt.Errorf("%w: %s contains whitespace or control characters", ErrInvalidRegistration, name)
	}
	return nil
}
