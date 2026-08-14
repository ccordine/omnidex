package api

import (
	"crypto/rand"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSyncRunningJobChannelChatUsesOnlyTypedStepContexts(t *testing.T) {
	job := model.JobDetails{
		Job: model.Job{ID: 3, Status: model.JobStatusRunning},
		Contexts: []model.StepContext{
			{ID: 1, Key: "event", Value: "event=structured_patch_apply_started applying"},
			{ID: 2, Key: "event", Value: "event=structured_patch_apply_finished applied"},
		},
	}
	card := scrumSyncTestCard(job.Job.ID, ScrumCard{})

	updated, ok, err := syncRunningJobChannelChat(card, job)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected first sync")
	}
	if len(updated.PendingChannelMessages) != 2 || updated.StepContextCursor != 2 {
		t.Fatalf("typed channel projection=%+v cursor=%d", updated.PendingChannelMessages, updated.StepContextCursor)
	}

	updated2, ok, err := syncRunningJobChannelChat(updated, job)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no duplicate sync")
	}
	if len(updated2.PendingChannelMessages) != 2 {
		t.Fatalf("typed step contexts were duplicated: %+v", updated2.PendingChannelMessages)
	}
}

func TestScrumMessageIDsAreCryptographicUniqueAndNeverClockDerived(t *testing.T) {
	t.Parallel()
	const count = 512
	ids := make(chan string, count)
	errors := make(chan error, count)
	var group sync.WaitGroup
	for index := 0; index < count; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			id, err := queue.NewScrumMessageID(rand.Reader)
			if err != nil {
				errors <- err
				return
			}
			ids <- id
		}()
	}
	group.Wait()
	close(ids)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	seen := make(map[string]struct{}, count)
	for id := range ids {
		if len(id) != len("chatmsg_")+32 || !strings.HasPrefix(id, "chatmsg_") {
			t.Fatalf("cryptographic message ID=%q", id)
		}
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("duplicate cryptographic message ID=%q", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("unique IDs=%d want=%d", len(seen), count)
	}
	raw, err := os.ReadFile("scrum_channel_chat.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"crypto/sha1", "UnixNano", "newScrumChatMessageID"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("Scrum channel message identity retains clock/hash derivation %q", forbidden)
		}
	}
}

func TestScrumMessageAppendRejectsMissingServerIdentity(t *testing.T) {
	t.Parallel()
	if _, err := scrumChannelMessageAppends([]ScrumChatMessage{{
		Role: "assistant", Content: "same content",
	}}); err == nil || !strings.Contains(err.Error(), "server-owned identity") {
		t.Fatalf("missing server identity error=%v", err)
	}
}

func TestAppendScrumChannelEventStagesDurableMessageAppend(t *testing.T) {
	card, err := appendScrumChannelEvent(ScrumCard{}, "system", "Queued for play")
	if err != nil {
		t.Fatal(err)
	}
	if len(card.PendingChannelMessages) != 1 || card.PendingChannelMessages[0].Role != "system" ||
		card.PendingChannelMessages[0].Content != "Queued for play" {
		t.Fatalf("pending messages=%v", card.PendingChannelMessages)
	}
	if len(card.Chat) != 0 {
		t.Fatalf("event bypassed durable message append: %+v", card.Chat)
	}
}
