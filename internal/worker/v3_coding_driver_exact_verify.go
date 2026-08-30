package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/operation"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (s *directCodingSession) verifyPreparedExactWorkspace(
	prepared *directCodingPreparedMutation,
) (verification directCodingVerification, begin directCodingCompletionTaskDisposition, resultErr error) {
	if s == nil || s.runtime == nil || s.runtime.ctx == nil || s.runtime.claim == nil ||
		prepared == nil || prepared.exactState == nil {
		return directCodingVerification{}, "", fmt.Errorf(
			"verified no-delta execution requires one prepared exact workspace",
		)
	}
	if err := s.requirePreparedExactWorkspace(prepared); err != nil {
		return directCodingVerification{}, "", err
	}
	sandbox, err := newDirectCodingWorkspaceSandbox(
		s.runtime.ctx, prepared.projection, prepared.exactState.authority.ID,
	)
	if err != nil {
		return directCodingVerification{}, "", err
	}
	defer func() {
		resultErr = errors.Join(resultErr, sandbox.Cleanup())
	}()
	if err := s.completePreparedTreeCognitionAtState(
		prepared, prepared.exactState.source.ID,
	); err != nil {
		return directCodingVerification{}, "", err
	}
	begin, err = s.BeginVerification()
	if err != nil {
		return directCodingVerification{}, "", err
	}
	verification, err = s.collectDirectCodingExactStateVerification(
		s.runtime.ctx, prepared, sandbox,
	)
	if err != nil {
		return directCodingVerification{}, "", err
	}
	return verification, begin, nil
}

func (s *directCodingSession) requirePreparedExactWorkspace(
	prepared *directCodingPreparedMutation,
) error {
	exact := prepared.exactState
	if err := exact.authority.validate(exact.source, prepared.allCommands); err != nil {
		return err
	}
	desired, err := s.directCodingAssemblyDesiredStates(exact.source, prepared.assembly)
	if err != nil {
		return err
	}
	_, err = workspacefacts.PlanMutation(
		s.runtime.ctx, exact.source, exact.authority.OwnerID, desired,
	)
	if !errors.Is(err, workspacefacts.ErrDesiredStateAlreadyExact) {
		if err == nil {
			return fmt.Errorf("verified no-delta workspace now requires a mutation")
		}
		return fmt.Errorf("revalidate verified no-delta workspace: %w", err)
	}
	if err := prepared.projection.VerifyExact(s.runtime.ctx); err != nil {
		return fmt.Errorf("verify exact direct-coding workspace projection: %w", err)
	}
	return nil
}

func (s *directCodingSession) collectDirectCodingExactStateVerification(
	ctx context.Context,
	prepared *directCodingPreparedMutation,
	sandbox *directCodingWorkspaceSandbox,
) (directCodingVerification, error) {
	if s == nil || s.runtime == nil || s.runtime.claim == nil || s.specification == nil ||
		s.program == nil || prepared == nil || prepared.exactState == nil || sandbox == nil {
		return directCodingVerification{}, fmt.Errorf(
			"verified no-delta verification requires one complete exact-state authority",
		)
	}
	exact := prepared.exactState
	commands := prepared.allCommands
	if err := exact.authority.validate(exact.source, commands); err != nil {
		return directCodingVerification{}, err
	}
	if err := sandbox.VerifyAuthority(ctx); err != nil {
		return directCodingVerification{}, err
	}
	jobID := s.runtime.claim.Authority.JobID
	stepID := s.runtime.claim.Authority.StepID
	records, diagnostic, testEvidence, err := s.executeDirectCodingVerificationJournal(
		ctx, commands, sandbox,
		func(index int, command testCommand, result operation.Result) (evidence.Record, error) {
			return directCodingExactStateVerificationEvidence(
				jobID, stepID, exact.authority, index, command, result,
			)
		},
		func(index int, command testCommand, detail string) evidence.Record {
			return directCodingSkippedExactStateVerificationEvidence(
				jobID, stepID, exact.authority, index, command, detail,
			)
		},
	)
	if err != nil {
		return directCodingVerification{}, err
	}
	if err := exact.source.VerifyExact(ctx); err != nil {
		return directCodingVerification{}, fmt.Errorf(
			"verify no-delta workspace after isolated commands: %w", err,
		)
	}
	evidenceIDs, err := s.persistCodeOwnedEvidenceIDs(operation.Result{Evidence: records})
	if err != nil {
		return directCodingVerification{}, fmt.Errorf(
			"persist verified no-delta command evidence: %w", err,
		)
	}
	passed := diagnostic == nil
	receiptSHA256, err := directCodingExactStateReceiptSHA256(
		exact.authority, evidenceIDs, passed,
	)
	if err != nil {
		return directCodingVerification{}, err
	}
	if len(prepared.primaryCommands) > len(evidenceIDs) {
		return directCodingVerification{}, fmt.Errorf(
			"verified no-delta command evidence is outside its sealed plan",
		)
	}
	verification := directCodingVerification{
		Passed: passed, TestsPassed: testEvidence,
		Commands:                directCodingPrimaryCommandLabels(commands),
		EvidenceIDs:             append([]int64(nil), evidenceIDs[:len(prepared.primaryCommands)]...),
		ExactStateAuthorityID:   exact.authority.ID,
		ExactStateReceiptSHA256: receiptSHA256,
		Diagnostic:              diagnostic,
	}
	if passed {
		sequence := s.nextSequence()
		s.completion.LatestCheckTurn = sequence
		if testEvidence {
			s.completion.LatestTestTurn = sequence
		}
		s.completion.LastTestHadNoTests = false
		s.lastCommands = append([]string(nil), verification.Commands...)
	}
	return verification, nil
}

func directCodingExactStateVerificationEvidence(
	jobID int64,
	stepID int64,
	authority directCodingExactStateAuthority,
	index int,
	command testCommand,
	result operation.Result,
) (evidence.Record, error) {
	if len(result.Evidence) != 1 || index < 0 || index >= len(authority.Verification.Commands) {
		return evidence.Record{}, fmt.Errorf(
			"exact-state verification command %d produced invalid evidence", index+1,
		)
	}
	planned := authority.Verification.Commands[index]
	record := result.Evidence[0]
	record.ID, record.JobID, record.StepID = 0, jobID, stepID
	record.Kind, record.Command = planned.Kind, planned.Command
	record.SourceType, record.SourceRef = "", ""
	record.Metadata = cloneDirectCodingEvidenceMetadata(record.Metadata)
	record.Metadata["succeeded"] = directCodingCommandSucceeded(result)
	record.Metadata["workspace_verification_role"] = string(command.WorkspaceRole)
	record.Metadata["verified_no_delta_authority_id"] = authority.ID
	return record, nil
}

func directCodingSkippedExactStateVerificationEvidence(
	jobID int64,
	stepID int64,
	authority directCodingExactStateAuthority,
	index int,
	command testCommand,
	detail string,
) evidence.Record {
	planned := authority.Verification.Commands[index]
	return evidence.Record{
		JobID: jobID, StepID: stepID,
		Kind: planned.Kind, ToolName: "command.run", Command: planned.Command,
		Summary:  "verification command not executed after an earlier authoritative failure",
		Warnings: []string{trimForBudget(detail, 1200)}, Confidence: 1,
		Metadata: map[string]any{
			"execution": false, "succeeded": false,
			"skipped_after_authoritative_failure": true,
			"workspace_verification_role":         string(command.WorkspaceRole),
			"verified_no_delta_authority_id":      authority.ID,
		},
	}
}
