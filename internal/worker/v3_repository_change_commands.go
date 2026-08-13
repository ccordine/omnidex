package worker

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/operation"
)

type repositoryVerificationScope string

const (
	repositoryVerificationBaseline      repositoryVerificationScope = "baseline"
	repositoryVerificationStaged        repositoryVerificationScope = "staged"
	repositoryVerificationAuthoritative repositoryVerificationScope = "authoritative"
)

func (session *directCodingSession) runExistingRepositoryVerification(
	root string,
	scope repositoryVerificationScope,
	commands []testCommand,
	authority repositoryVerificationEvidenceAuthority,
	ownership *repositoryGoCorrectionOwnership,
	assertExact func(context.Context) error,
) (resultErr error) {
	if session == nil || session.runtime == nil || session.runtime.ctx == nil {
		return fmt.Errorf("repository verification requires one active coding runtime")
	}
	if scope != repositoryVerificationBaseline && scope != repositoryVerificationStaged &&
		scope != repositoryVerificationAuthoritative {
		return fmt.Errorf("repository verification scope %q is not registered", scope)
	}
	if len(commands) == 0 {
		return fmt.Errorf("repository verification requires at least one command")
	}
	if assertExact == nil {
		return fmt.Errorf("repository verification requires one exact final-state assertion")
	}
	if authority == nil || !authority.allowsScope(scope) {
		return fmt.Errorf("repository verification authority does not permit scope %q", scope)
	}
	pathlessDesiredStage := scope == repositoryVerificationStaged && ownership == nil &&
		repositoryVerificationAuthorityOwnsDesiredGraph(authority)
	if scope == repositoryVerificationStaged && ownership == nil && !pathlessDesiredStage {
		return fmt.Errorf("staged repository verification requires exact parsed target ownership or desired-state authority")
	}
	if scope == repositoryVerificationAuthoritative && ownership != nil {
		return fmt.Errorf("authoritative repository verification cannot accept correction ownership")
	}
	if err := validateRepositoryGoVerificationPlan(scope, commands); err != nil {
		return err
	}
	if err := authority.validate(commands); err != nil {
		return err
	}
	environment, err := newRepositoryGoVerificationEnvironment(session.runtime.ctx, root)
	if err != nil {
		return fmt.Errorf("construct exact repository Go verification environment: %w", err)
	}
	defer func() {
		if environment != nil {
			resultErr = errors.Join(resultErr, environment.Cleanup())
		}
	}()
	for _, command := range commands {
		label := directCodingCommandLabel(command)
		if err := assertExact(session.runtime.ctx); err != nil {
			return fmt.Errorf(
				"assert exact repository bytes before verification %q: %w", label, err,
			)
		}
		request, err := repositoryGoVerificationRequestFromCommand(command)
		if err != nil {
			return fmt.Errorf("construct repository verification request %q: %w", label, err)
		}
		result, executionErr := environment.executeRepositoryGoVerification(session.runtime.ctx, root, request)
		if len(result.Evidence) == 0 && executionErr == nil {
			return fmt.Errorf("repository verification %q returned no exact command evidence", label)
		}
		commandSucceeded := executionErr == nil && directCodingCommandSucceeded(result)
		var proofErr error
		if commandSucceeded {
			proofErr = validateRepositoryGoTestProof(
				*command.RepositoryProof, operationResultText(result.Output, "stdout"),
			)
		}
		proofValid := commandSucceeded && proofErr == nil
		var evidenceErr error
		for _, record := range result.Evidence {
			record.ToolName = "command.run"
			record.Metadata = repositoryCommandMetadata(
				record.Metadata, authority, scope, command, proofValid,
			)
			if err := session.runtime.writeEvidence(record); err != nil {
				evidenceErr = errors.Join(evidenceErr, fmt.Errorf("record repository verification %q: %w", label, err))
			}
		}
		if executionErr != nil {
			if len(result.Evidence) == 0 {
				failure := repositoryCommandFailureEvidence(authority, scope, command, executionErr)
				if writeErr := session.runtime.writeEvidence(failure); writeErr != nil {
					evidenceErr = errors.Join(evidenceErr, fmt.Errorf("record repository verification failure: %w", writeErr))
				}
			}
			return errors.Join(
				fmt.Errorf("execute repository verification %q: %w", label, executionErr),
				evidenceErr,
			)
		}
		if evidenceErr != nil {
			return evidenceErr
		}
		if !commandSucceeded {
			failure := fmt.Errorf(
				"repository verification %q failed: %s",
				label, trimForBudget(directCodingCommandResult(result), 1200),
			)
			if exitErr := requireRepositoryGoOrdinaryFailure(result); exitErr != nil {
				return errors.Join(failure, exitErr)
			}
			if exactErr := assertExact(session.runtime.ctx); exactErr != nil {
				return errors.Join(failure, fmt.Errorf(
					"assert exact repository bytes before failure classification: %w", exactErr,
				))
			}
			if scope != repositoryVerificationStaged || pathlessDesiredStage {
				return failure
			}
			owned, classifyErr := classifyRepositoryGoVerificationFailure(
				command, operationResultText(result.Output, "stdout"), *ownership,
			)
			if classifyErr != nil {
				return errors.Join(failure, fmt.Errorf(
					"repository staged failure is not model-correctable: %w", classifyErr,
				))
			}
			return owned
		}
		if proofErr != nil {
			return fmt.Errorf("repository verification %q has invalid structured proof: %w", label, proofErr)
		}
		session.runtime.svc.emitStepEvent(
			session.runtime.claim.Authority,
			"repository_verification_command_passed",
			fmt.Sprintf("scope=%s command=%s", scope, directCodingEventToken(label, "unknown")),
		)
	}
	if err := assertExact(session.runtime.ctx); err != nil {
		return fmt.Errorf("assert exact repository bytes before plan acceptance: %w", err)
	}
	if err := environment.Cleanup(); err != nil {
		return fmt.Errorf("clean exact repository Go verification environment before acceptance: %w", err)
	}
	environment = nil
	acceptance := repositoryVerificationAcceptanceEvidence(authority, scope, commands)
	if err := session.runtime.writeEvidence(acceptance); err != nil {
		return fmt.Errorf("record accepted repository verification plan: %w", err)
	}
	session.runtime.svc.emitStepEvent(
		session.runtime.claim.Authority,
		repositoryVerificationAcceptanceEvent(scope),
		fmt.Sprintf("scope=%s plan=%s", scope, authority.planIdentity()),
	)
	return nil
}

func repositoryVerificationAuthorityOwnsDesiredGraph(
	authority repositoryVerificationEvidenceAuthority,
) bool {
	if authority == nil {
		return false
	}
	owner, ok := authority.metadata()["repository_desired_artifact_graph_id"].(string)
	return ok && validRepositoryVerificationOpaqueID(owner, "desired_graph_")
}

func repositoryCommandMetadata(
	current map[string]any,
	authority repositoryVerificationEvidenceAuthority,
	scope repositoryVerificationScope,
	command testCommand,
	proofValid bool,
) map[string]any {
	metadata := make(map[string]any, len(current)+12)
	for key, value := range current {
		if key != "workspace" && key != "repository_verification_accepted" &&
			key != "repository_verification_plan_accepted" &&
			key != "repository_verification_baseline_accepted" {
			metadata[key] = value
		}
	}
	for key, value := range authority.metadata() {
		metadata[key] = value
	}
	metadata["repository_verification_scope"] = string(scope)
	metadata["repository_structured_proof_valid"] = proofValid
	if command.RepositoryProof != nil {
		ids := make([]string, len(command.RepositoryProof.Expected))
		names := make([]string, len(command.RepositoryProof.Expected))
		targetSet := make(map[string]struct{})
		bindings := make([]map[string]any, len(command.RepositoryProof.Expected))
		for index, item := range command.RepositoryProof.Expected {
			ids[index] = item.SymbolID
			names[index] = item.Name
			targets := append([]string(nil), item.TargetSymbolIDs...)
			for _, targetID := range targets {
				targetSet[targetID] = struct{}{}
			}
			bindings[index] = map[string]any{
				"test_symbol_id": item.SymbolID, "test_name": item.Name,
				"target_symbol_ids": targets,
			}
		}
		targets := make([]string, 0, len(targetSet))
		for targetID := range targetSet {
			targets = append(targets, targetID)
		}
		sort.Strings(targets)
		metadata["repository_verification_proof_mode"] = string(command.RepositoryProof.Mode)
		metadata["repository_verification_package"] = command.RepositoryProof.Package
		metadata["repository_expected_test_ids"] = ids
		metadata["repository_expected_test_names"] = names
		metadata["repository_verified_target_ids"] = targets
		metadata["repository_test_target_bindings"] = bindings
	}
	return metadata
}

func repositoryCommandFailureEvidence(
	authority repositoryVerificationEvidenceAuthority,
	scope repositoryVerificationScope,
	command testCommand,
	err error,
) evidence.Record {
	detail := trimForBudget(err.Error(), 1200)
	label := directCodingCommandLabel(command)
	return evidence.Record{
		Kind: evidence.KindTestResult, SourceType: "command", SourceRef: "go",
		ToolName: "command.run", Command: label,
		Summary:  "repository verification command failed before producing a result",
		Warnings: []string{detail}, Confidence: 1,
		Metadata: repositoryCommandMetadata(
			map[string]any{"succeeded": false}, authority, scope, command, false,
		),
	}
}

func repositoryVerificationAcceptanceEvidence(
	authority repositoryVerificationEvidenceAuthority,
	scope repositoryVerificationScope,
	commands []testCommand,
) evidence.Record {
	metadata := authority.metadata()
	metadata["repository_verification_scope"] = string(scope)
	metadata["repository_verification_command_count"] = len(commands)
	if scope == repositoryVerificationBaseline {
		metadata["repository_verification_baseline_accepted"] = true
		return evidence.Record{
			Kind: evidence.KindTestResult, SourceType: "command-baseline", SourceRef: "go",
			ToolName: "command.run", Summary: "exact source repository Go baseline accepted",
			Confidence: 1, Metadata: metadata,
		}
	}
	metadata["repository_verification_plan_accepted"] = true
	return evidence.Record{
		Kind: evidence.KindTestResult, SourceType: "command-plan", SourceRef: "go",
		ToolName: "command.run", Summary: "ordered repository Go verification plan accepted",
		Confidence: 1, Metadata: metadata,
	}
}

func repositoryVerificationAcceptanceEvent(scope repositoryVerificationScope) string {
	if scope == repositoryVerificationBaseline {
		return "repository_verification_baseline_accepted"
	}
	return "repository_verification_plan_accepted"
}

func requireRepositoryGoOrdinaryFailure(result operation.Result) error {
	succeeded, succeededOK := result.Output["succeeded"].(bool)
	exitCode, exitOK := result.Output["exit_code"].(int)
	if !succeededOK || succeeded || !exitOK || exitCode != 1 {
		return fmt.Errorf("repository Go verification result has unregistered exit semantics")
	}
	return nil
}
