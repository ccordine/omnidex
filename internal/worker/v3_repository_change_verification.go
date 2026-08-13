package worker

import (
	"fmt"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

const (
	maxExistingRepositoryVerificationPackages = 32
	maxExistingRepositoryVerificationTests    = 256
	maxExistingRepositoryTestSelectorBytes    = 4096
)

type repositoryGoProofMode string

const (
	repositoryGoProofFocused repositoryGoProofMode = "focused"
	repositoryGoProofPackage repositoryGoProofMode = "package"
	repositoryGoProofBroad   repositoryGoProofMode = "broad"
)

type repositoryGoExpectedTest struct {
	SymbolID        string
	Name            string
	TargetSymbolIDs []string
}

type repositoryGoTestProof struct {
	Mode     repositoryGoProofMode
	Package  string
	Expected []repositoryGoExpectedTest
}

type repositoryGoTestGroup struct {
	packageArgument string
	expectedByID    map[string]repositoryGoExpectedTest
}

func existingRepositoryGoVerificationCommands(
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	contract repositoryfacts.ChangeContract,
) ([]testCommand, error) {
	if err := contract.Validate(snapshot, analysis); err != nil {
		return nil, fmt.Errorf("derive repository verification from exact contract: %w", err)
	}
	if analysis.Adapter.Name != "go" {
		return nil, fmt.Errorf("repository verification does not support adapter %q", analysis.Adapter.Name)
	}
	files := make(map[string]repositoryfacts.File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.ID] = file
	}
	symbols := make(map[string]repositoryfacts.Symbol, len(analysis.Symbols))
	for _, symbol := range analysis.Symbols {
		symbols[symbol.ID] = symbol
	}
	groups := make(map[string]*repositoryGoTestGroup)
	totalTests := 0
	for _, target := range contract.Targets {
		if len(target.VerificationSymbolIDs) == 0 {
			return nil, fmt.Errorf(
				"repository target %q has no direct verification symbol",
				target.SymbolID,
			)
		}
		for _, verificationID := range target.VerificationSymbolIDs {
			symbol, exists := symbols[verificationID]
			if !exists || symbol.Kind != "test" {
				return nil, fmt.Errorf(
					"repository target %q has invalid verification symbol %q",
					target.SymbolID, verificationID,
				)
			}
			if !runnableGoTestName(symbol.Name) {
				return nil, fmt.Errorf(
					"repository verification symbol %q is not one runnable Test function",
					verificationID,
				)
			}
			packageArgument, err := goVerificationPackage(files[symbol.FileID])
			if err != nil {
				return nil, fmt.Errorf("repository verification symbol %q: %w", verificationID, err)
			}
			group := groups[packageArgument]
			if group == nil {
				group = &repositoryGoTestGroup{
					packageArgument: packageArgument,
					expectedByID:    make(map[string]repositoryGoExpectedTest),
				}
				groups[packageArgument] = group
			}
			item, exists := group.expectedByID[verificationID]
			if !exists {
				item = repositoryGoExpectedTest{SymbolID: verificationID, Name: symbol.Name}
				totalTests++
			}
			item.TargetSymbolIDs = appendUniqueSorted(item.TargetSymbolIDs, target.SymbolID)
			group.expectedByID[verificationID] = item
		}
	}
	if len(groups) == 0 || len(groups) > maxExistingRepositoryVerificationPackages {
		return nil, fmt.Errorf(
			"repository verification requires 1-%d exact Go package scopes; received %d",
			maxExistingRepositoryVerificationPackages, len(groups),
		)
	}
	if totalTests > maxExistingRepositoryVerificationTests {
		return nil, fmt.Errorf(
			"repository verification requires %d exact tests and exceeds %d",
			totalTests, maxExistingRepositoryVerificationTests,
		)
	}
	packages := make([]string, 0, len(groups))
	for packageArgument := range groups {
		packages = append(packages, packageArgument)
	}
	sort.Strings(packages)
	commands := make([]testCommand, 0, len(packages)+1)
	for _, packageArgument := range packages {
		command, err := focusedRepositoryGoVerificationCommand(groups[packageArgument])
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	}
	commands = append(commands, testCommand{
		Family: "go", Name: "go", Args: []string{"test", "-json", "-count=1", "./..."},
		RepositoryProof: &repositoryGoTestProof{Mode: repositoryGoProofBroad, Package: "./..."},
	})
	for _, command := range commands {
		if err := validateRepositoryGoTestCommand(command); err != nil {
			return nil, fmt.Errorf(
				"repository verification command %q is invalid: %w",
				directCodingCommandLabel(command), err,
			)
		}
	}
	return commands, nil
}

func focusedRepositoryGoVerificationCommand(group *repositoryGoTestGroup) (testCommand, error) {
	if group == nil || len(group.expectedByID) == 0 {
		return testCommand{}, fmt.Errorf("focused repository verification requires exact tests")
	}
	expected := make([]repositoryGoExpectedTest, 0, len(group.expectedByID))
	for _, item := range group.expectedByID {
		expected = append(expected, item)
	}
	sort.Slice(expected, func(left, right int) bool {
		if expected[left].Name == expected[right].Name {
			return expected[left].SymbolID < expected[right].SymbolID
		}
		return expected[left].Name < expected[right].Name
	})
	selector, err := repositoryGoTestSelector(expected)
	if err != nil {
		return testCommand{}, fmt.Errorf("repository package %q: %w", group.packageArgument, err)
	}
	return testCommand{
		Family: "go", Name: "go",
		Args: []string{"test", "-json", "-count=1", "-run", selector, group.packageArgument},
		RepositoryProof: &repositoryGoTestProof{
			Mode: repositoryGoProofFocused, Package: group.packageArgument, Expected: expected,
		},
	}, nil
}

func goVerificationPackage(file repositoryfacts.File) (string, error) {
	if file.ID == "" || file.Kind != repositoryfacts.EntryRegular || file.Language != "go" || !file.Test {
		return "", fmt.Errorf("verification authority is not one exact regular Go test file")
	}
	directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(file.Path)))
	if directory == "." {
		return ".", nil
	}
	return "./" + directory, nil
}

func runnableGoTestName(name string) bool {
	if name == "TestMain" || !token.IsIdentifier(name) || !strings.HasPrefix(name, "Test") {
		return false
	}
	suffix := strings.TrimPrefix(name, "Test")
	if suffix == "" {
		return true
	}
	first, _ := utf8.DecodeRuneInString(suffix)
	return !unicode.IsLower(first)
}

func appendUniqueSorted(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	result := append(append([]string(nil), items...), value)
	sort.Strings(result)
	return result
}

func registeredGoTestAction(action string) bool {
	switch action {
	case "start", "run", "pause", "cont", "pass", "bench", "fail", "output", "skip":
		return true
	default:
		return false
	}
}
