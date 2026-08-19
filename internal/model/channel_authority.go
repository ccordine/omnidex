package model

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	MaxChannelIDBytes   = 96
	MaxChannelNameBytes = 256
	MaxChannelTags      = 32
	MaxChannelTagBytes  = 64
	// MaxFreeFormTurnBytes is the accepted user-turn boundary. It fits the
	// first semantic station without trimming or rewriting.
	MaxFreeFormTurnBytes         = 4 * 1024
	MaxChannelContentBytes       = 32 * 1024
	MaxChannelWorkspaceRootBytes = 4096
	MaxDataSourceIDBytes         = 128
	MaxRoleplayCharacterIDBytes  = 36
)

type ChannelScope string
type ChannelMode string

const (
	ChannelScopeUser     ChannelScope = "user"
	ChannelModeAssistant ChannelMode  = "assistant"
	ChannelModeRoleplay  ChannelMode  = "roleplay"
)

func (id ChannelID) Validate() error {
	value := string(id)
	if len(value) == 0 || len(value) > MaxChannelIDBytes || !utf8.ValidString(value) {
		return fmt.Errorf("channel id must contain 1..%d valid UTF-8 bytes", MaxChannelIDBytes)
	}
	for index, character := range []byte(value) {
		allowed := character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' ||
			character == '_' || character == '.' || character == ':' || character == '-'
		if !allowed || index == 0 && !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9') {
			return fmt.Errorf("channel id %q is not canonical", value)
		}
	}
	return nil
}

func (id DataSourceID) Validate() error {
	value := string(id)
	if len(value) == 0 || len(value) > MaxDataSourceIDBytes || !utf8.ValidString(value) {
		return fmt.Errorf("data source id must contain 1..%d valid UTF-8 bytes", MaxDataSourceIDBytes)
	}
	for index, character := range []byte(value) {
		allowed := character >= 'a' && character <= 'z' ||
			character >= '0' && character <= '9' ||
			character == '_' || character == '.' || character == ':' || character == '-'
		if !allowed || index == 0 && !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9') {
			return fmt.Errorf("data source id %q is not canonical", value)
		}
	}
	return nil
}

func (id RoleplayCharacterID) Validate() error {
	value := string(id)
	if len(value) != MaxRoleplayCharacterIDBytes || !strings.HasPrefix(value, "rpc_") {
		return fmt.Errorf("roleplay character id must be one code-issued opaque identity")
	}
	for _, character := range []byte(value[4:]) {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return fmt.Errorf("roleplay character id must be one code-issued opaque identity")
		}
	}
	return nil
}

func (scope ChannelScope) Validate() error {
	if scope != ChannelScopeUser {
		return fmt.Errorf("channel scope %q is unsupported", scope)
	}
	return nil
}

func (mode ChannelMode) Validate() error {
	if mode != ChannelModeAssistant && mode != ChannelModeRoleplay {
		return fmt.Errorf("channel mode %q is unsupported", mode)
	}
	return nil
}

func (role ChannelMessageRole) Validate() error {
	if role != ChannelMessageRoleUser && role != ChannelMessageRoleAssistant {
		return fmt.Errorf("channel message role %q is unsupported", role)
	}
	return nil
}

func (channel Channel) ValidateForCreate() error {
	if channel.ProjectID != 0 {
		return fmt.Errorf("channel project identity is server-resolved and must be omitted on create")
	}
	return channel.validateAuthority()
}

func (channel Channel) validateAuthority() error {
	if err := channel.ID.Validate(); err != nil {
		return err
	}
	if err := channel.Scope.Validate(); err != nil {
		return err
	}
	if err := channel.Mode.Validate(); err != nil {
		return err
	}
	if err := validateExactChannelText(channel.Name, "channel name", MaxChannelNameBytes); err != nil {
		return err
	}
	if len(channel.Tags) > MaxChannelTags {
		return fmt.Errorf("channel tags exceed the %d-tag bound", MaxChannelTags)
	}
	seen := make(map[string]struct{}, len(channel.Tags))
	for _, tag := range channel.Tags {
		if err := validateExactChannelText(tag, "channel tag", MaxChannelTagBytes); err != nil {
			return err
		}
		if tag != strings.ToLower(tag) {
			return fmt.Errorf("channel tag %q must be lowercase", tag)
		}
		if _, exists := seen[tag]; exists {
			return fmt.Errorf("channel tag %q is duplicated", tag)
		}
		seen[tag] = struct{}{}
	}
	if channel.DataSourceID != "" {
		if err := channel.DataSourceID.Validate(); err != nil {
			return err
		}
	}
	switch channel.Mode {
	case ChannelModeAssistant:
		if channel.RoleplayViewpointCharacterID != "" {
			return fmt.Errorf("assistant channel cannot carry fictional viewpoint authority")
		}
	case ChannelModeRoleplay:
		if channel.DataSourceID != "" {
			return fmt.Errorf("roleplay channel cannot bind a real-world data source")
		}
		if err := channel.RoleplayViewpointCharacterID.Validate(); err != nil {
			return err
		}
	}
	return ValidateChannelWorkspaceRoot(channel.WorkspaceRoot)
}

func (channel Channel) ValidateStored() error {
	if err := channel.validateAuthority(); err != nil {
		return err
	}
	if channel.ProjectID < 1 {
		return fmt.Errorf("stored channel requires a positive project identity")
	}
	return nil
}

func ValidateChannelWorkspaceRoot(root string) error {
	if err := validateExactChannelText(root, "channel workspace root", MaxChannelWorkspaceRootBytes); err != nil {
		return err
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("channel workspace root must be absolute")
	}
	if filepath.Clean(root) != root {
		return fmt.Errorf("channel workspace root must be canonical")
	}
	return nil
}

func ValidateChannelMessageContent(content string) error {
	if len(content) == 0 || len(content) > MaxChannelContentBytes || !utf8.ValidString(content) || strings.IndexByte(content, 0) >= 0 {
		return fmt.Errorf("channel message content must contain 1..%d valid UTF-8 bytes without NUL", MaxChannelContentBytes)
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("channel message content is blank")
	}
	return nil
}

func ValidateChannelMessage(role ChannelMessageRole, content string) error {
	if err := role.Validate(); err != nil {
		return err
	}
	if err := ValidateChannelMessageContent(content); err != nil {
		return err
	}
	if role == ChannelMessageRoleUser && len(content) > MaxFreeFormTurnBytes {
		return fmt.Errorf("channel user message exceeds the %d-byte free-form turn bound", MaxFreeFormTurnBytes)
	}
	return nil
}

func validateExactChannelText(value, name string, maxBytes int) error {
	if len(value) == 0 || len(value) > maxBytes || !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("%s must contain 1..%d valid UTF-8 bytes without NUL", name, maxBytes)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must be exact nonblank text without surrounding whitespace", name)
	}
	return nil
}
