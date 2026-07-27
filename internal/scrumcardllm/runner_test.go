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
	got, err := RunCardTicket(context.Background(), client, "ticket-model", "system", "user")
	if err != nil {
		t.Fatalf("RunCardTicket: %v", err)
	}
	if got != "drafted ticket" {
		t.Fatalf("ticket = %q", got)
	}
}

func TestRunCardTicketRejectsMissingModel(t *testing.T) {
	client := &fakeTicketLLM{chatReply: "drafted ticket"}
	if _, err := RunCardTicket(context.Background(), client, "", "system", "user"); err == nil {
		t.Fatal("expected missing model to fail")
	}
}

func TestParseTagsSuggestResponseRequiresStrictJSON(t *testing.T) {
	for _, raw := range []string{
		`Here you go: {"tags":["go"],"notes":"ok"}`,
		`{"tags":["go"],"notes":"ok","extra":true}`,
		`{"tags":["go","go"],"notes":"duplicate"}`,
		`{"tags":[],"notes":""}`,
	} {
		if _, err := ParseTagsSuggestResponse(raw); err == nil {
			t.Fatalf("expected invalid response to fail: %s", raw)
		}
	}
}

func TestParseTagsSuggestResponseNormalizesValidatedTags(t *testing.T) {
	result, err := ParseTagsSuggestResponse(`{"tags":[" Go ","realtime"],"notes":"Useful tags"}`)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(result.Suggested) != 2 || result.Suggested[0] != "go" {
		t.Fatalf("unexpected tags: %#v", result.Suggested)
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

func TestRunCardTicketRejectsInvalidDeadlineConfiguration(t *testing.T) {
	t.Setenv("OMNI_TICKET_CONTEXT_DEADLINE", "eventually")
	client := &fakeTicketLLM{chatReply: "drafted ticket"}
	if _, err := RunCardTicket(context.Background(), client, "ticket-model", "system", "user"); err == nil {
		t.Fatal("expected invalid deadline configuration to fail")
	}
}

func TestRunCardTicketRequiresChatCapableClient(t *testing.T) {
	client := generateOnlyTicketLLM{}
	if _, err := RunCardTicket(context.Background(), client, "ticket-model", "system", "user"); err == nil {
		t.Fatal("expected non-chat client to fail")
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

type generateOnlyTicketLLM struct{}

func (generateOnlyTicketLLM) Generate(context.Context, string, string) (string, error) {
	return "generated", nil
}
