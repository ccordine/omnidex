package userintent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// BuildContext preserves caller-scoped persisted facts and recent exchanges.
// The intent call resolves their meaning directly, without preliminary inference.
func BuildContext(ctx context.Context, c Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	for _, fact := range c.Facts {
		if err := validateText(fact); err != nil {
			return "", err
		}
	}
	if c.Pending != "" {
		if err := validateText(c.Pending); err != nil {
			return "", err
		}
	}
	for _, turn := range c.Recent {
		if turn.Role != "user" && turn.Role != "assistant" {
			return "", fmt.Errorf("unregistered conversation role")
		}
		if err := validateText(turn.Text); err != nil {
			return "", err
		}
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}
func validateText(value string) error {
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return fmt.Errorf("semantic text must be nonblank UTF-8 without NUL")
	}
	return nil
}
