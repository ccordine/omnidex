package taskstate

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

const commandIdentitySchemaV1 = "omnidex.task-state-command.v1"

var (
	ledgerIDPattern  = regexp.MustCompile(`^ledger_[0-9a-f]{64}$`)
	commandIDPattern = regexp.MustCompile(`^command_[0-9a-f]{64}$`)
	uuidPattern      = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	hexDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	refURIPattern    = regexp.MustCompile(`^[a-z][a-z0-9+.-]*:[^[:space:]]+$`)
)

func NewLedgerID(owner LedgerOwner) (LedgerID, error) {
	if err := validateLedgerOwner(owner); err != nil {
		return "", err
	}
	digest := identityDigest(LedgerSchemaV1, strconv.FormatInt(owner.JobID, 10), owner.RunID)
	return LedgerID("ledger_" + digest), nil
}

func NewCommandID(parts ...string) (CommandID, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("%w: command identity requires at least one part", ErrInvalidCommand)
	}
	for index, part := range parts {
		if strings.TrimSpace(part) == "" || part != strings.TrimSpace(part) {
			return "", fmt.Errorf("%w: command identity part %d must be one nonempty exact value", ErrInvalidCommand, index)
		}
	}
	return CommandID("command_" + identityDigest(append([]string{commandIdentitySchemaV1}, parts...)...)), nil
}

type CommandDescriptor struct {
	ID              CommandID
	ExpectedVersion uint64
	Actor           Authority
	Kind            CommandKind
	SHA256          string
}

func DescribeCommand(command Command) (CommandDescriptor, error) {
	if isNilCommand(command) {
		return CommandDescriptor{}, fmt.Errorf("%w: command is required", ErrInvalidCommand)
	}
	if !commandIDPattern.MatchString(string(command.commandID())) {
		return CommandDescriptor{}, fmt.Errorf("%w: command ID must match command_ plus 64 lowercase hex characters", ErrInvalidCommand)
	}
	if err := validateAuthority(command.actor()); err != nil {
		return CommandDescriptor{}, err
	}
	raw, err := json.Marshal(command)
	if err != nil {
		return CommandDescriptor{}, fmt.Errorf("%w: encode %s command: %v", ErrInvalidCommand, command.kind(), err)
	}
	sha := identityDigest(commandIdentitySchemaV1, string(command.kind()), string(raw))
	return CommandDescriptor{
		ID: command.commandID(), ExpectedVersion: command.expectedVersion(),
		Actor: command.actor(), Kind: command.kind(), SHA256: sha,
	}, nil
}

func identityDigest(parts ...string) string {
	var canonical bytes.Buffer
	for _, part := range parts {
		_ = binary.Write(&canonical, binary.BigEndian, uint64(len(part)))
		_, _ = canonical.WriteString(part)
	}
	sum := sha256.Sum256(canonical.Bytes())
	return hex.EncodeToString(sum[:])
}

func contentDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func validateLedgerID(id LedgerID, owner LedgerOwner) error {
	if !ledgerIDPattern.MatchString(string(id)) {
		return fmt.Errorf("%w: ledger ID must match ledger_ plus 64 lowercase hex characters", ErrInvalidState)
	}
	expected, err := NewLedgerID(owner)
	if err != nil {
		return err
	}
	if id != expected {
		return fmt.Errorf("%w: ledger ID does not match its job and run owner", ErrInvalidState)
	}
	return nil
}

func validDigest(value string) bool { return hexDigestPattern.MatchString(value) }

func isNilCommand(command Command) bool {
	if command == nil {
		return true
	}
	value := reflect.ValueOf(command)
	return value.Kind() == reflect.Pointer && value.IsNil()
}
