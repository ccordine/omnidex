package scrumcardllm

import (
	"context"
	"os"
	"time"
)

const defaultTicketContextDeadline = 10 * time.Second

// TicketContextDeadline returns the max time allowed for card ticket LLM calls.
// Override with OMNI_TICKET_CONTEXT_DEADLINE (Go duration syntax, e.g. "10s", "30s").
func TicketContextDeadline() time.Duration {
	if v := os.Getenv("OMNI_TICKET_CONTEXT_DEADLINE"); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil && parsed > 0 {
			return parsed
		}
	}
	return defaultTicketContextDeadline
}

// TicketLLMContext detaches from parent cancellation (proxy disconnect) and applies
// the ticket generation deadline.
func TicketLLMContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), TicketContextDeadline())
}
