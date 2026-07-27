package scrumcardllm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

const defaultTicketContextDeadline = 10 * time.Second

// TicketContextDeadline returns the max time allowed for card ticket LLM calls.
// Override with OMNI_TICKET_CONTEXT_DEADLINE (Go duration syntax, e.g. "10s", "30s").
func TicketContextDeadline() (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv("OMNI_TICKET_CONTEXT_DEADLINE"))
	if v == "" {
		return defaultTicketContextDeadline, nil
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("OMNI_TICKET_CONTEXT_DEADLINE must be a Go duration: %w", err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("OMNI_TICKET_CONTEXT_DEADLINE must be positive, received %q", v)
	}
	return parsed, nil
}

// TicketLLMContext preserves worker cancellation and applies the generation deadline.
func TicketLLMContext(parent context.Context) (context.Context, context.CancelFunc, error) {
	if parent == nil {
		return nil, nil, fmt.Errorf("ticket LLM parent context is required")
	}
	deadline, err := TicketContextDeadline()
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(parent, deadline)
	return ctx, cancel, nil
}
