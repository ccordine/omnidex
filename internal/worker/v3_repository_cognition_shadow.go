package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/cognitionstore"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/repository/cognitionenv"
)

func (session *directCodingSession) runRepositoryCognitionShadow(
	decision assemblyline.RepositoryRetrievalDecision,
	analysisID string,
) error {
	if session == nil || session.runtime == nil || session.runtime.ctx == nil ||
		session.runtime.svc == nil || session.runtime.svc.repo == nil ||
		session.runtime.svc.llm == nil || session.runtime.claim == nil ||
		session.repositoryIndex == nil || session.runtime.svc.repositoryRetrieval == nil {
		return fmt.Errorf("repository cognition shadow requires one claimed PostgreSQL repository runtime")
	}
	if err := session.requireCurrentRepositoryAuthority("repository cognition shadow"); err != nil {
		return err
	}
	need, err := repositoryCognitionNeedAuthority(decision)
	if err != nil {
		return fmt.Errorf("repository cognition accepted need: %w", err)
	}
	projectID, err := session.runtime.svc.repo.JobProjectID(
		session.runtime.ctx, session.runtime.claim.Job.ID,
	)
	if err != nil {
		return fmt.Errorf("resolve repository cognition project authority: %w", err)
	}
	analysis, err := exactRepositoryChangeAnalysis(session.repositoryIndex.Analyses, analysisID)
	if err != nil {
		return fmt.Errorf("repository cognition analysis authority: %w", err)
	}
	investigation, err := cognitionenv.NewInvestigation(
		projectID, session.repositoryIndex.Snapshot, analysis, need,
		decision.Operation, decision.QueryQuote,
	)
	if err != nil {
		return fmt.Errorf("compile repository cognition investigation: %w", err)
	}
	brain, err := repositoryCognitionBrain(session.runtime)
	if err != nil {
		return err
	}
	budget, cycles, err := repositoryCognitionBudget(brain, investigation.Catalog())
	if err != nil {
		return fmt.Errorf("compile repository cognition budget: %w", err)
	}
	episode, err := repositoryCognitionEpisodeRef(session.runtime.claim.Authority, investigation.Ref())
	if err != nil {
		return err
	}
	facts := cognitionstate.NewNoFactAcceptanceAuthority()
	store, err := cognitionstore.New(session.runtime.svc.repo, facts)
	if err != nil {
		return err
	}
	binding, err := cognitionstore.BindAttempt(episode.ID, session.runtime.claim.Authority)
	if err != nil {
		return err
	}
	if err := store.AuthorizeAttempt(session.runtime.ctx, binding.Attempt); err != nil {
		return fmt.Errorf("authorize repository cognition attempt: %w", err)
	}
	bootstrapOutcome, err := cognitionpolicy.AttestBrain(
		session.runtime.ctx, session.runtime.svc.llm, brain,
	)
	if err != nil {
		if bootstrapOutcome.Failure == nil {
			return fmt.Errorf("attest repository cognition brain without durable failure evidence: %w", err)
		}
		if persistErr := store.RecordBrainBootstrapFailure(
			session.runtime.ctx, session.runtime.claim.Authority,
			binding.Episode, *bootstrapOutcome.Failure,
		); persistErr != nil {
			return errors.Join(err, fmt.Errorf("persist repository cognition Brain failure: %w", persistErr))
		}
		return fmt.Errorf("attest repository cognition brain: %w", err)
	}
	bootstrap, err := bootstrapOutcome.RequireSuccess()
	if err != nil {
		return err
	}
	environment, err := cognitionenv.NewEnvironment(
		investigation, episode, session.runtime.svc.repositoryRetrieval,
		store.AuthorizeAttempt, store,
	)
	if err != nil {
		return err
	}
	start, err := environment.Start(session.runtime.ctx, investigation.Ref())
	if err != nil {
		return fmt.Errorf("start repository cognition environment: %w", err)
	}
	activationOutcome, err := cognitionpolicy.ObserveProviderProcess(
		session.runtime.ctx, session.runtime.svc.llm, bootstrap.AttestedBrain,
		binding.Episode, binding.Attempt, cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		if activationOutcome.Failure == nil {
			return fmt.Errorf("observe repository cognition provider process without durable failure evidence: %w", err)
		}
		if persistErr := store.RecordProviderProcessFailure(
			session.runtime.ctx, bootstrap, *activationOutcome.Failure,
		); persistErr != nil {
			return errors.Join(err, fmt.Errorf("persist repository cognition provider process failure: %w", persistErr))
		}
		return fmt.Errorf("observe repository cognition provider process: %w", err)
	}
	activation, err := activationOutcome.RequireSuccess(bootstrap.AttestedBrain)
	if err != nil {
		return err
	}
	storedEpisode, err := startRepositoryCognitionEpisode(
		session, store, environment, investigation, episode, bootstrap, activation, budget, start,
	)
	if err != nil {
		return err
	}
	activationAuthority, err := activation.Authority()
	if err != nil {
		return fmt.Errorf("bind repository cognition provider process: %w", err)
	}
	policy, err := cognitionpolicy.New(
		session.runtime.svc.llm, storedEpisode.AttestedBrain, activationAuthority, store, store,
	)
	if err != nil {
		return err
	}
	runtime, err := cognitionruntime.New(cognitionruntime.Dependencies{
		Policy: policy, Environment: environment, Snapshots: store, Accepted: store,
		PolicyRecovery: store, Completion: environment, Episodes: store,
		Reconciler: store, Actions: store, TerminalSeal: store,
	})
	if err != nil {
		return err
	}
	run, runErr := runtime.Run(
		session.runtime.ctx, binding, cognitionruntime.RunLimits{MaxCycles: uint32(cycles)},
	)
	if runErr != nil {
		return session.cancelRepositoryCognitionShadow(store, binding, runErr)
	}
	if run.Terminal.State != cognitionruntime.StepEpisodeCompleted || run.Terminal.Seal == nil {
		return fmt.Errorf("repository cognition shadow stopped without exact completed terminal evidence")
	}
	session.runtime.svc.emitStepEvent(
		session.runtime.claim.Authority, "repository_cognition_shadow_completed",
		fmt.Sprintf("episode=%s calls=%d actions=%d", episode.ID, run.PolicyCalls, run.EnvironmentActions),
	)
	return nil
}

func (session *directCodingSession) cancelRepositoryCognitionShadow(
	store *cognitionstore.Store,
	binding cognitionruntime.Binding,
	source error,
) error {
	code, message, allowed := repositoryCognitionCancellation(source)
	if !allowed {
		return source
	}
	evidence, err := cognitionruntime.NewCancellationEvidence(code, message, source)
	if err != nil {
		return err
	}
	episode, err := session.runtime.svc.repo.CognitionEpisode(session.runtime.ctx, binding.Episode.ID)
	if err != nil {
		return fmt.Errorf("load repository cognition cancellation authority: %w", err)
	}
	if _, err := store.Cancel(session.runtime.ctx, cognitionruntime.CancellationCommand{
		Binding: binding, ExpectedRevision: episode.CurrentRevision,
		Code: code, SourceEvidence: evidence,
	}); err != nil {
		return fmt.Errorf("cancel repository cognition shadow: %w", err)
	}
	session.runtime.svc.emitStepEvent(
		session.runtime.claim.Authority, "repository_cognition_shadow_canceled",
		fmt.Sprintf("episode=%s code=%s evidence=%s", binding.Episode.ID, code, evidence.ID),
	)
	return nil
}

func repositoryCognitionCancellation(source error) (
	cognitionruntime.CancellationCode,
	string,
	bool,
) {
	if source == nil || repositoryCognitionRequiresLoudFailure(source) {
		return "", "", false
	}
	if errors.Is(source, cognitionpolicy.ErrInvalidDecision) ||
		errors.Is(source, cognitionpolicy.ErrResponseLimit) ||
		errors.Is(source, cognitionpolicy.ErrGeneration) {
		return cognitionruntime.CancellationPolicyFailure,
			"repository cognition shadow stopped after a registered policy outcome", true
	}
	if errors.Is(source, cognition.ErrCoordinatorBudgetExhausted) ||
		errors.Is(source, cognitionruntime.ErrRunCycleLimit) {
		return cognitionruntime.CancellationRunBudgetExhausted,
			"repository cognition shadow exhausted its registered run budget", true
	}
	return "", "", false
}

func repositoryCognitionRequiresLoudFailure(source error) bool {
	for _, sentinel := range []error{
		cognitionpolicy.ErrEnvelopeLimit,
		cognitionpolicy.ErrCallJournal,
		cognitionpolicy.ErrCallIndeterminate,
		cognitionpolicy.ErrCallRejected,
		cognitionpolicy.ErrInputLimit,
		cognitionpolicy.ErrInvalidConfig,
		cognitionpolicy.ErrInvalidEvidence,
		cognitionpolicy.ErrInvalidBrain,
		cognitionpolicy.ErrInvalidProjection,
		cognitionpolicy.ErrProjectionMismatch,
		cognitionpolicy.ErrProviderIdentity,
		cognitionruntime.ErrInvalidConfiguration,
		cognitionruntime.ErrInvalidBinding,
		cognitionruntime.ErrInvalidPreparedState,
		cognitionruntime.ErrInvalidJournalState,
		cognitionruntime.ErrInvalidProgress,
		cognitionruntime.ErrInvalidSeal,
		cognitionruntime.ErrEnvironment,
		cognition.ErrAuthorityDenied,
		cognition.ErrEnvironmentJournalConflict,
		cognition.ErrEnvironmentJournalNotStarted,
		cognition.ErrEnvironmentJournalStaleRevision,
		cognition.ErrEnvironmentJournalTerminal,
		queue.ErrCognitionConflict,
		queue.ErrCognitionTerminal,
		queue.ErrStaleStepAttempt,
	} {
		if errors.Is(source, sentinel) {
			return true
		}
	}
	return false
}
