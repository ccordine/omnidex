package worker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

const codingAgentObjectiveRequirementID = "objective"

func (runtime *nativeRuntimeV3) runAgentPlanningAction(configuration codingagent.Configuration) error {
	if runtime.svc.agentProviders == nil {
		return fmt.Errorf("Agents mode has no registered planning environment")
	}
	scope, err := runtime.svc.workspaceScopeForV3Job(runtime.ctx, runtime.claim.Job)
	if err != nil {
		return err
	}
	if err := runtime.acquireWorkspaceMutationFence(scope.Root); err != nil {
		return err
	}
	identity, err := projectroot.DirectoryIdentity(scope.Root)
	if err != nil {
		return err
	}
	agentContext, cancelAgent, err := runtime.svc.nativeAgentWorkspaceContext(runtime.ctx, runtime.claim.Job, scope, identity)
	if err != nil {
		return err
	}
	defer cancelAgent()
	request, err := runtime.directCodingRequest()
	if err != nil {
		return err
	}
	intake, err := runtime.agentIntakeContext(request, scope.Root)
	if err != nil {
		return err
	}
	providers, _, err := runtime.svc.agentProviders()
	if err != nil {
		return err
	}
	authority := (&directCodingSession{request: request}).directCodingAuthority()
	controller := codingagent.Controller{
		JobID: runtime.claim.Job.ID, Generation: runtime.claim.Authority.Generation,
		Root: scope.Root, Configuration: configuration,
		Requirements:     []codingagent.Requirement{{ID: codingAgentObjectiveRequirementID, Outcome: authority}},
		Objective:        &codingagent.Objective{Instruction: request.Instruction, Clarifications: request.Feedback},
		ArtifactBindings: intake.ArtifactBindings, PlanningEnvironment: agentPlanningEnvironment(),
		Providers:   providers,
		Environment: &codingAgentEnvironment{runtime: runtime, root: scope.Root, identity: identity},
		Evidence:    runtime.svc.repo.CodingAgentEvidence(runtime.claim.Authority),
		Activity: func(activity model.AgentActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID, Attempt: runtime.claim.Authority.Attempt, Kind: "agent_activity", AgentActivity: &activity})
		},
		CallActivity: func(activity model.AgentCallActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID, Attempt: runtime.claim.Authority.Attempt, Kind: "agent_call_activity", AgentCallActivity: &activity})
		},
	}
	stopHeartbeat := runtime.svc.startProgressHeartbeat(runtime.ctx, runtime.claim.Authority, "scoped-agent-planning")
	defer stopHeartbeat()
	exactTurnText := authority
	if count := len(request.Feedback); count > 0 {
		exactTurnText = request.Feedback[count-1]
	}
	turn := codingagent.PlanningTurn{
		OperationID: fmt.Sprintf("planning:%d:%d", runtime.claim.Job.ID, runtime.claim.Authority.Generation),
		OwnerTurnID: fmt.Sprintf("owner:%d:%d", runtime.claim.Job.ID, runtime.claim.Authority.Generation),
		Text:        exactTurnText, ObservedRevision: request.ObservedLedgerRevision,
	}
	if request.PlanningOperationID != "" {
		turn.OperationID = request.PlanningOperationID
	}
	turn, err = planningTurnFromDurableAnswer(runtime.ctx, controller.Evidence, turn)
	if err != nil {
		return err
	}
	existing, exists, err := runtime.svc.repo.CurrentCodingLedgerForJob(runtime.ctx, runtime.claim.Job.ID)
	if err != nil {
		return err
	}
	alreadyApplied := false
	if exists {
		_, alreadyApplied, err = runtime.svc.repo.CodingLedgerRevisionForOperation(runtime.ctx, existing.LedgerID, turn.OperationID)
		if err != nil {
			return err
		}
	}
	if exists && turn.ObservedRevision == 0 && !alreadyApplied {
		return fmt.Errorf("planning turn %q lacks the exact ledger revision observed by the client", turn.OperationID)
	}
	var ledger queue.CodingLedgerRevision
	if exists && alreadyApplied {
		ledger, err = resumeCodingLedgerPlanningCoverage(agentContext, runtime.svc.repo, &controller, existing.LedgerID, turn)
	} else if exists {
		ledger, err = applyCodingLedgerPlanningTransaction(agentContext, runtime.svc.repo, &controller, existing.LedgerID, turn, nil)
	} else {
		ledger, err = createCodingLedgerPlanningTransaction(agentContext, runtime.svc.repo, &controller, runtime.claim.Job.ID, turn)
	}
	if err != nil {
		var questions *codingagent.QuestionsPending
		if errors.As(err, &questions) {
			if persistErr := persistPlanningQuestions(agentContext, controller.Evidence, controller.Generation, ledger.Revision, questions.Questions); persistErr != nil {
				return persistErr
			}
			if pauseErr := runtime.transitionAfterSessionMessages(agentContext, func() error {
				return runtime.svc.repo.PauseCodingAgentStep(agentContext, runtime.claim.Authority, questions.Questions)
			}); pauseErr != nil {
				return pauseErr
			}
		}
		return err
	}
	if err := runtime.transitionAfterSessionMessages(runtime.ctx, func() error {
		return runtime.svc.repo.MarkCodingLedgerReviewReady(runtime.ctx, runtime.claim.Authority, ledger.LedgerID, ledger.Revision)
	}); err != nil {
		return err
	}
	runtime.svc.emitStepEvent(runtime.claim.Authority, "coding_plan_review_ready", fmt.Sprintf("ledger=%d revision=%d tasks=%d", ledger.LedgerID, ledger.Revision, len(ledger.Ledger.Tasks)))
	return errCodingPlanReviewPending
}

// A question answer is a new owner turn, not permission to rerun coverage for
// the operation which produced the question. Its lifecycle operation identity,
// exact text, and observed ledger revision all come from durable database
// evidence, so a fresh provider session needs no conversation memory.
func planningTurnFromDurableAnswer(
	ctx context.Context,
	evidence codingagent.EvidenceStore,
	fallback codingagent.PlanningTurn,
) (codingagent.PlanningTurn, error) {
	entries, err := evidence.Read(ctx)
	if err != nil {
		return codingagent.PlanningTurn{}, err
	}
	questions := make(map[int64]codingagent.Entry)
	for _, entry := range entries {
		switch entry.Phase {
		case "questions":
			if entry.TaskID == "" {
				questions[entry.ID] = entry
			}
		case "answers":
			question, known := questions[entry.QuestionID]
			if !known {
				continue
			}
			if entry.OperationID == "" || len(entry.Answers) != 1 || question.LedgerRevision < 1 {
				return codingagent.PlanningTurn{}, fmt.Errorf("durable planning answer lacks operation, exact text, or observed ledger revision")
			}
			fallback = codingagent.PlanningTurn{
				OperationID: entry.OperationID, OwnerTurnID: "owner:" + entry.OperationID,
				Text: entry.Answers[0], ObservedRevision: question.LedgerRevision,
			}
		}
	}
	return fallback, nil
}

func persistPlanningQuestions(
	ctx context.Context,
	evidence codingagent.EvidenceStore,
	generation int64,
	revision int64,
	questions []string,
) error {
	if generation < 1 || revision < 1 {
		return fmt.Errorf("planning questions lack their generation or observed ledger revision")
	}
	entries, err := evidence.Read(ctx)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		latest := entries[len(entries)-1]
		if latest.Phase == "questions" && latest.TaskID == "" && latest.LedgerRevision == revision && slices.Equal(latest.Questions, questions) {
			return nil
		}
	}
	_, err = evidence.Append(ctx, codingagent.Entry{
		Generation: generation, Phase: "questions", LedgerRevision: revision,
		Questions: append([]string(nil), questions...),
	})
	return err
}

func resumeCodingLedgerPlanningCoverage(
	ctx context.Context,
	repository codingLedgerPlanningRepository,
	controller *codingagent.Controller,
	ledgerID int64,
	turn codingagent.PlanningTurn,
) (queue.CodingLedgerRevision, error) {
	current, err := readCompleteCodingLedger(ctx, repository, ledgerID)
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	turn.ObservedRevision = current.Revision
	coverage, err := controller.AnalyzePlanningCoverage(ctx, turn, current.Ledger,
		model.OwnerTurnDisposition{Statements: current.OwnerTurnDispositions})
	if err != nil {
		return current, err
	}
	if coverage.Covered {
		return current, nil
	}
	return current, planningCoverageUnready(coverage)
}

func (runtime *nativeRuntimeV3) agentIntakeContext(request directCodingRequest, root string) (*codingagent.IntakeContext, error) {
	authority := (&directCodingSession{request: request}).directCodingAuthority()
	provenance, err := objectiveInstructionPathProvenance(runtime.ctx, root, authority)
	if err != nil {
		return nil, err
	}
	redacted, identities, err := assemblyline.RedactArtifactIdentities(authority, provenance)
	if err != nil {
		return nil, err
	}
	bindings := make([]codingagent.ArtifactBinding, len(identities))
	for index, identity := range identities {
		bindings[index] = codingagent.ArtifactBinding{Token: identity.Token, Value: identity.Value}
	}
	intake := &codingagent.IntakeContext{RedactedRequest: redacted, ArtifactBindings: bindings}
	return intake, intake.Validate()
}

func agentPlanningEnvironment() *codingagent.PlanningEnvironment {
	environment := &codingagent.PlanningEnvironment{NewFilesAllowed: true, Executables: []string{}}
	for _, name := range []string{"go", "python3", "sh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != "" {
			environment.Executables = append(environment.Executables, name)
		}
	}
	return environment
}

type codingLedgerPlanningRepository interface {
	CreateCodingLedger(context.Context, queue.CreateCodingLedgerCommand) (queue.CodingLedgerRevision, error)
	ReadCodingLedger(context.Context, int64, queue.CodingLedgerReadOptions) (queue.CodingLedgerRevision, error)
	ApplyCodingLedgerMutation(context.Context, queue.ApplyCodingLedgerMutationCommand) (queue.CodingLedgerRevision, error)
}

type codingLedgerPlanningAuditRepository interface {
	RecordCodingLedgerPlanningAudit(context.Context, queue.RecordCodingLedgerPlanningAuditCommand) error
	CodingLedgerPlanningTurns(context.Context, int64) ([]codingagent.PersistedPlanningTurn, error)
}

func codingLedgerPlanningRoutes(configuration codingagent.Configuration, revision int64) []queue.CodingLedgerRoute {
	values := []struct {
		role  string
		route codingagent.Route
	}{
		{role: "planner", route: configuration.Planner},
		{role: "executor", route: configuration.Executor},
		{role: "reviewer", route: configuration.Reviewer},
	}
	routes := make([]queue.CodingLedgerRoute, len(values))
	for index, value := range values {
		routes[index] = queue.CodingLedgerRoute{
			Role: value.role, ConfigurationRevision: revision,
			Provider: value.route.Provider, Model: value.route.Model,
			ReasoningEffort: value.route.Effort, ServiceTier: value.route.ServiceTier,
		}
	}
	return routes
}

// createCodingLedgerPlanningTransaction is the sole full-ledger planning pass.
// The proposal is persisted before semantic coverage so provider interruption
// always leaves the exact current proposal as the durable restart point.
func createCodingLedgerPlanningTransaction(
	ctx context.Context,
	repository codingLedgerPlanningRepository,
	controller *codingagent.Controller,
	jobID int64,
	turn codingagent.PlanningTurn,
) (queue.CodingLedgerRevision, error) {
	proposal, err := controller.ProposePlanningLedger(ctx, turn)
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	created, err := repository.CreateCodingLedger(ctx, queue.CreateCodingLedgerCommand{
		JobID: jobID, OperationID: turn.OperationID, OwnerTurnID: turn.OwnerTurnID,
		OwnerTurnText: turn.Text, OwnerTurnDisposition: proposal.OwnerTurnDisposition,
		Ledger: proposal.Ledger, CoveragePassed: false,
		Routes: codingLedgerPlanningRoutes(controller.Configuration, 1),
	})
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	analysisTurn := turn
	analysisTurn.ObservedRevision = created.Revision
	coverage, err := controller.AnalyzePlanningCoverage(ctx, analysisTurn, created.Ledger, proposal.OwnerTurnDisposition)
	if err != nil {
		return created, err
	}
	if coverage.Covered {
		return created, nil
	}
	return created, planningCoverageUnready(coverage)
}

// applyCodingLedgerPlanningTransaction reloads the authoritative proposal on
// every attempt. It never depends on a prior provider thread or in-memory ledger.
func applyCodingLedgerPlanningTransaction(
	ctx context.Context,
	repository codingLedgerPlanningRepository,
	controller *codingagent.Controller,
	ledgerID int64,
	turn codingagent.PlanningTurn,
	direct *codingagent.DirectPlanningAction,
) (queue.CodingLedgerRevision, error) {
	current, err := readCompleteCodingLedger(ctx, repository, ledgerID)
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	if current.Revision != turn.ObservedRevision {
		return queue.CodingLedgerRevision{}, fmt.Errorf(
			"planning turn observed revision %d but database revision is %d",
			turn.ObservedRevision, current.Revision,
		)
	}
	var mutation codingagent.LedgerMutation
	if direct != nil {
		mutation, err = codingagent.DirectPlanningMutation(turn, *direct)
	} else {
		mutation, err = controller.PlanPlanningMutation(ctx, current.Ledger, turn)
	}
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	applied, err := repository.ApplyCodingLedgerMutation(ctx, queue.ApplyCodingLedgerMutationCommand{
		LedgerID: ledgerID, OwnerTurnText: turn.Text, Mutation: mutation,
		CoveragePassed: false,
	})
	if err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	if codingagent.RequiresPlanningCoverageForTurn(mutation, direct != nil) {
		analysisTurn := turn
		analysisTurn.ObservedRevision = applied.Revision
		var coverage codingagent.PlanningCoverageJudgment
		coverage, err = controller.AnalyzePlanningCoverage(ctx, analysisTurn, applied.Ledger, mutation.OwnerTurnDisposition)
		if err != nil {
			return applied, err
		}
		if !coverage.Covered {
			return applied, planningCoverageUnready(coverage)
		}
	}
	return applied, nil
}

func planningCoverageUnready(coverage codingagent.PlanningCoverageJudgment) error {
	questions := make([]string, 0, len(coverage.Suggestions)+1)
	questions = append(questions, coverage.Explanation)
	questions = append(questions, coverage.Suggestions...)
	return &codingagent.QuestionsPending{Questions: questions}
}

func readCompleteCodingLedger(
	ctx context.Context,
	repository codingLedgerPlanningRepository,
	ledgerID int64,
) (queue.CodingLedgerRevision, error) {
	const pageSize = 1000
	var complete queue.CodingLedgerRevision
	for offset := 0; ; offset += pageSize {
		page, err := repository.ReadCodingLedger(ctx, ledgerID, queue.CodingLedgerReadOptions{
			Limit: pageSize, Offset: offset,
		})
		if err != nil {
			return queue.CodingLedgerRevision{}, err
		}
		if offset == 0 {
			complete = page
			complete.Ledger.Objectives = nil
			complete.Ledger.Requirements = nil
			complete.Ledger.Constraints = nil
			complete.Ledger.Groups = nil
			complete.Ledger.Tasks = nil
			complete.Ledger.AcceptanceCriteria = nil
			complete.Ledger.Checks = nil
			complete.Ledger.DecisionsAndQuestions = nil
			complete.Ledger.Dependencies = nil
			complete.Ledger.Findings = nil
		} else if page.Revision != complete.Revision {
			return queue.CodingLedgerRevision{}, fmt.Errorf("coding ledger changed while loading planning context")
		}
		count := codingLedgerPageItemCount(page.Ledger)
		appendCodingLedgerPage(&complete.Ledger, page.Ledger)
		if count < pageSize {
			break
		}
	}
	if err := complete.Ledger.ValidateDraft(); err != nil {
		return queue.CodingLedgerRevision{}, err
	}
	return complete, nil
}

func codingLedgerPageItemCount(ledger model.EngineeringLedger) int {
	return len(ledger.Objectives) + len(ledger.Requirements) + len(ledger.Constraints) +
		len(ledger.Groups) + len(ledger.Tasks) + len(ledger.AcceptanceCriteria) +
		len(ledger.Checks) + len(ledger.DecisionsAndQuestions) + len(ledger.Dependencies) +
		len(ledger.Findings)
}

func appendCodingLedgerPage(target *model.EngineeringLedger, page model.EngineeringLedger) {
	target.Objectives = append(target.Objectives, page.Objectives...)
	target.Requirements = append(target.Requirements, page.Requirements...)
	target.Constraints = append(target.Constraints, page.Constraints...)
	target.Groups = append(target.Groups, page.Groups...)
	target.Tasks = append(target.Tasks, page.Tasks...)
	target.AcceptanceCriteria = append(target.AcceptanceCriteria, page.AcceptanceCriteria...)
	target.Checks = append(target.Checks, page.Checks...)
	target.DecisionsAndQuestions = append(target.DecisionsAndQuestions, page.DecisionsAndQuestions...)
	target.Dependencies = append(target.Dependencies, page.Dependencies...)
	target.Findings = append(target.Findings, page.Findings...)
}

func auditCodingLedgerPlanningSignoff(
	ctx context.Context,
	repository codingLedgerPlanningRepository,
	controller *codingagent.Controller,
	ledgerID int64,
	auditID string,
	turn codingagent.PlanningTurn,
) (queue.CodingLedgerRevision, codingagent.PlanningAuditReceipt, error) {
	current, err := readCompleteCodingLedger(ctx, repository, ledgerID)
	if err != nil {
		return queue.CodingLedgerRevision{}, codingagent.PlanningAuditReceipt{}, err
	}
	if current.Revision != turn.ObservedRevision {
		return current, codingagent.PlanningAuditReceipt{}, fmt.Errorf(
			"planning audit observed revision %d but database revision is %d",
			turn.ObservedRevision, current.Revision,
		)
	}
	auditRepository, ok := repository.(codingLedgerPlanningAuditRepository)
	if !ok {
		return current, codingagent.PlanningAuditReceipt{}, fmt.Errorf("planning repository cannot persist mandatory audit authority")
	}
	persistedTurns, err := auditRepository.CodingLedgerPlanningTurns(ctx, ledgerID)
	if err != nil {
		return current, codingagent.PlanningAuditReceipt{}, err
	}
	audit, err := controller.AuditPlanningLedger(ctx, auditID, turn, current.Ledger, persistedTurns)
	if err != nil {
		return current, codingagent.PlanningAuditReceipt{}, err
	}
	if err := auditRepository.RecordCodingLedgerPlanningAudit(ctx, queue.RecordCodingLedgerPlanningAuditCommand{
		LedgerID: ledgerID, AuditID: audit.AuditID, Revision: audit.Revision,
		Ready: audit.Ready, Explanation: audit.Judgment.Explanation,
	}); err != nil {
		return current, audit, err
	}
	if err := codingagent.ValidatePlanningSignoff(current.Ledger, audit); err != nil {
		return current, audit, err
	}
	return current, audit, nil
}
