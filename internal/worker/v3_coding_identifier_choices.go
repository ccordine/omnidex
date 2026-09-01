package worker

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	treesitter "github.com/tree-sitter/go-tree-sitter"
)

type directCodingIdentifierCandidate struct {
	name string
	role string
}

func directCodingTrialIdentifierCandidates(
	body string,
	startByte int,
	endByte int,
	candidates []directCodingIdentifierCandidate,
	validate func(string) error,
) []directCodingIdentifierCandidate {
	validated := make([]directCodingIdentifierCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.name == "" || startByte < 0 || endByte <= startByte || endByte > len(body) {
			continue
		}
		trial := body[:startByte] + candidate.name + body[endByte:]
		err := validate(trial)
		if err == nil {
			validated = append(validated, candidate)
			continue
		}
		var defect *assemblyline.SourceBodyDefect
		if !errors.As(err, &defect) {
			continue
		}
		failedStart, failedEnd, spanErr := defect.MutableRange(trial)
		if spanErr != nil {
			continue
		}
		insertedEnd := startByte + len(candidate.name)
		if failedEnd <= startByte || failedStart >= insertedEnd {
			validated = append(validated, candidate)
		}
	}
	return validated
}

func directCodingIdentifierChoices(
	language string,
	failed string,
	candidates []directCodingIdentifierCandidate,
) ([]assemblyline.OpaqueModelChoice, error) {
	byName := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		name := strings.TrimSpace(candidate.name)
		role := strings.TrimSpace(candidate.role)
		if name == "" || name == failed || role == "" {
			continue
		}
		if previous, exists := byName[name]; !exists || role < previous {
			byName[name] = role
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	choices := make([]assemblyline.OpaqueModelChoice, 0, len(names))
	for _, name := range names {
		choice, err := assemblyline.NewOpaqueModelChoice(
			fmt.Sprintf("%s %s named %q", language, byName[name], name), name,
		)
		if err != nil {
			return nil, err
		}
		choices = append(choices, choice)
	}
	return choices, nil
}

func directCodingTreeBindingAvailableAt(
	binding *treesitter.Node,
	at *treesitter.Node,
	isScope func(string) bool,
) bool {
	if binding == nil || at == nil {
		return false
	}
	availableAfter := binding.EndByte()
	for parent := binding.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case "variable_declarator", "let_declaration":
			availableAfter = parent.EndByte()
		}
		if !isScope(parent.Kind()) {
			continue
		}
		return availableAfter <= at.StartByte() &&
			parent.StartByte() <= at.StartByte() && at.EndByte() <= parent.EndByte()
	}
	return false
}

func directCodingTreeRoot(node *treesitter.Node) *treesitter.Node {
	if node == nil {
		return nil
	}
	root := node
	for parent := root.Parent(); parent != nil; parent = root.Parent() {
		root = parent
	}
	return root
}

func directCodingExplicitIdentifierAuthorities(
	input assemblyline.FragmentGenerationInput,
	valid func(string) bool,
) map[string]struct{} {
	result := make(map[string]struct{})
	for _, authority := range append(
		append([]string(nil), input.Capabilities...), input.PermittedSymbols...,
	) {
		trimmed := strings.TrimSpace(authority)
		if valid(trimmed) {
			result[trimmed] = struct{}{}
		}
		for _, match := range javaScriptDeclaredAPIPattern.FindAllStringSubmatch(trimmed, -1) {
			if valid(match[1]) {
				result[match[1]] = struct{}{}
			}
		}
	}
	return result
}
