package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func objectiveAssistantMetadata() json.RawMessage {
	return json.RawMessage(`{"channel_id":"test-chat","channel_mode":"assistant"}`)
}

func objectiveAssistantDataSourceMetadata() json.RawMessage {
	return json.RawMessage(`{
		"channel_id":"test-chat",
		"channel_mode":"assistant",
		"data_source_id":"source-1"
	}`)
}

func TestTurnAuthorityPreservesOpaqueDelegatedDatabaseIdentity(t *testing.T) {
	authorityID := "dba_" + strings.Repeat("a", 64)
	metadata, err := json.Marshal(map[string]any{
		"channel_id": "test-chat", "channel_mode": "assistant",
		"data_source_id": "source-1", "delegated_data_authority_id": authorityID,
	})
	if err != nil {
		t.Fatal(err)
	}
	authority, err := newTurnAuthority(model.Job{
		ID: 1, Pipeline: model.PipelineChat, Instruction: "Find the knee collection.", Metadata: metadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if authority.DataSourceID != "source-1" || authority.DelegatedDataAuthorityID != authorityID {
		t.Fatalf("turn authority=%+v", authority)
	}
	firstID := objectiveTurnID(authority, assemblyline.ObjectiveKindDatabaseRead)
	authority.DelegatedDataAuthorityID = "dba_" + strings.Repeat("b", 64)
	if secondID := objectiveTurnID(authority, assemblyline.ObjectiveKindDatabaseRead); secondID == firstID {
		t.Fatal("objective identity ignored delegated database authority")
	}
}

func TestTurnAuthorityRejectsMalformedOrUnboundDelegatedIdentity(t *testing.T) {
	for _, metadata := range []json.RawMessage{
		json.RawMessage(`{"channel_id":"test-chat","channel_mode":"assistant","delegated_data_authority_id":"dba_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`),
		json.RawMessage(`{"channel_id":"test-chat","channel_mode":"assistant","data_source_id":"source-1","delegated_data_authority_id":"not-an-authority"}`),
	} {
		if _, err := newTurnAuthority(model.Job{
			ID: 1, Pipeline: model.PipelineChat, Instruction: "Exact question.", Metadata: metadata,
		}); err == nil {
			t.Fatalf("invalid delegated metadata was accepted: %s", metadata)
		}
	}
}
