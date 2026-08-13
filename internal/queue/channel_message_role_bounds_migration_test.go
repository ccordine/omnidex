package queue

import (
	"os"
	"strings"
	"testing"
)

func TestChannelMessageRoleBoundsMigrationIsTypedAndFailClosed(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("../../migrations/072_channel_message_role_bounds.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"LOCK TABLE ai_channel_messages IN ACCESS EXCLUSIVE MODE",
		"cannot install channel message role bounds: message",
		"role = 'user' AND octet_length(content) BETWEEN 1 AND 4096",
		"role = 'assistant' AND octet_length(content) BETWEEN 1 AND 32768",
		"DROP CONSTRAINT ai_channel_messages_content_check",
		"channel message role bounds postcondition failed",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{"IF EXISTS", "CASCADE", "ON CONFLICT", "DELETE FROM", "UPDATE ai_channel_messages"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("migration contains forbidden %q", forbidden)
		}
	}
}
