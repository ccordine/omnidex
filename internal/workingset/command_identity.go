package workingset

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"

	"github.com/gryph/omnidex/internal/taskstate"
)

const WorkingSetCommandSchemaV1 = "omnidex.working-set-command.v1"

var commandIDPattern = regexp.MustCompile(`^working_command_[0-9a-f]{64}$`)

type CommandDescriptor struct {
	ID              CommandID
	ExpectedVersion uint64
	Actor           taskstate.Authority
	Kind            CommandKind
	SHA256          string
	Raw             json.RawMessage
}

func NewCommandID(parts ...string) (CommandID, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("%w: command identity requires at least one part", ErrInvalidCommand)
	}
	for index, part := range parts {
		if part == "" || strings.TrimSpace(part) != part {
			return "", fmt.Errorf("%w: command identity part %d must be one nonempty exact value", ErrInvalidCommand, index)
		}
	}
	return CommandID("working_command_" + workingSetIdentityDigest(
		append([]string{WorkingSetCommandSchemaV1}, parts...)...,
	)), nil
}

func DescribeCommand(command Command) (CommandDescriptor, error) {
	if command == nil || (reflect.ValueOf(command).Kind() == reflect.Pointer && reflect.ValueOf(command).IsNil()) {
		return CommandDescriptor{}, fmt.Errorf("%w: command is required", ErrInvalidCommand)
	}
	if !commandIDPattern.MatchString(string(command.commandID())) {
		return CommandDescriptor{}, fmt.Errorf(
			"%w: command ID must match working_command_ plus 64 lowercase hex characters", ErrInvalidCommand,
		)
	}
	if command.actor() != taskstate.AuthorityCode {
		return CommandDescriptor{}, fmt.Errorf("%w: working-set mutation requires code authority", ErrInvalidCommand)
	}
	if command.expectedVersion() > uint64(math.MaxInt64) {
		return CommandDescriptor{}, fmt.Errorf("%w: expected version exceeds PostgreSQL BIGINT", ErrInvalidCommand)
	}
	raw, err := json.Marshal(command)
	if err != nil {
		return CommandDescriptor{}, fmt.Errorf("%w: encode %s command: %v", ErrInvalidCommand, command.kind(), err)
	}
	return CommandDescriptor{
		ID: command.commandID(), ExpectedVersion: command.expectedVersion(), Actor: command.actor(),
		Kind: command.kind(), SHA256: workingSetIdentityDigest(
			WorkingSetCommandSchemaV1, string(command.kind()), string(raw),
		), Raw: append(json.RawMessage(nil), raw...),
	}, nil
}
