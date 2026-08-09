package workingset

import "github.com/gryph/omnidex/internal/taskstate"

type CommandKind string

const (
	CommandAcquire         CommandKind = "acquire"
	CommandRetain          CommandKind = "retain"
	CommandRelease         CommandKind = "release"
	CommandTouch           CommandKind = "touch"
	CommandInvalidateStale CommandKind = "invalidate_stale"
	CommandCloseScope      CommandKind = "close_scope"
)

type Command interface {
	workingSetCommand()
	commandID() CommandID
	expectedVersion() uint64
	actor() taskstate.Authority
	kind() CommandKind
	decide(*Set) error
}

type AcquireCommand struct {
	CommandID       CommandID           `json:"command_id"`
	ExpectedVersion uint64              `json:"expected_version"`
	Actor           taskstate.Authority `json:"actor"`
	Request         AcquireRequest      `json:"request"`
}

type RetainCommand struct {
	CommandID       CommandID           `json:"command_id"`
	ExpectedVersion uint64              `json:"expected_version"`
	Actor           taskstate.Authority `json:"actor"`
	ItemID          ItemID              `json:"item_id"`
	Scope           Scope               `json:"scope"`
	Retention       Retention           `json:"retention"`
}

type ReleaseCommand struct {
	CommandID       CommandID           `json:"command_id"`
	ExpectedVersion uint64              `json:"expected_version"`
	Actor           taskstate.Authority `json:"actor"`
	ItemID          ItemID              `json:"item_id"`
	Scope           Scope               `json:"scope"`
	Reason          string              `json:"reason"`
}

type TouchCommand struct {
	CommandID       CommandID           `json:"command_id"`
	ExpectedVersion uint64              `json:"expected_version"`
	Actor           taskstate.Authority `json:"actor"`
	ItemIDs         []ItemID            `json:"item_ids"`
}

type InvalidateStaleCommand struct {
	CommandID       CommandID           `json:"command_id"`
	ExpectedVersion uint64              `json:"expected_version"`
	Actor           taskstate.Authority `json:"actor"`
	ItemID          ItemID              `json:"item_id"`
	CurrentVersion  string              `json:"current_version"`
	CurrentHash     string              `json:"current_hash"`
	Reason          string              `json:"reason"`
}

type CloseScopeCommand struct {
	CommandID       CommandID           `json:"command_id"`
	ExpectedVersion uint64              `json:"expected_version"`
	Actor           taskstate.Authority `json:"actor"`
	Scope           Scope               `json:"scope"`
	Reason          string              `json:"reason"`
}

func (AcquireCommand) workingSetCommand()         {}
func (RetainCommand) workingSetCommand()          {}
func (ReleaseCommand) workingSetCommand()         {}
func (TouchCommand) workingSetCommand()           {}
func (InvalidateStaleCommand) workingSetCommand() {}
func (CloseScopeCommand) workingSetCommand()      {}

func (c AcquireCommand) commandID() CommandID         { return c.CommandID }
func (c RetainCommand) commandID() CommandID          { return c.CommandID }
func (c ReleaseCommand) commandID() CommandID         { return c.CommandID }
func (c TouchCommand) commandID() CommandID           { return c.CommandID }
func (c InvalidateStaleCommand) commandID() CommandID { return c.CommandID }
func (c CloseScopeCommand) commandID() CommandID      { return c.CommandID }

func (c AcquireCommand) expectedVersion() uint64         { return c.ExpectedVersion }
func (c RetainCommand) expectedVersion() uint64          { return c.ExpectedVersion }
func (c ReleaseCommand) expectedVersion() uint64         { return c.ExpectedVersion }
func (c TouchCommand) expectedVersion() uint64           { return c.ExpectedVersion }
func (c InvalidateStaleCommand) expectedVersion() uint64 { return c.ExpectedVersion }
func (c CloseScopeCommand) expectedVersion() uint64      { return c.ExpectedVersion }

func (c AcquireCommand) actor() taskstate.Authority         { return c.Actor }
func (c RetainCommand) actor() taskstate.Authority          { return c.Actor }
func (c ReleaseCommand) actor() taskstate.Authority         { return c.Actor }
func (c TouchCommand) actor() taskstate.Authority           { return c.Actor }
func (c InvalidateStaleCommand) actor() taskstate.Authority { return c.Actor }
func (c CloseScopeCommand) actor() taskstate.Authority      { return c.Actor }

func (AcquireCommand) kind() CommandKind         { return CommandAcquire }
func (RetainCommand) kind() CommandKind          { return CommandRetain }
func (ReleaseCommand) kind() CommandKind         { return CommandRelease }
func (TouchCommand) kind() CommandKind           { return CommandTouch }
func (InvalidateStaleCommand) kind() CommandKind { return CommandInvalidateStale }
func (CloseScopeCommand) kind() CommandKind      { return CommandCloseScope }

func (c AcquireCommand) decide(set *Set) error {
	_, err := set.Acquire(c.Request)
	return err
}

func (c RetainCommand) decide(set *Set) error {
	_, err := set.Retain(c.ItemID, c.Scope, c.Retention)
	return err
}

func (c ReleaseCommand) decide(set *Set) error {
	_, err := set.Release(c.ItemID, c.Scope, c.Reason)
	return err
}

func (c TouchCommand) decide(set *Set) error {
	_, err := set.TouchMany(c.ItemIDs)
	return err
}

func (c InvalidateStaleCommand) decide(set *Set) error {
	_, changed, err := set.InvalidateStale(c.ItemID, c.CurrentVersion, c.CurrentHash, c.Reason)
	if err == nil && !changed {
		return ErrNoStateChange
	}
	return err
}

func (c CloseScopeCommand) decide(set *Set) error {
	_, err := set.CloseScope(c.Scope, c.Reason)
	return err
}
