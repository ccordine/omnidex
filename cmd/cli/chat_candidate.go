package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/specialist"
)

type chatActionCandidate struct {
	Kind           string
	Input          string
	Summary        string
	SpecialistID   string
	SpecialistName string
}

func withCandidateSpecialist(candidate *chatActionCandidate) *chatActionCandidate {
	if candidate == nil {
		return nil
	}
	role := specialist.ForLocalCapability(candidate.Kind)
	candidate.SpecialistID = strings.TrimSpace(role.ID)
	candidate.SpecialistName = strings.TrimSpace(role.Name)
	return candidate
}

func candidateSpecialistRole(candidate *chatActionCandidate) specialist.Role {
	if candidate == nil {
		return specialist.ForLocalCapability("")
	}
	role := specialist.ForLocalCapability(candidate.Kind)
	if strings.TrimSpace(candidate.SpecialistID) != "" {
		role.ID = strings.TrimSpace(candidate.SpecialistID)
	}
	if strings.TrimSpace(candidate.SpecialistName) != "" {
		role.Name = strings.TrimSpace(candidate.SpecialistName)
	}
	return role
}

func candidateSummaryWithSpecialist(candidate *chatActionCandidate) string {
	if candidate == nil {
		return ""
	}
	role := candidateSpecialistRole(candidate)
	if strings.TrimSpace(role.ID) == "" {
		return candidate.Summary
	}
	return fmt.Sprintf("[%s] %s", strings.TrimSpace(role.ID), candidate.Summary)
}

func adoptFreshLocalShellSuggestionCandidate(
	candidate *chatActionCandidate,
	shellState *localShellState,
	previousSuggestedCommand string,
	previousSuggestedAt time.Time,
) *chatActionCandidate {
	if candidate == nil || shellState == nil {
		return candidate
	}
	if strings.TrimSpace(candidate.Kind) != "local_shell" {
		return candidate
	}
	if !shellState.LastSuggestedAt.After(previousSuggestedAt) {
		return candidate
	}
	suggested := strings.TrimSpace(shellState.LastSuggestedCommand)
	if suggested == "" {
		return candidate
	}
	if strings.EqualFold(strings.TrimSpace(previousSuggestedCommand), suggested) {
		return candidate
	}
	intent, ok := parseLocalShellIntent(suggested, nil)
	if !ok || strings.TrimSpace(intent.Action) != "run_command" || strings.TrimSpace(intent.Command) == "" {
		return candidate
	}
	return withCandidateSpecialist(&chatActionCandidate{
		Kind:    "local_shell",
		Input:   strings.TrimSpace(intent.Command),
		Summary: describeLocalShellIntent(intent),
	})
}

func localAutomationSourceLine(kind string, detail string) string {
	role := specialist.ForLocalCapability(kind)
	if strings.TrimSpace(role.ID) == "" {
		return strings.TrimSpace(kind) + ": " + strings.TrimSpace(detail)
	}
	return fmt.Sprintf("%s (%s): %s", strings.TrimSpace(kind), strings.TrimSpace(role.ID), strings.TrimSpace(detail))
}

func specialistModelOverrideForRole(roleID string) string {
	key := strings.TrimSpace(specialist.EnvVarForRoleID(roleID))
	if key == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(key))
}

func applySpecialistTurnOverrides(metadata map[string]any, roleID string) {
	if metadata == nil {
		return
	}
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return
	}
	metadata["specialist_role_id"] = roleID

	model := specialistModelOverrideForRole(roleID)
	if model == "" {
		return
	}
	for _, key := range []string{
		"model_plan",
		"model_analyze",
		"model_response",
		"model_verify",
	} {
		if _, exists := metadata[key]; exists {
			continue
		}
		metadata[key] = model
	}
}

func specialistRoleIDForCandidateTurn(candidate *chatActionCandidate) string {
	if candidate == nil {
		return ""
	}
	switch strings.TrimSpace(candidate.Kind) {
	case "local_media", "local_browser", "local_screen", "local_shell", "local_audio":
		return strings.TrimSpace(candidate.SpecialistID)
	default:
		return ""
	}
}
