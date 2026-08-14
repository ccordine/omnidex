package queue

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

type rejectedScrumEntropy struct{}

func (rejectedScrumEntropy) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }

func TestScrumMessageIdentityIsExactAndEntropyFailureIsLoud(t *testing.T) {
	t.Parallel()
	first, err := NewScrumMessageID(bytes.NewReader(bytes.Repeat([]byte{0x11}, scrumMessageIdentityBytes)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewScrumMessageID(bytes.NewReader(bytes.Repeat([]byte{0x22}, scrumMessageIdentityBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first != "chatmsg_"+strings.Repeat("11", scrumMessageIdentityBytes) ||
		second != "chatmsg_"+strings.Repeat("22", scrumMessageIdentityBytes) {
		t.Fatalf("identities=%q/%q", first, second)
	}
	if _, err := NewScrumMessageID(rejectedScrumEntropy{}); err == nil || !strings.Contains(err.Error(), "entropy unavailable") {
		t.Fatalf("entropy failure=%v", err)
	}
	if _, err := NewScrumMessageID(nil); err == nil {
		t.Fatal("nil entropy reader unexpectedly produced an identity")
	}
}

func TestScrumMessageIdentityHasNoClockOrHashFallback(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"scrum_message_identity.go", "scrum_play_authority.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"time.Now", "UnixNano", "crypto/sha1", "crypto/sha256"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains clock/hash identity fallback %q", path, forbidden)
			}
		}
	}
}

func TestPostgresScrumMessageIdentityCollisionRollsBack(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Message collision", "/tmp/scrum-message-collision", "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(ctx, project.ID, "collision-card", "Collision", "", "backlog", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := NewScrumMessageID(bytes.NewReader(bytes.Repeat([]byte{0x33}, scrumMessageIdentityBytes)))
	if err != nil {
		t.Fatal(err)
	}
	appendOne := func(content string) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := lockScrumCardTx(ctx, tx, project.ID, card.ID); err != nil {
			return err
		}
		if _, err := insertScrumCardMessageTx(ctx, tx, project.ID, card.ID, ScrumCardMessageAppend{
			ID: id, Role: "system", Content: content,
		}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err := appendOne("first exact content"); err != nil {
		t.Fatal(err)
	}
	if err := appendOne("divergent collision content"); err == nil {
		t.Fatal("duplicate cryptographic message identity committed")
	}
	var count int
	var content string
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*),MIN(content) FROM scrum_card_messages
		WHERE project_id=$1 AND card_id=$2 AND message_id=$3
	`, project.ID, card.ID, id).Scan(&count, &content); err != nil {
		t.Fatal(err)
	}
	if count != 1 || content != "first exact content" {
		t.Fatalf("collision post-state count=%d content=%q", count, content)
	}
}
