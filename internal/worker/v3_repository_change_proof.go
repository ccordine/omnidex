package worker

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

type goTestEvent struct {
	Action     string `json:"Action"`
	Package    string `json:"Package"`
	ImportPath string `json:"ImportPath,omitempty"`
	Test       string `json:"Test,omitempty"`
	Output     string `json:"Output,omitempty"`
}

type goExpectedTestState struct {
	runs   int
	passes int
}

type goPackageProofState struct {
	started bool
	passed  bool
	skipped bool
}

func validateRepositoryGoVerificationPlan(
	scope repositoryVerificationScope,
	commands []testCommand,
) error {
	if len(commands) == 0 {
		return fmt.Errorf("repository %s verification plan is empty", scope)
	}
	for index, command := range commands {
		if err := validateRepositoryGoTestCommand(command); err != nil {
			return fmt.Errorf("repository verification command %d: %w", index+1, err)
		}
		mode := command.RepositoryProof.Mode
		if index == len(commands)-1 {
			if mode != repositoryGoProofBroad {
				return fmt.Errorf("repository verification plan must end with one broad proof")
			}
		} else if mode != repositoryGoProofFocused {
			return fmt.Errorf("repository verification plan contains a non-terminal broad proof")
		}
	}
	switch scope {
	case repositoryVerificationBaseline, repositoryVerificationStaged, repositoryVerificationAuthoritative:
		if len(commands) < 2 {
			return fmt.Errorf(
				"repository %s verification requires focused proofs before broad proof",
				scope,
			)
		}
	default:
		return fmt.Errorf("repository verification scope %q is not registered", scope)
	}
	return nil
}

func validateRepositoryGoTestCommand(command testCommand) error {
	if command.RepositoryProof == nil {
		return fmt.Errorf("repository Go verification command has no structured proof contract")
	}
	if command.Family != "go" || command.Name != "go" {
		return fmt.Errorf("repository Go verification command must use the Go family")
	}
	proof := *command.RepositoryProof
	var expectedArgs []string
	switch proof.Mode {
	case repositoryGoProofFocused:
		if proof.Package == "" || proof.Package == "./..." || len(proof.Expected) == 0 {
			return fmt.Errorf("focused repository Go proof requires one exact package and tests")
		}
		selector, err := repositoryGoTestSelector(proof.Expected)
		if err != nil {
			return err
		}
		expectedArgs = []string{"test", "-json", "-count=1", "-run", selector, proof.Package}
	case repositoryGoProofBroad:
		if proof.Package != "./..." || len(proof.Expected) != 0 {
			return fmt.Errorf("broad repository Go proof must bind only ./...")
		}
		expectedArgs = []string{"test", "-json", "-count=1", "./..."}
	default:
		return fmt.Errorf("repository Go proof mode %q is not registered", proof.Mode)
	}
	if !reflect.DeepEqual(command.Args, expectedArgs) {
		return fmt.Errorf("repository Go verification command differs from its proof contract")
	}
	if err := validateV3Command(command.Name, command.Args); err != nil {
		return err
	}
	return nil
}

func repositoryGoTestSelector(expected []repositoryGoExpectedTest) (string, error) {
	if len(expected) == 0 {
		return "", fmt.Errorf("repository Go test selector requires at least one exact test")
	}
	names := make([]string, len(expected))
	seenIDs := make(map[string]struct{}, len(expected))
	seenNames := make(map[string]struct{}, len(expected))
	for index, item := range expected {
		if item.SymbolID == "" || !runnableGoTestName(item.Name) || len(item.TargetSymbolIDs) == 0 {
			return "", fmt.Errorf("repository Go test expectation is incomplete or not runnable")
		}
		if !sort.StringsAreSorted(item.TargetSymbolIDs) {
			return "", fmt.Errorf("repository Go test %q target authority is not canonical", item.Name)
		}
		for targetIndex, targetID := range item.TargetSymbolIDs {
			if targetID == "" || targetIndex > 0 && targetID == item.TargetSymbolIDs[targetIndex-1] {
				return "", fmt.Errorf("repository Go test %q has invalid target authority", item.Name)
			}
		}
		if _, exists := seenIDs[item.SymbolID]; exists {
			return "", fmt.Errorf("repository Go test symbol %q is duplicated", item.SymbolID)
		}
		if _, exists := seenNames[item.Name]; exists {
			return "", fmt.Errorf("repository Go test name %q is ambiguous", item.Name)
		}
		seenIDs[item.SymbolID] = struct{}{}
		seenNames[item.Name] = struct{}{}
		names[index] = item.Name
	}
	sort.Strings(names)
	quoted := make([]string, len(names))
	for index, name := range names {
		quoted[index] = regexp.QuoteMeta(name)
	}
	selector := "^" + quoted[0] + "$"
	if len(quoted) > 1 {
		selector = "^(" + strings.Join(quoted, "|") + ")$"
	}
	if len([]byte(selector)) > maxExistingRepositoryTestSelectorBytes {
		return "", fmt.Errorf(
			"repository exact test selector has %d bytes and exceeds %d",
			len([]byte(selector)), maxExistingRepositoryTestSelectorBytes,
		)
	}
	return selector, nil
}

func validateRepositoryGoTestProof(proof repositoryGoTestProof, output string) error {
	if len([]byte(output)) > maxRepositoryGoVerificationStdoutBytes {
		return fmt.Errorf(
			"repository structured Go proof exceeds its exact %d-byte evidence bound",
			maxRepositoryGoVerificationStdoutBytes,
		)
	}
	if proof.Mode != repositoryGoProofFocused && proof.Mode != repositoryGoProofBroad {
		return fmt.Errorf("repository Go proof mode %q is not registered", proof.Mode)
	}
	expected := make(map[string]*goExpectedTestState, len(proof.Expected))
	for _, item := range proof.Expected {
		if _, duplicate := expected[item.Name]; duplicate {
			return fmt.Errorf("repository Go proof test name %q is duplicated", item.Name)
		}
		expected[item.Name] = &goExpectedTestState{}
	}
	if proof.Mode == repositoryGoProofFocused && len(expected) == 0 {
		return fmt.Errorf("focused repository Go proof has no expected tests")
	}
	if proof.Mode == repositoryGoProofBroad && len(expected) != 0 {
		return fmt.Errorf("broad repository Go proof cannot claim focused tests")
	}
	packages := make(map[string]*goPackageProofState)
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	eventCount := 0
	for index, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event goTestEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return fmt.Errorf("repository Go proof line %d is malformed JSON: %w", index+1, err)
		}
		eventCount++
		if err := consumeRepositoryGoTestEvent(proof.Mode, expected, packages, event); err != nil {
			return fmt.Errorf("repository Go proof line %d: %w", index+1, err)
		}
	}
	if eventCount == 0 || len(packages) == 0 {
		return fmt.Errorf("repository Go proof contains no structured package events")
	}
	for packageName, state := range packages {
		if !state.started || !state.passed && (proof.Mode != repositoryGoProofBroad || !state.skipped) {
			return fmt.Errorf("repository Go package %q has no terminal success proof", packageName)
		}
	}
	for name, state := range expected {
		if state.runs != 1 || state.passes != 1 {
			return fmt.Errorf(
				"repository expected test %q ran %d times and passed %d times; require exactly once",
				name, state.runs, state.passes,
			)
		}
	}
	return nil
}

func consumeRepositoryGoTestEvent(
	mode repositoryGoProofMode,
	expected map[string]*goExpectedTestState,
	packages map[string]*goPackageProofState,
	event goTestEvent,
) error {
	if event.Package == "" || !registeredGoTestAction(event.Action) {
		return fmt.Errorf("event has missing package or unregistered action %q", event.Action)
	}
	state := packages[event.Package]
	if state == nil {
		state = &goPackageProofState{}
		packages[event.Package] = state
	}
	if event.Action == "start" {
		if event.Test != "" || state.started {
			return fmt.Errorf("package %q has an invalid or duplicate start", event.Package)
		}
		state.started = true
		return nil
	}
	if !state.started {
		return fmt.Errorf("package %q emitted %q before start", event.Package, event.Action)
	}
	if event.Test == "" {
		return consumeRepositoryGoPackageEvent(mode, state, event)
	}
	if mode == repositoryGoProofBroad {
		if event.Action == "fail" {
			return fmt.Errorf("broad test %q failed despite successful command authority", event.Test)
		}
		return nil
	}
	name, _, _ := strings.Cut(event.Test, "/")
	testState := expected[name]
	if testState == nil {
		return fmt.Errorf("unexpected top-level test %q entered focused proof", name)
	}
	if event.Action == "skip" {
		return fmt.Errorf("expected test %q or one of its subtests was skipped", event.Test)
	}
	if event.Action == "fail" {
		return fmt.Errorf("expected test %q failed despite successful command authority", event.Test)
	}
	if event.Test != name {
		return nil
	}
	switch event.Action {
	case "run":
		testState.runs++
		if testState.runs > 1 {
			return fmt.Errorf("expected test %q ran more than once", name)
		}
	case "pass":
		if testState.runs != 1 {
			return fmt.Errorf("expected test %q passed without one exact run", name)
		}
		testState.passes++
		if testState.passes > 1 {
			return fmt.Errorf("expected test %q passed more than once", name)
		}
	}
	return nil
}

func consumeRepositoryGoPackageEvent(
	mode repositoryGoProofMode,
	state *goPackageProofState,
	event goTestEvent,
) error {
	switch event.Action {
	case "pass":
		if state.passed || state.skipped {
			return fmt.Errorf("package %q has more than one terminal result", event.Package)
		}
		state.passed = true
	case "skip":
		if mode != repositoryGoProofBroad {
			return fmt.Errorf("focused package %q was skipped", event.Package)
		}
		if state.passed || state.skipped {
			return fmt.Errorf("package %q has more than one terminal result", event.Package)
		}
		state.skipped = true
	case "fail":
		return fmt.Errorf("package %q ended with fail", event.Package)
	case "run", "pause", "cont", "bench":
		return fmt.Errorf("package %q emitted test action %q without a test", event.Package, event.Action)
	}
	return nil
}
