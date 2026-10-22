package sessionintent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/userintent"
)

type Mode string

const (
	Infer    Mode = "infer"
	Interact Mode = "interact"
	Ledger   Mode = "ledger"
	Research Mode = "research"
)

func (m Mode) Validate() error {
	switch m {
	case "", Infer, Interact, Ledger, Research:
		return nil
	default:
		return fmt.Errorf("unknown conversation mode %q", m)
	}
}

type Recipient string

const (
	Planner              Recipient = "planner"
	Codex                Recipient = "codex"
	Approval             Recipient = "approval"
	Native               Recipient = "native"
	ResearchConversation Recipient = "research"
)

func InteractionRecipient(c Context) Recipient {
	if c.Pending != nil {
		return Approval
	}
	if c.InputID != nil {
		return Native
	}
	if c.Planning {
		return Planner
	}
	return Codex
}

// DirectContent returns a deterministic proposal. Only the meaning of an
// approval or a constrained native answer can still require semantic parsing.
func DirectContent(request Request, c Context) (Result, error) {
	if request.Recipient == "" {
		request.Recipient = InteractionRecipient(c)
	}
	result := Result{OperationID: request.OperationID, Patch: []userintent.Patch{}}
	if request.Mode != Interact || request.Recipient != InteractionRecipient(c) {
		return result, fmt.Errorf("typed recipient no longer owns this interaction")
	}
	var value any
	path := "/message"
	switch request.Recipient {
	case Planner, Codex:
		value = Content{Text: request.Text, Recipient: string(request.Recipient)}
	case Native:
		if len(c.NativeChoices) > 0 {
			return result, fmt.Errorf("native choice needs interpretation")
		}
		path, value = "/native_reply", request.Text
	default:
		return result, fmt.Errorf("approval reply needs interpretation")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return result, err
	}
	result.Patch = []userintent.Patch{{Op: "replace", Path: path, Value: raw}}
	_, err = Apply(c, result.Patch)
	return result, err
}

func (r Request) ValidateRouting(c Context) error {
	if err := r.Mode.Validate(); err != nil {
		return err
	}
	switch r.Mode {
	case "", Infer:
		if r.Recipient != "" {
			return fmt.Errorf("Infer has no typed recipient")
		}
	case Interact:
		if r.Recipient != "" && r.Recipient != InteractionRecipient(c) {
			return fmt.Errorf("interaction recipient changed")
		}
	default:
		return fmt.Errorf("%s uses its dedicated controls or conversation", r.Mode)
	}
	return nil
}

// ValidateRoutedPatch keeps typed transport authority at persistence and replay
// boundaries, including when a semantic approval parser returns a wrong field.
func ValidateRoutedPatch(r Request, c Context, patch []userintent.Patch) error {
	if err := r.ValidateRouting(c); err != nil {
		return err
	}
	if r.Mode != Interact {
		return nil
	}
	recipient := InteractionRecipient(c)
	path := "/message"
	if recipient == Approval {
		path = "/approval"
	}
	if recipient == Native {
		path = "/native_reply"
	}
	for _, op := range patch {
		if op.Path != path {
			return fmt.Errorf("typed interaction cannot change %s", op.Path)
		}
	}
	if recipient == Planner || recipient == Codex {
		after, err := Apply(c, patch)
		if err != nil {
			return err
		}
		if after.Message == nil || after.Message.Text != r.Text {
			return fmt.Errorf("typed content must preserve the exact owner text")
		}
	}
	if recipient == Native && len(c.NativeChoices) == 0 {
		after, err := Apply(c, patch)
		if err != nil {
			return err
		}
		if after.NativeReply == nil || *after.NativeReply != r.Text {
			return fmt.Errorf("typed reply must preserve the exact owner text")
		}
	}
	return nil
}

// Selecting message delivery is semantic; reproducing the owner's text and its
// sole current recipient is not. Bind both from the admitted request/state.
func BindContent(r Request, c Context, patch []userintent.Patch) ([]userintent.Patch, error) {
	bound := append([]userintent.Patch{}, patch...)
	for i, op := range bound {
		if op.Path != "/message" || (op.Op != "replace" && op.Op != "add") {
			continue
		}
		value := strings.TrimSpace(string(op.Value))
		if !strings.HasPrefix(value, "{") && value != "true" {
			continue
		}
		raw, err := json.Marshal(Content{Text: r.Text, Recipient: messageRecipient(c)})
		if err != nil {
			return nil, err
		}
		bound[i].Value = raw
	}
	return bound, nil
}
