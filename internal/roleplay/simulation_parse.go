package roleplay

import (
	"fmt"
	"regexp"
	"strings"
)

var simulationCommandKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

func ParseSimulationAction(exact string) (SimulationAction, error) {
	if exact == "" || exact != strings.TrimSpace(exact) || strings.ContainsAny(exact, "\r\n\x00") {
		return SimulationAction{}, fmt.Errorf("simulation action must be one exact nonempty line")
	}
	if !strings.HasPrefix(exact, "/") {
		return SimulationAction{}, fmt.Errorf("simulation action must begin with a slash command")
	}
	body := exact[1:]
	key := body
	argument := ""
	hasArgument := false
	if split := strings.IndexByte(body, ' '); split >= 0 {
		key = body[:split]
		raw := body[split+1:]
		if raw == "" || len(raw) < 3 || raw[0] != '"' || raw[len(raw)-1] != '"' ||
			strings.ContainsAny(raw[1:len(raw)-1], "\"\\\r\n\x00") {
			return SimulationAction{}, fmt.Errorf("simulation action argument must be one exact quoted value")
		}
		argument = raw[1 : len(raw)-1]
		if argument == "" || argument != strings.TrimSpace(argument) || len([]byte(argument)) > MaxSimulationTextBytes {
			return SimulationAction{}, fmt.Errorf("simulation action argument must be 1 to %d exact trimmed bytes", MaxSimulationTextBytes)
		}
		hasArgument = true
	}
	if !simulationCommandKeyPattern.MatchString(key) {
		return SimulationAction{}, fmt.Errorf("simulation command name is invalid")
	}
	action := SimulationAction{Kind: SimulationActionInteraction, CommandKey: key, Argument: argument, HasArgument: hasArgument}
	switch key {
	case string(SimulationActionGive):
		action.Kind = SimulationActionGive
	case string(SimulationActionTake):
		action.Kind = SimulationActionTake
	}
	if (action.Kind == SimulationActionGive || action.Kind == SimulationActionTake) && !hasArgument {
		return SimulationAction{}, fmt.Errorf("/%s requires one exact quoted item name", key)
	}
	return action, nil
}

// CanonicalItemAction returns the only slash-command representation accepted
// for one persisted item name. Item definitions use the same validation, so an
// accepted name is always addressable without an escaping dialect.
func CanonicalItemAction(kind SimulationActionKind, itemName string) (string, error) {
	if kind != SimulationActionGive && kind != SimulationActionTake {
		return "", fmt.Errorf("canonical item action must be give or take")
	}
	if err := validateSimulationText("item name", itemName, 256, true); err != nil {
		return "", err
	}
	if strings.ContainsAny(itemName, "\"\\\r\n") {
		return "", fmt.Errorf("item name cannot contain quote, backslash, CR, or LF")
	}
	exact := "/" + string(kind) + ` "` + itemName + `"`
	parsed, err := ParseSimulationAction(exact)
	if err != nil {
		return "", fmt.Errorf("canonical item action: %w", err)
	}
	if parsed.Kind != kind || !parsed.HasArgument || parsed.Argument != itemName {
		return "", fmt.Errorf("canonical item action did not round-trip exactly")
	}
	return exact, nil
}
