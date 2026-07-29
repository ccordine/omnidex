package omni

import (
	"encoding/json"
	"fmt"
	"strings"
)

func buildExternalCodingPrompt(agent string, request ExternalCodingRequest) (string, error) {
	if strings.TrimSpace(request.Instruction) == "" {
		return "", fmt.Errorf("external coding instruction is required")
	}
	if strings.TrimSpace(request.Workspace) == "" {
		return "", fmt.Errorf("external coding workspace is required")
	}
	payload := struct {
		Role        string   `json:"role"`
		Workspace   string   `json:"workspace"`
		Instruction string   `json:"instruction"`
		Context     string   `json:"context,omitempty"`
		Rules       []string `json:"rules"`
	}{
		Role:        strings.TrimSpace(agent) + "_coding_executor",
		Workspace:   strings.TrimSpace(request.Workspace),
		Instruction: strings.TrimSpace(request.Instruction),
		Context:     strings.TrimSpace(request.Context),
		Rules: []string{
			"Work directly in the existing workspace and continue from its current files.",
			"Implement substantive production code; do not substitute placeholders or boilerplate for requested behavior.",
			"Intermediate files may reference work that is still being constructed. Verify after the implementation is ready.",
			"If a command or check fails, use its exact output to repair the current work instead of restarting.",
			"Report concise progress, changed files, verification run, and any explicit blocker.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode external coding prompt: %w", err)
	}
	return string(blob), nil
}
