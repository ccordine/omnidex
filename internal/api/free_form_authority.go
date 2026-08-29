package api

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

func requireFreeFormAuthority(value, label string) (string, error) {
	if strings.TrimSpace(label) == "" {
		return "", fmt.Errorf("free-form authority label is required")
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, value); err != nil {
		return "", fmt.Errorf("%s: %w", label, err)
	}
	return value, nil
}
