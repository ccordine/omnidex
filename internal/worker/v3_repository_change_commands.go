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
	projection repositoryWorkspaceProjection,
	scope repositoryVerificationScope,
	commands []testCommand,
	authority repositoryVerificationEvidenceAuthority,
	assertExact func(context.Context) error,
) (resultErr error) {
	return session.runExistingRepositoryVerificationWithSink(
		projection, scope, commands, authority, assertExact,
		func(record evidence.Record) error {
			return session.runtime.writeEvidence(record)
		},
		true,
	)
}

type repositoryVerificationFailure struct {
	detail string
}

func (failure *repositoryVerificationFailure) Error() string {
	return failure.detail
}

func (session *directCodingSession) runExistingRepositoryVerificationWithSink(
	projection repositoryWorkspaceProjection,
	scope repositoryVerificationScope,
	commands []testCommand,
	authority repositoryVerificationEvidenceAuthority,
	assertExact func(context.Context) error,
	writeEvidence func(evidence.Record) error,
	writeAcceptance bool,
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
	if writeEvidence == nil {
		return fmt.Errorf("repository verification requires one exact evidence sink")
	}
	if authority == nil || !authority.allowsScope(scope) {
		return fmt.Errorf("repository verification authority does not permit scope %q", scope)
	}
	if err := validateRepositoryGoVerificationPlan(scope, commands); err != nil {
		return err
	}
	if err := authority.validate(commands); err != nil {
		return err
	}
	if err := projection.VerifyExact(session.runtime.ctx); err != nil {
		return fmt.Errorf("verify exact repository workspace projection: %w", err)
	}
	environment, err := newRepositoryGoVerificationEnvironment(session.runtime.ctx, projection)
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
		result, executionErr := environment.executeRepositoryGoVerification(
			session.runtime.ctx, projection, request,
		)
		if len(result.Evidence) != 1 && executionErr == nil {
			return fmt.Errorf("repository verification %q returned %d evidence records; expected one", label, len(result.Evidence))
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
			record.Metadata["succeeded"] = proofValid
			if err := writeEvidence(record); err != nil {
				evidenceErr = errors.Join(evidenceErr, fmt.Errorf("record repository verification %q: %w", label, err))
			}
		}
		if executionErr != nil {
			if len(result.Evidence) == 0 {
				failure := repositoryCommandFailureEvidence(authority, scope, command, executionErr)
				if writeErr := writeEvidence(failure); writeErr != nil {
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
			return &repositoryVerificationFailure{detail: failure.Error()}
		}
		if proofErr != nil {
			return &repositoryVerificationFailure{detail: fmt.Sprintf(
				"repository verification %q has invalid structured proof: %v", label, proofErr,
			)}
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
	if writeAcceptance {
		acceptance, err := repositoryVerificationAcceptanceEvidence(
			authority, scope, commands,
		)
		if err != nil {
			return err
		}
		if err := writeEvidence(acceptance); err != nil {
			return fmt.Errorf("record accepted repository verification plan: %w", err)
		}
		switch scope {
		case repositoryVerificationBaseline:
			session.runtime.svc.emitStepEvent(
				session.runtime.claim.Authority,
				"repository_verification_baseline_accepted",
				fmt.Sprintf("scope=%s plan=%s", scope, authority.planIdentity()),
			)
		case repositoryVerificationStaged:
			session.runtime.svc.emitStepEvent(
				session.runtime.claim.Authority,
				"repository_verification_plan_accepted",
				fmt.Sprintf("scope=%s plan=%s", scope, authority.planIdentity()),
			)
		default:
			return fmt.Errorf(
				"repository verification scope %q acceptance is owned by the workspace mutation journal",
				scope,
			)
		}
	}
	return nil
}

func (session *directCodingSession) collectExistingRepositoryVerification(
	projection repositoryWorkspaceProjection,
	scope repositoryVerificationScope,
	commands []testCommand,
	authority repositoryVerificationEvidenceAuthority,
	assertExact func(context.Context) error,
) ([]evidence.Record, string, error) {
	if session == nil || session.runtime == nil || session.runtime.claim == nil {
		return nil, "", fmt.Errorf("collect repository verification requires one active claim")
	}
	records := make([]evidence.Record, 0, len(commands))
	sink := func(record evidence.Record) error {
		if len(records) >= len(commands) {
			return fmt.Errorf("repository verification produced evidence outside its exact command plan")
		}
		commandAuthority, err := encodeWorkspaceVerificationCommand(commands[len(records)])
		if err != nil {
			return err
		}
		record.JobID = session.runtime.claim.Job.ID
		record.StepID = session.runtime.claim.Step.ID
		record.Command = commandAuthority
		record.SourceType = ""
		record.SourceRef = ""
		record.ID = 0
		record.CreatedAt = record.CreatedAt.UTC()
		records = append(records, record)
		return nil
	}
	err := session.runExistingRepositoryVerificationWithSink(
		projection, scope, commands, authority, assertExact, sink, false,
	)
	var failure *repositoryVerificationFailure
	if !errors.As(err, &failure) {
		if err != nil {
			return nil, "", err
		}
		if len(records) != len(commands) {
			return nil, "", fmt.Errorf("repository verification evidence count differs from its exact plan")
		}
		return records, "", nil
	}
	for index := len(records); index < len(commands); index++ {
		record := repositorySkippedCommandEvidence(
			authority, scope, commands[index], failure.Error(),
		)
		if err := sink(record); err != nil {
			return nil, "", err
		}
	}
	if len(records) != len(commands) {
		return nil, "", fmt.Errorf("failed repository verification evidence count differs from its exact plan")
	}
	return records, failure.Error(), nil
}

func repositorySkippedCommandEvidence(
	authority repositoryVerificationEvidenceAuthority,
	scope repositoryVerificationScope,
	command testCommand,
	priorFailure string,
) evidence.Record {
	metadata := repositoryCommandMetadata(
		map[string]any{
			"execution": false, "succeeded": false,
			"skipped_after_authoritative_failure": true,
		},
		authority, scope, command, false,
	)
	metadata["succeeded"] = false
	return evidence.Record{
		Kind: evidence.KindTestResult, ToolName: "command.run",
		Command:    directCodingCommandLabel(command),
		Summary:    "verification command not executed after an earlier authoritative failure",
		Warnings:   []string{trimForBudget(priorFailure, 1200)},
		Confidence: 1, Metadata: metadata,
	}
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
) (evidence.Record, error) {
	metadata := authority.metadata()
	metadata["repository_verification_scope"] = string(scope)
	metadata["repository_verification_command_count"] = len(commands)
	if scope == repositoryVerificationBaseline {
		metadata["repository_verification_baseline_accepted"] = true
		return evidence.Record{
			Kind: evidence.KindTestResult, SourceType: "command-baseline", SourceRef: "go",
			ToolName: "command.run", Summary: "exact source repository Go baseline accepted",
			Confidence: 1, Metadata: metadata,
		}, nil
	}
	if scope != repositoryVerificationStaged {
		return evidence.Record{}, fmt.Errorf(
			"repository verification scope %q acceptance is owned by the workspace mutation journal",
			scope,
		)
	}
	metadata["repository_verification_plan_accepted"] = true
	return evidence.Record{
		Kind: evidence.KindTestResult, SourceType: "command-plan", SourceRef: "go",
		ToolName: "command.run", Summary: "ordered repository Go verification plan accepted",
		Confidence: 1, Metadata: metadata,
	}, nil
}

func requireRepositoryGoOrdinaryFailure(result operation.Result) error {
	succeeded, succeededOK := result.Output["succeeded"].(bool)
	exitCode, exitOK := result.Output["exit_code"].(int)
	if !succeededOK || succeeded || !exitOK || exitCode != 1 {
		return fmt.Errorf("repository Go verification result has unregistered exit semantics")
	}
	return nil
}
