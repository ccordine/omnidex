package specialists

import (
	"errors"
	"fmt"
	"strings"
)

type Spec struct {
	ID           string `json:"id"`
	Purpose      string `json:"purpose"`
	Instructions string `json:"instructions,omitempty"`
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
