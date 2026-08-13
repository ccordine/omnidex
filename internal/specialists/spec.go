package specialists

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Spec struct {
	ID              string `json:"id"`
	Purpose         string `json:"purpose"`
	Instructions    string `json:"instructions,omitempty"`
	inputSchemaRaw  json.RawMessage
	outputSchemaRaw json.RawMessage
}

func (s Spec) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return errors.New("specialist id is required")
	}
	if strings.TrimSpace(s.Purpose) == "" {
		return fmt.Errorf("specialist %s missing purpose", s.ID)
	}
	if strings.TrimSpace(s.Instructions) == "" {
		return fmt.Errorf("specialist %s missing instructions", s.ID)
	}
	return nil
}

func (s Spec) ValidateInputPayload(payload any) error {
	if len(s.inputSchemaRaw) == 0 {
		return nil
	}
	if err := ValidatePayloadAgainstSchema(s.inputSchemaRaw, payload); err != nil {
		return fmt.Errorf("specialist %s input payload: %w", s.ID, err)
	}
	return nil
}

func (s Spec) ValidateOutputPayload(payload any) error {
	if len(s.outputSchemaRaw) == 0 {
		return nil
	}
	if err := ValidatePayloadAgainstSchema(s.outputSchemaRaw, payload); err != nil {
		return fmt.Errorf("specialist %s output payload: %w", s.ID, err)
	}
	return nil
}

func (s Spec) OutputSchemaDocument() json.RawMessage {
	return append(json.RawMessage(nil), s.outputSchemaRaw...)
}
