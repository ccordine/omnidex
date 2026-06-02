package scrumcardllm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunCardTicketUsesChatPath(t *testing.T) {
	client := &fakeTicketLLM{chatReply: "  drafted ticket  "}
	got, err := RunCardTicket(context.Background(), client, "", "system", "user")
	if err != nil {
		t.Fatalf("RunCardTicket: %v", err)
	}
	if got != "drafted ticket" {
		t.Fatalf("ticket = %q", got)
	}
}

func TestRunCardTicketRespectsDeadline(t *testing.T) {
	t.Setenv("OMNI_TICKET_CONTEXT_DEADLINE", "30ms")
	client := &fakeTicketLLM{
		chatFn: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return nil
			}
		},
	}
	_, err := RunCardTicket(context.Background(), client, "llama3.2", "system", "user")
	if err == nil {
		t.Fatal("expected deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(strings.ToLower(err.Error()), "deadline") {
		t.Fatalf("unexpected error: %v", err)
	}
}

type fakeTicketLLM struct {
	chatReply string
	chatFn    func(context.Context) error
}

func (f *fakeTicketLLM) Chat(ctx context.Context, model, system, user string) (string, error) {
	if f.chatFn != nil {
		if err := f.chatFn(ctx); err != nil {
			return "", err
		}
	}
	return f.chatReply, nil
}

func (f *fakeTicketLLM) Generate(context.Context, string, string) (string, error) {
	return "", errors.New("generate should not be called")
}
