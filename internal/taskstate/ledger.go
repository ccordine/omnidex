package taskstate

import "fmt"

func NewLedger(id LedgerID, owner LedgerOwner) (*Ledger, error) {
	if err := validateLedgerID(id, owner); err != nil {
		return nil, err
	}
	return &Ledger{
		id: id, owner: owner, status: LedgerActive,
		nodes: make(map[NodeID]Node), edges: make(map[EdgeID]Edge),
		entries:           make(map[EntryID]Entry),
		nodeSupersessions: make(map[NodeID]NodeGenerationSupersession),
		commandEvents:     make(map[CommandID]Event),
	}, nil
}

func (ledger *Ledger) Apply(command Command) (Event, error) {
	if err := ledger.validateInitialized(); err != nil {
		return Event{}, err
	}
	descriptor, err := DescribeCommand(command)
	if err != nil {
		return Event{}, err
	}
	if existing, ok := ledger.commandEvents[descriptor.ID]; ok {
		if existing.CommandSHA256 != descriptor.SHA256 {
			return Event{}, fmt.Errorf("%w: command ID %q was reused with different content", ErrCommandIDConflict, descriptor.ID)
		}
		return cloneEvent(existing), nil
	}
	if descriptor.ExpectedVersion != ledger.version {
		return Event{}, VersionConflictError{Expected: descriptor.ExpectedVersion, Actual: ledger.version}
	}
	if ledger.status != LedgerActive {
		return Event{}, fmt.Errorf("%w: ledger is terminal with status %q", ErrInvalidState, ledger.status)
	}
	event, err := command.decide(ledger)
	if err != nil {
		return Event{}, err
	}
	event.LedgerID = ledger.id
	event.Version = ledger.version + 1
	event.CommandID = descriptor.ID
	event.CommandSHA256 = descriptor.SHA256
	event.CommandKind = descriptor.Kind
	event.Authority = descriptor.Actor
	if err := ValidateEventProjection(event); err != nil {
		return Event{}, fmt.Errorf("%w: generated event projection is invalid: %v", ErrInvalidState, err)
	}
	if err := ledger.applyEvent(event); err != nil {
		return Event{}, fmt.Errorf("%w: generated event is invalid: %v", ErrInvalidState, err)
	}
	stored := cloneEvent(event)
	ledger.events = append(ledger.events, stored)
	ledger.commandEvents[event.CommandID] = stored
	return cloneEvent(stored), nil
}

func (ledger *Ledger) validateInitialized() error {
	if ledger == nil {
		return fmt.Errorf("%w: ledger is required", ErrInvalidState)
	}
	if ledger.nodes == nil || ledger.edges == nil || ledger.entries == nil ||
		ledger.nodeSupersessions == nil || ledger.commandEvents == nil {
		return fmt.Errorf("%w: ledger aggregate is uninitialized", ErrInvalidState)
	}
	if err := validateLedgerID(ledger.id, ledger.owner); err != nil {
		return fmt.Errorf("%w: ledger aggregate identity is invalid: %v", ErrInvalidState, err)
	}
	if ledger.status != LedgerActive && !terminalLedger(ledger.status) {
		return fmt.Errorf("%w: ledger aggregate status %q is not registered", ErrInvalidState, ledger.status)
	}
	if err := ledger.validateAggregateCapacity(); err != nil {
		return err
	}
	return nil
}

func Reconstruct(id LedgerID, owner LedgerOwner, events []Event) (*Ledger, error) {
	ledger, err := NewLedger(id, owner)
	if err != nil {
		return nil, err
	}
	for index, supplied := range events {
		event := cloneEvent(supplied)
		if event.LedgerID != ledger.id {
			return nil, fmt.Errorf("%w: event %d belongs to ledger %q", ErrInvalidState, index, event.LedgerID)
		}
		if event.Version != ledger.version+1 {
			return nil, VersionConflictError{Expected: ledger.version + 1, Actual: event.Version}
		}
		if _, exists := ledger.commandEvents[event.CommandID]; exists {
			return nil, fmt.Errorf("%w: duplicate event command ID %q", ErrInvalidState, event.CommandID)
		}
		if err := ValidateEventProjection(event); err != nil {
			return nil, fmt.Errorf("%w: event %d: %v", ErrInvalidState, index, err)
		}
		if err := ledger.applyEvent(event); err != nil {
			return nil, fmt.Errorf("%w: event %d cannot replay: %v", ErrInvalidState, index, err)
		}
		ledger.events = append(ledger.events, cloneEvent(event))
		ledger.commandEvents[event.CommandID] = cloneEvent(event)
	}
	return ledger, nil
}

func validateCommandEventKinds(command CommandKind, event EventKind) error {
	want := map[CommandKind]EventKind{
		CommandAddNode: EventNodeAdded, CommandAddEdge: EventEdgeAdded,
		CommandAddEntry: EventEntryAdded, CommandRejectEntry: EventEntryRejected,
		CommandResolveEntry: EventEntryResolved, CommandSupersedeEntry: EventEntrySuperseded,
		CommandAcceptDecision: EventDecisionAccepted, CommandPromoteReady: EventNodesReadied,
		CommandAssignStep: EventNodeStepAssigned, CommandTransitionNode: EventNodeTransitioned,
		CommandSupersedeNodeGeneration: EventNodeGenerationSuperseded,
		CommandCloseLedger:             EventLedgerClosed,
	}
	expected, ok := want[command]
	if !ok || expected != event {
		return fmt.Errorf("command kind %q cannot produce event kind %q", command, event)
	}
	return nil
}
