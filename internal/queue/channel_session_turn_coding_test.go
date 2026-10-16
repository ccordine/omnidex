package queue

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestChannelSessionTurnModePreservesConversationOperationIdentity(t *testing.T) {
	id, err := model.NewLifecycleOperationID()
	if err != nil {
		t.Fatal(err)
	}
	command := ChannelSessionTurnCommand{
		OperationID: id, ChannelID: "channel_session_fixture", WorkspaceRoot: "/workspace",
		WorkspaceIdentity: "client_11111111111111111111111111111111_directory_1_101", Text: "exact",
	}
	historical, err := json.Marshal(struct {
		OperationID       model.LifecycleOperationID `json:"operation_id"`
		ChannelID         model.ChannelID            `json:"channel_id"`
		WorkspaceRoot     string                     `json:"workspace_root"`
		WorkspaceIdentity string                     `json:"workspace_identity"`
		Text              string                     `json:"text"`
	}{command.OperationID, command.ChannelID, command.WorkspaceRoot, command.WorkspaceIdentity, command.Text})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []codingagent.Mode{"", codingagent.Agents} {
		command.CodingMode = mode
		normalized, err := normalizeChannelSessionTurnCommand(command)
		if err != nil {
			t.Fatal(err)
		}
		descriptor, err := describeLifecycleOperation(id, LifecycleChannelSession, normalized)
		if err != nil {
			t.Fatal(err)
		}
		if mode == codingagent.Agents {
			if normalized.CodingMode != mode || !bytes.Contains(descriptor.Payload, []byte(`"coding_mode":"agents"`)) {
				t.Fatalf("Agents mode is not exact operation authority: %s", descriptor.Payload)
			}
		} else if normalized.CodingMode != "" || !bytes.Equal(descriptor.Payload, historical) {
			t.Fatalf("conversation changed historical operation identity: %s", descriptor.Payload)
		}
	}
	for _, mode := range []codingagent.Mode{"normal", "agent", "Agents", " agents ", "null"} {
		command.CodingMode = mode
		_, err := normalizeChannelSessionTurnCommand(command)
		if !errors.Is(err, ErrChannelSessionTurnInvalid) || !strings.Contains(err.Error(), "unknown coding mode") {
			t.Fatalf("mode %q did not fail at the typed boundary: %v", mode, err)
		}
	}
}
