package worker

import (
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const directCodingRustStageTimeout = 2 * time.Minute

func newDirectCodingRustProjectStageExecutor(
	session *directCodingSession,
	_ directCodingProgram,
) (directCodingProjectStageExecutor, error) {
	return newDirectCodingLanguageProjectStageExecutor(session, directCodingLanguageStageConfig{
		Language: "rust", AdapterID: "rust", Timeout: directCodingRustStageTimeout,
		ProjectFragment:    assemblyline.ProjectRustFragment,
		ValidateFragment:   validateDirectCodingRustFragment,
		ValidateAcceptance: validateDirectCodingRustAcceptance,
		TaskCommands:       rustCommandLineTaskVerificationCommands,
		FinalCommands:      rustCommandLineVerificationCommands,
	})
}

func rustCommandLineTaskVerificationCommands(
	context assemblyline.ApplicationTaskContext,
	program directCodingProgram,
) ([]testCommand, error) {
	if context.WorkloadSHA256 == "" || context.WorkloadSHA256 != program.Workload.SHA256 {
		return nil, fmt.Errorf("Rust task verification context differs from program workload authority")
	}
	pair, err := rustCommandLineTaskPair(program.Coverage, context.Task.TaskID)
	if err != nil {
		return nil, err
	}
	target, err := rustCommandLineTestTarget(pair.VerificationPath)
	if err != nil {
		return nil, err
	}
	return []testCommand{{
		Family: "cargo", Name: "cargo",
		Args:    []string{"test", "--locked", "--offline", "--test", target},
		Purpose: verificationTest,
	}}, nil
}

func validateDirectCodingRustAcceptance(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	source string,
) error {
	implementationName, err := rustAcceptanceImplementationName(stage, ref)
	if err != nil {
		return fmt.Errorf("Rust acceptance block %s: %w", ref.Block.ID, err)
	}
	fixtureName, err := rustAcceptanceFixtureName(stage, ref, implementationName)
	if err != nil {
		return fmt.Errorf("Rust acceptance block %s: %w", ref.Block.ID, err)
	}
	if _, err := assemblyline.ValidateRustFragment(ref.Block.Signature, source); err != nil {
		return fmt.Errorf("Rust acceptance block %s: %w", ref.Block.ID, err)
	}
	requiredAssertions, err := directCodingAcceptanceCriterionCount(stage, ref)
	if err != nil {
		return err
	}
	calledImplementation, assertions, err := inspectRustAcceptance(
		[]byte(source), implementationName, fixtureName,
	)
	if err != nil {
		return fmt.Errorf("Rust acceptance block %s: %w", ref.Block.ID, err)
	}
	if !calledImplementation || assertions < requiredAssertions {
		return fmt.Errorf(
			"Rust acceptance block %s must bind %s's result using %s and prove all %d frozen criteria with distinct result-field assertions",
			ref.Block.ID, implementationName, fixtureName, requiredAssertions,
		)
	}
	return nil
}

func rustAcceptanceFixtureName(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	implementationName string,
) (string, error) {
	expected := "representative_capability_results_for_" + implementationName
	expectedSignature := "fn " + expected + "() -> CapabilityResults"
	found := false
	for _, dependencyID := range ref.Block.DependsOn {
		dependency, exists := directCodingSourceBlueprintBlock(stage.Source, dependencyID)
		if !exists || dependency.Role != assemblyline.SourceBlockTaskSupport ||
			dependency.API != expectedSignature {
			continue
		}
		if found {
			return "", fmt.Errorf("multiple representative capability fixtures")
		}
		found = true
	}
	if !found {
		return "", fmt.Errorf("no representative capability fixture %s", expected)
	}
	return expected, nil
}

func rustAcceptanceImplementationName(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	name := ""
	for _, dependencyID := range ref.Block.DependsOn {
		dependency, exists := directCodingSourceBlueprintBlock(stage.Source, dependencyID)
		if !exists || dependency.Role != assemblyline.SourceBlockTaskImplementation {
			continue
		}
		const prefix = "pub fn "
		if !strings.HasPrefix(dependency.Signature, prefix) {
			return "", fmt.Errorf("implementation signature %q is not a public Rust function", dependency.Signature)
		}
		separator := strings.IndexByte(dependency.Signature, '(')
		if separator <= len(prefix) {
			return "", fmt.Errorf("implementation signature %q has no callable name", dependency.Signature)
		}
		if name != "" {
			return "", fmt.Errorf("multiple implementation owners")
		}
		name = strings.TrimSpace(dependency.Signature[len(prefix):separator])
	}
	if name == "" {
		return "", fmt.Errorf("no implementation owner")
	}
	return name, nil
}
