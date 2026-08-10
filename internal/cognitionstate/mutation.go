package cognitionstate

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/taskstate"
)

type SourceKind string

const (
	SourceEnvironmentObservation SourceKind = "environment_observation"
	SourceActionFailure          SourceKind = "action_failure"
	SourceModelObservation       SourceKind = "model_observation"
	SourceModelHypothesis        SourceKind = "model_hypothesis"
	SourceModelQuestion          SourceKind = "model_question"
	SourceModelDecision          SourceKind = "model_decision_candidate"
	SourceModelObligation        SourceKind = "model_obligation_candidate"
	SourceModelPlanRevision      SourceKind = "model_plan_revision_candidate"
	SourceAcceptedFact           SourceKind = "accepted_fact"
)

type ReplayDescriptor struct {
	Schema          string
	ID              string
	SHA256          string
	SourceKind      SourceKind
	SourceSHA256    string
	LedgerID        taskstate.LedgerID
	LedgerSHA256    string
	ExpectedVersion uint64
	Actor           taskstate.Authority
	EntryID         taskstate.EntryID
	CommandID       taskstate.CommandID
	CommandSHA256   string
}

type EntryMutation struct {
	descriptor ReplayDescriptor
	command    taskstate.AddEntryCommand
}

func (mutation EntryMutation) Descriptor() ReplayDescriptor { return mutation.descriptor }

func (mutation EntryMutation) Command() taskstate.AddEntryCommand {
	command := mutation.command
	command.Refs = append([]taskstate.Ref(nil), command.Refs...)
	return command
}

func (descriptor ReplayDescriptor) Validate(command taskstate.AddEntryCommand) error {
	if descriptor.Schema != EntryMappingSchemaV1 || !strings.HasPrefix(descriptor.ID, "cognition_mapping_") ||
		!validMappingDigest(descriptor.SHA256) || descriptor.ID != "cognition_mapping_"+descriptor.SHA256 ||
		!validMappingDigest(descriptor.SourceSHA256) || !validMappingDigest(descriptor.LedgerSHA256) {
		return fmt.Errorf("%w: replay identity is invalid", ErrInvalidMapping)
	}
	if !registeredSourceKind(descriptor.SourceKind) || descriptor.LedgerID == "" ||
		descriptor.Actor != command.Actor || descriptor.EntryID != command.ID ||
		descriptor.CommandID != command.CommandID || descriptor.ExpectedVersion != command.ExpectedVersion {
		return fmt.Errorf("%w: replay authority disagrees with its command", ErrInvalidMapping)
	}
	commandDescriptor, err := taskstate.DescribeCommand(command)
	if err != nil || commandDescriptor.SHA256 != descriptor.CommandSHA256 {
		return fmt.Errorf("%w: replay command identity is invalid", ErrInvalidMapping)
	}
	expected, err := replayDescriptorSHA(descriptor)
	if err != nil || expected != descriptor.SHA256 {
		return fmt.Errorf("%w: replay hash does not bind the exact descriptor", ErrInvalidMapping)
	}
	return nil
}

func (mutation EntryMutation) Validate() error {
	return mutation.descriptor.Validate(mutation.Command())
}

type entryCommandInput struct {
	Ledger          taskstate.MaterializedState
	ScopeNodeID     taskstate.NodeID
	SourceKind      SourceKind
	Source          any
	Actor           taskstate.Authority
	Kind            taskstate.EntryKind
	Content         string
	Refs            []taskstate.Ref
	Metadata        map[string]any
	ExpectedVersion uint64
}

func newEntryMutation(input entryCommandInput) (EntryMutation, error) {
	if err := taskstate.ValidateMaterializedState(input.Ledger); err != nil {
		return EntryMutation{}, fmt.Errorf("%w: ledger: %v", ErrInvalidMapping, err)
	}
	if !validMappedContent(input.Content) {
		return EntryMutation{}, fmt.Errorf("%w: entry content is invalid", ErrInvalidMapping)
	}
	sourceSHA, err := mappingDigest(input.Source)
	if err != nil {
		return EntryMutation{}, err
	}
	ledgerSHA, err := mappingDigest(input.Ledger)
	if err != nil {
		return EntryMutation{}, err
	}
	entryDigest, err := mappingDigest(struct {
		Schema, SHA256, LedgerSHA string
		Source                    SourceKind
		Ledger                    taskstate.LedgerID
		Scope                     taskstate.NodeID
	}{EntryMappingSchemaV1, sourceSHA, ledgerSHA, input.SourceKind, input.Ledger.ID, input.ScopeNodeID})
	if err != nil {
		return EntryMutation{}, err
	}
	entryID := taskstate.EntryID("cognition_entry_" + entryDigest)
	commandID, err := taskstate.NewCommandID(
		EntryMappingSchemaV1, string(input.SourceKind), sourceSHA, string(input.Ledger.ID),
		ledgerSHA, strconv.FormatUint(input.ExpectedVersion, 10), string(entryID),
	)
	if err != nil {
		return EntryMutation{}, fmt.Errorf("%w: command identity: %v", ErrInvalidMapping, err)
	}
	entryMetadata := make(map[string]any, len(input.Metadata)+3)
	for key, value := range input.Metadata {
		entryMetadata[key] = value
	}
	entryMetadata["schema"] = EntryMappingSchemaV1
	entryMetadata["source_kind"] = input.SourceKind
	entryMetadata["source_sha256"] = sourceSHA
	metadataValue, err := metadata(entryMetadata)
	if err != nil {
		return EntryMutation{}, err
	}
	command := taskstate.AddEntryCommand{
		CommandID: commandID, ExpectedVersion: input.ExpectedVersion,
		Actor: input.Actor, ID: entryID, ScopeNodeID: input.ScopeNodeID, Kind: input.Kind,
		Content: input.Content, Metadata: metadataValue,
		Refs: append([]taskstate.Ref(nil), input.Refs...),
	}
	commandDescriptor, err := taskstate.DescribeCommand(command)
	if err != nil {
		return EntryMutation{}, fmt.Errorf("%w: command: %v", ErrInvalidMapping, err)
	}
	descriptor := ReplayDescriptor{
		Schema: EntryMappingSchemaV1, SourceKind: input.SourceKind, SourceSHA256: sourceSHA,
		LedgerID: input.Ledger.ID, LedgerSHA256: ledgerSHA, ExpectedVersion: input.ExpectedVersion,
		Actor: input.Actor, EntryID: entryID, CommandID: commandID,
		CommandSHA256: commandDescriptor.SHA256,
	}
	descriptor.SHA256, err = replayDescriptorSHA(descriptor)
	if err != nil {
		return EntryMutation{}, err
	}
	descriptor.ID = "cognition_mapping_" + descriptor.SHA256
	mutation := EntryMutation{descriptor: descriptor, command: command}
	if err := mutation.Validate(); err != nil {
		return EntryMutation{}, err
	}
	return mutation, nil
}

func replayDescriptorSHA(descriptor ReplayDescriptor) (string, error) {
	return mappingDigest(struct {
		Schema, SourceKind, SourceSHA, Ledger, LedgerSHA, Version, Actor, Entry, Command, CommandSHA string
	}{descriptor.Schema, string(descriptor.SourceKind), descriptor.SourceSHA256, string(descriptor.LedgerID),
		descriptor.LedgerSHA256, strconv.FormatUint(descriptor.ExpectedVersion, 10), string(descriptor.Actor), string(descriptor.EntryID),
		string(descriptor.CommandID), descriptor.CommandSHA256})
}

func registeredSourceKind(kind SourceKind) bool {
	switch kind {
	case SourceEnvironmentObservation, SourceActionFailure, SourceModelObservation,
		SourceModelHypothesis, SourceModelQuestion, SourceModelDecision, SourceModelObligation,
		SourceModelPlanRevision, SourceAcceptedFact:
		return true
	default:
		return false
	}
}

func validateEntryMutation(state taskstate.MaterializedState, mutation EntryMutation) error {
	ledger, err := taskstate.RestoreLedger(state)
	if err != nil {
		return fmt.Errorf("%w: ledger: %v", ErrInvalidMapping, err)
	}
	if _, err := ledger.Apply(mutation.Command()); err != nil {
		return fmt.Errorf("%w: apply mapped entry: %v", ErrInvalidMapping, err)
	}
	return nil
}
