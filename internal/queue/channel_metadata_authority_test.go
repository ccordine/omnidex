package queue

import (
	"encoding/json"
	"testing"
)

func TestGenericJobMetadataRejectsChannelBindingAuthority(t *testing.T) {
	t.Parallel()
	for _, metadata := range []map[string]any{
		{"channel_id": "channel-one"},
		{"channel_user_message_id": float64(1)},
		{"channel_id": "channel-one", "channel_user_message_id": float64(1)},
	} {
		if err := ValidateJobMetadataAuthority(metadata); err == nil {
			t.Fatalf("generic job accepted channel binding: %+v", metadata)
		}
		raw, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := stepsForJob("chat", "exact instruction", raw); err == nil {
			t.Fatalf("step resolver accepted channel binding: %s", raw)
		}
	}
}
