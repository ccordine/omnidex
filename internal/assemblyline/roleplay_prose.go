package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

func validateRoleplayProse(label, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s is empty", label)
	}
	if (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid([]byte(trimmed)) {
		return fmt.Errorf("%s must be narrative prose, not copied JSON data", label)
	}
	return nil
}
