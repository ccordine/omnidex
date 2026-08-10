package cognitiongauntlet

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
)

const rawShellContract = "Use one command only: rg QUERY; sed ARTIFACT; or worldctl KIND NAME=VALUE ... . Registered worldctl kinds are observe, navigate, take, use, and write. Do not use pipes, redirects, substitutions, absolute paths, or unregistered commands."

const rawShellArgument = cognition.ActionArgumentName("command")

func rawShellCatalog() (cognition.ActionCatalog, error) {
	schema, err := cognition.NewActionSchema(
		"omnidex.typed-shell-action.v1", "1.0.0", "shell",
		[]cognition.ActionParameterSpec{{Name: rawShellArgument, Required: true, MaxBytes: 4096}},
		cognition.EvidenceOptional,
	)
	if err != nil {
		return cognition.ActionCatalog{}, err
	}
	return cognition.NewActionCatalog(
		"omnidex.typed-shell-catalog.v1", "1.0.0", []cognition.ActionSchema{schema},
	)
}

func parseRawShellDecision(
	decision cognition.ActionRequest,
	world cognition.ActionCatalog,
) (cognition.ActionRequest, error) {
	if decision.Kind != "shell" || len(decision.Arguments) != 1 ||
		decision.Arguments[0].Name != rawShellArgument {
		return cognition.ActionRequest{}, fmt.Errorf("raw-shell decision must contain one exact command")
	}
	fields := strings.Fields(decision.Arguments[0].Value)
	if len(fields) == 2 && fields[0] == "rg" {
		return shellWorldRequest(world, "search", []cognition.ActionArgument{{Name: "query", Value: fields[1]}})
	}
	if len(fields) == 2 && fields[0] == "sed" {
		return shellWorldRequest(world, "read", []cognition.ActionArgument{{Name: "artifact", Value: fields[1]}})
	}
	if len(fields) < 2 || fields[0] != "worldctl" {
		return cognition.ActionRequest{}, fmt.Errorf("raw-shell command is outside the registered bounded vocabulary")
	}
	kind := cognition.ActionKind(fields[1])
	if kind == "search" || kind == "read" {
		return cognition.ActionRequest{}, fmt.Errorf("raw-shell search and read require rg and sed")
	}
	arguments := make([]cognition.ActionArgument, 0, len(fields)-2)
	seen := make(map[cognition.ActionArgumentName]struct{}, len(fields)-2)
	for _, field := range fields[2:] {
		name, value, found := strings.Cut(field, "=")
		argument := cognition.ActionArgumentName(name)
		if !found || name == "" || value == "" {
			return cognition.ActionRequest{}, fmt.Errorf("raw-shell worldctl arguments require NAME=VALUE")
		}
		if _, duplicate := seen[argument]; duplicate {
			return cognition.ActionRequest{}, fmt.Errorf("raw-shell worldctl argument %q is duplicated", name)
		}
		seen[argument] = struct{}{}
		arguments = append(arguments, cognition.ActionArgument{Name: argument, Value: value})
	}
	return shellWorldRequest(world, kind, arguments)
}

func shellWorldRequest(
	catalog cognition.ActionCatalog,
	kind cognition.ActionKind,
	arguments []cognition.ActionArgument,
) (cognition.ActionRequest, error) {
	schema, exists := catalog.Schema(kind)
	if !exists {
		return cognition.ActionRequest{}, fmt.Errorf("raw-shell command kind %q is absent from the world catalog", kind)
	}
	request, err := cognition.NewActionRequest(kind, arguments)
	if err != nil {
		return cognition.ActionRequest{}, err
	}
	if err := schema.ValidateRequest(request, nil); err != nil {
		if schema.EvidencePolicy != cognition.EvidenceRequired || !errors.Is(err, cognition.ErrInvalidEvidence) {
			return cognition.ActionRequest{}, err
		}
	}
	return request, nil
}

func rawShellCommand(request cognition.ActionRequest) (string, error) {
	values := make([]string, len(request.Arguments))
	for index, argument := range request.Arguments {
		if strings.ContainsAny(argument.Value, " \t\r\n") {
			return "", fmt.Errorf("raw-shell fixture argument requires unsupported quoting")
		}
		values[index] = string(argument.Name) + "=" + argument.Value
	}
	switch request.Kind {
	case "search":
		if len(request.Arguments) != 1 {
			return "", fmt.Errorf("raw-shell search fixture is invalid")
		}
		return "rg " + request.Arguments[0].Value, nil
	case "read":
		if len(request.Arguments) != 1 {
			return "", fmt.Errorf("raw-shell read fixture is invalid")
		}
		return "sed " + request.Arguments[0].Value, nil
	default:
		command := "worldctl " + string(request.Kind)
		if len(values) > 0 {
			command += " " + strings.Join(values, " ")
		}
		return command, nil
	}
}
