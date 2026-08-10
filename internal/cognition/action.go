package cognition

import "fmt"

func (actor AttemptRef) Validate() error {
	if actor.JobID <= 0 {
		return fmt.Errorf("%w: job ID must be positive", ErrInvalidAttempt)
	}
	if actor.Generation <= 0 {
		return fmt.Errorf("%w: generation must be positive", ErrInvalidAttempt)
	}
	if actor.StepID <= 0 {
		return fmt.Errorf("%w: step ID must be positive", ErrInvalidAttempt)
	}
	if actor.Attempt == 0 {
		return fmt.Errorf("%w: attempt must be positive", ErrInvalidAttempt)
	}
	if err := validateIdentity(actor.WorkerID, "worker ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidAttempt, err)
	}
	return nil
}

func NewActionRequest(kind ActionKind, arguments []ActionArgument) (ActionRequest, error) {
	request := ActionRequest{Kind: kind, Arguments: cloneSlice(arguments)}
	if err := request.Validate(); err != nil {
		return ActionRequest{}, err
	}
	return request, nil
}

func (request ActionRequest) Validate() error {
	if err := validateIdentity(string(request.Kind), "action kind"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidAction, err)
	}
	if len(request.Arguments) > MaxActionArguments {
		return fmt.Errorf("%w: argument count exceeds %d", ErrInvalidAction, MaxActionArguments)
	}
	seen := make(map[ActionArgumentName]struct{}, len(request.Arguments))
	for index, argument := range request.Arguments {
		if err := validateIdentity(string(argument.Name), "action argument name"); err != nil {
			return fmt.Errorf("%w: argument %d: %v", ErrInvalidAction, index, err)
		}
		if _, duplicate := seen[argument.Name]; duplicate {
			return fmt.Errorf("%w: argument %q is duplicated", ErrInvalidAction, argument.Name)
		}
		seen[argument.Name] = struct{}{}
		if err := validateExactText(argument.Value, "action argument value", MaxActionValueBytes); err != nil {
			return fmt.Errorf("%w: argument %q: %v", ErrInvalidAction, argument.Name, err)
		}
	}
	return nil
}

func (request ActionRequest) Clone() ActionRequest {
	request.Arguments = cloneSlice(request.Arguments)
	return request
}

func (schema ActionSchema) ValidateRequest(request ActionRequest, evidenceRefs []EvidenceRef) error {
	if err := schema.Validate(); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Kind != schema.Kind {
		return fmt.Errorf("%w: request kind %q does not match schema kind %q", ErrInvalidAction, request.Kind, schema.Kind)
	}
	arguments := make(map[ActionArgumentName]ActionArgument, len(request.Arguments))
	for _, argument := range request.Arguments {
		arguments[argument.Name] = argument
	}
	for _, parameter := range schema.Parameters {
		argument, exists := arguments[parameter.Name]
		if parameter.Required && !exists {
			return fmt.Errorf("%w: required argument %q is missing", ErrInvalidAction, parameter.Name)
		}
		if exists && len(argument.Value) > parameter.MaxBytes {
			return fmt.Errorf("%w: argument %q exceeds its %d-byte schema limit", ErrInvalidAction, parameter.Name, parameter.MaxBytes)
		}
		delete(arguments, parameter.Name)
	}
	for name := range arguments {
		return fmt.Errorf("%w: argument %q is not registered by the schema", ErrInvalidAction, name)
	}
	if err := validateEvidenceRefs(evidenceRefs); err != nil {
		return err
	}
	switch schema.EvidencePolicy {
	case EvidenceRequired:
		if len(evidenceRefs) == 0 {
			return fmt.Errorf("%w: action schema requires evidence", ErrInvalidEvidence)
		}
	case EvidenceForbidden:
		if len(evidenceRefs) != 0 {
			return fmt.Errorf("%w: action schema forbids evidence", ErrInvalidEvidence)
		}
	case EvidenceOptional:
	default:
		return fmt.Errorf("%w: schema has an unregistered evidence policy", ErrInvalidActionSchema)
	}
	return nil
}

func NewRegisteredAction(
	id ActionID,
	actor AttemptRef,
	schema ActionSchema,
	request ActionRequest,
	evidenceRefs []EvidenceRef,
) (RegisteredAction, error) {
	action := RegisteredAction{
		ID: id, Actor: actor, Schema: schema.Ref(), Request: request.Clone(),
		EvidenceRefs: cloneSlice(evidenceRefs),
	}
	if err := action.Validate(schema); err != nil {
		return RegisteredAction{}, err
	}
	return action, nil
}

func (action RegisteredAction) Validate(schema ActionSchema) error {
	if err := action.validateBase(); err != nil {
		return err
	}
	if action.Schema != schema.Ref() {
		return fmt.Errorf("%w: registered schema identity does not match the supplied schema", ErrInvalidAction)
	}
	if err := schema.ValidateRequest(action.Request, action.EvidenceRefs); err != nil {
		return err
	}
	return nil
}

func (action RegisteredAction) validateBase() error {
	if err := validateIdentity(string(action.ID), "action ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidAction, err)
	}
	if err := action.Actor.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAction, err)
	}
	if err := action.Schema.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAction, err)
	}
	if err := action.Request.Validate(); err != nil {
		return err
	}
	if err := validateEvidenceRefs(action.EvidenceRefs); err != nil {
		return err
	}
	return nil
}

func (action RegisteredAction) Clone() RegisteredAction {
	action.Request = action.Request.Clone()
	action.EvidenceRefs = cloneSlice(action.EvidenceRefs)
	return action
}
