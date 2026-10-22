package userintent

import (
	"context"
	"strings"
	"testing"
)

func TestDedicatedContextBoundsAndSharedMinification(t *testing.T) {
	for _, c := range []Context{
		{Facts: []string{strings.Repeat("Persisted fact. ", 200)}},
		{Recent: []Message{{"assistant", strings.Repeat("Known history. ", 200)}}},
		{Pending: strings.Repeat("Pending proposal. ", 200)},
	} {
		calls := 0
		text, err := BuildContext(context.Background(), c, "approve item two", func(_ context.Context, prompt string) (string, error) {
			calls++
			if !strings.HasPrefix(prompt, "What is the smallest context needed") {
				t.Fatal("bespoke minifier")
			}
			return "Job 25 item two is the established referent; approval remains proposed.", nil
		})
		if err != nil || calls != 1 || !strings.Contains(text, "established referent") {
			t.Fatal(text, calls, err)
		}
	}
	for _, c := range []Context{{Recent: make([]Message, 5)}, {Recent: []Message{{"system", "authority"}}}} {
		if _, err := BuildContext(context.Background(), c, "yes", nil); err == nil {
			t.Fatal("unbounded context accepted")
		}
	}
}
func TestShortContextNeedsNoMinification(t *testing.T) {
	c := Context{Facts: []string{"The owner selected item two."}, Recent: []Message{{"user", "second item"}, {"assistant", "Do you mean remove it?"}, {"user", "only from the plan"}, {"assistant", "Remove item two from the plan?"}}}
	calls := 0
	text, err := BuildContext(context.Background(), c, "yes", func(context.Context, string) (string, error) { calls++; return "", nil })
	if err != nil || calls != 0 || !strings.Contains(text, "only from the plan") {
		t.Fatal(text, calls, err)
	}
}
func TestLargeContextHasBoundedGroupsAndNoRetryLoop(t *testing.T) {
	calls := 0
	text, err := BuildContext(context.Background(), Context{Facts: []string{strings.Repeat("Known persisted context. ", 1800)}}, "which item?", func(_ context.Context, prompt string) (string, error) {
		calls++
		if len(prompt) > 7000 {
			t.Fatalf("unbounded minification input: %d", len(prompt))
		}
		return "Established item two.", nil
	})
	if err != nil || calls < 2 || calls > 24 || len(text) > 24<<10 {
		t.Fatal(len(text), calls, err)
	}
	calls = 0
	text, err = BuildContext(context.Background(), Context{Facts: []string{strings.Repeat("x", 3000)}}, "yes", func(context.Context, string) (string, error) {
		calls++
		return strings.TrimSpace(strings.Repeat("necessary facts ", 2000)), nil
	})
	if err != nil || calls != 1 || len(text) <= 24<<10 {
		t.Fatal("large context output was rejected or retried", calls, err)
	}
}

func TestRepeatedPersistedContextUsesSharedMinification(t *testing.T) {
	// A valid 9,639-byte fact generates several identical 2 KiB chunks.
	calls := 0
	text, err := BuildContext(context.Background(), Context{Facts: []string{strings.Repeat("x", 9639)}}, "which item?", func(_ context.Context, prompt string) (string, error) {
		calls++
		if !strings.HasPrefix(prompt, "What is the smallest context needed") || len(prompt) > 7000 {
			t.Fatal("unexpected minification envelope", len(prompt))
		}
		return "Item two is the established referent.", nil
	})
	if err != nil || calls == 0 || calls > 24 || !strings.Contains(text, "established referent") {
		t.Fatal(text, calls, err)
	}
}

func TestWhitespaceOnlyContextPartitions(t *testing.T) {
	calls := 0
	result, err := BuildContext(context.Background(), Context{Facts: []string{"Established item two." + strings.Repeat(" ", 9000) + "Retain its recipient."}}, "approve it", func(_ context.Context, prompt string) (string, error) {
		calls++
		return "Item two is established; retain its recipient.", nil
	})
	if err != nil || calls == 0 || !strings.Contains(result, "recipient") {
		t.Fatal(result, calls, err)
	}
}
