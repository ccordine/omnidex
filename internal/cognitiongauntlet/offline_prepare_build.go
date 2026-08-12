package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/taskstate"
	buildversion "github.com/gryph/omnidex/internal/version"
	"github.com/gryph/omnidex/internal/workingset"
)

const (
	offlineEnvironmentContractVersionV1 = "omnidex.cognition-environment-contract.v1"
	offlineEvaluatorVersionV1           = "omnidex.symbolic-state-evaluator.v1"
	offlineAuthorityPolicyVersionV1     = "omnidex.cognition-authority-policy.v1"
	offlineOracleIsolationVersionV1     = "omnidex.separate-process-oracle-isolation.v1"
	offlineProviderDiscoveryScopeV1     = "offline-cognition-provider-discovery.v1"
)

type preparedOfflineExperiment struct {
	mode      OfflineExperimentMode
	promotion OfflinePromotionConfig
	takeover  OfflineTakeoverConfig
}

func prepareOfflineExperiment(
	request OfflineExperimentRequest,
	discovery llm.ObservedProviderIdentity,
	provider llm.ObservedProviderIdentity,
	host cognitionpolicy.HostHardwareAttestation,
	executable string,
	embeddedCommit string,
	embeddedSourceSHA256 string,
	embeddedMigrationsSHA256 string,
	runtimeVersion string,
) (preparedOfflineExperiment, error) {
	if err := request.Validate(); err != nil {
		return preparedOfflineExperiment{}, err
	}
	if !validCommitIdentity(embeddedCommit) || !validDigest(embeddedSourceSHA256) ||
		!validDigest(embeddedMigrationsSHA256) {
		return preparedOfflineExperiment{}, fmt.Errorf("offline prepare requires exact embedded release metadata")
	}
	if err := requireExact(runtimeVersion, "embedded runtime version", 256); err != nil {
		return preparedOfflineExperiment{}, err
	}
	executableSHA, err := executableSHA256(executable)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	sampling, err := cognitionpolicy.NewSamplingIdentity(
		request.Brain.NativeContextLimit, request.Budget.ContextBytes,
		request.Budget.Station.MaxOutputTokens,
	)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	executionBudget, err := NewExecutableRunBudgetV2(request.Budget, sampling)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	brain, err := request.Brain.build(executionBudget, provider, host)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	evidenceArtifact, err := newPreparedBrainEvidenceArtifact(discovery, provider, brain)
	if err != nil {
		return preparedOfflineExperiment{}, fmt.Errorf("bind prepared Brain raw evidence: %w", err)
	}
	fixed := FixedExperiment{
		Brain: brain, ContextCeilingBytes: executionBudget.ContextBytes,
		EnvironmentContractVersion: offlineEnvironmentContractVersionV1,
		EvaluatorVersion:           offlineEvaluatorVersionV1,
		AuthorityPolicyVersion:     offlineAuthorityPolicyVersionV1,
		OracleIsolationVersion:     offlineOracleIsolationVersionV1,
	}
	runtime := RuntimeCandidate{
		Version: runtimeVersion, SourceSHA256: embeddedSourceSHA256,
		ExecutableSHA256: executableSHA, MigrationsSHA256: embeddedMigrationsSHA256,
	}
	generationID, err := digestJSON(struct {
		Fixed   FixedExperiment  `json:"fixed"`
		Runtime RuntimeCandidate `json:"runtime"`
	}{fixed, runtime})
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	generation, err := NewRatGeneration("rat-generation-"+generationID, fixed, runtime)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	fingerprint, err := currentRuntimeFingerprint(embeddedSourceSHA256)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	scenario, err := ResolveOfflineScenarioSpecV1(
		request.Suite, request.Seed, executionBudget,
	)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	evidenceAuthority, err := sealPreparedBrainEvidenceArtifact(
		request.PublicOutputDirectory, evidenceArtifact, brain,
	)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	promotion := OfflinePromotionConfig{
		Schema: OfflinePromotionConfigSchemaV2, DatabaseURL: request.DatabaseURL,
		OllamaEndpoint:          request.OllamaEndpoint,
		InferenceTimeoutSeconds: request.InferenceTimeoutSeconds, Scenario: scenario,
		Variant: request.Variant, Surface: request.Surface,
		RatGeneration: generation, PreparedBrainEvidence: evidenceAuthority,
		RuntimeFingerprint: fingerprint,
		Repetition:         request.Repetition, PublicOutputDirectory: request.PublicOutputDirectory,
		PrivateOutputDirectory: request.PrivateOutputDirectory, OmnidexCommit: embeddedCommit,
		LedgerSchemaVersion:     taskstate.LedgerSchemaV1,
		WorkingSetPolicyVersion: workingset.WorkingSetSchemaV1,
		ProjectionPolicyVersion: contextbuilder.ProjectionSchemaV1,
	}
	if err := promotion.Validate(); err != nil {
		return preparedOfflineExperiment{}, err
	}
	if _, err := validateOfflinePromotionIdentity(
		promotion, executable, embeddedCommit, embeddedSourceSHA256,
		embeddedMigrationsSHA256, runtimeVersion,
	); err != nil {
		return preparedOfflineExperiment{}, err
	}
	prepared := preparedOfflineExperiment{mode: request.Mode, promotion: promotion}
	if request.Mode == OfflineExperimentTakeover {
		prepared.takeover = OfflineTakeoverConfig{
			Schema: OfflineTakeoverConfigSchemaV2, Promotion: promotion,
			AfterSuccessfulActions: *request.AfterSuccessfulActions,
		}
		if err := prepared.takeover.Validate(); err != nil {
			return preparedOfflineExperiment{}, err
		}
	}
	return prepared, nil
}

func initialMicrogauntletSpec(suite Suite) (MicrogauntletSpec, error) {
	for _, spec := range InitialMicrogauntletsV2() {
		candidate, err := gauntletSuite(spec.Generator.Suite)
		if err != nil {
			return MicrogauntletSpec{}, err
		}
		if candidate == suite {
			return spec, nil
		}
	}
	return MicrogauntletSpec{}, fmt.Errorf("offline cognition suite %q is not an initial microgauntlet", suite)
}

func prepareCurrentOfflineExperiment(
	ctx context.Context,
	request OfflineExperimentRequest,
	executable string,
) (preparedOfflineExperiment, error) {
	if ctx == nil {
		return preparedOfflineExperiment{}, fmt.Errorf("offline prepare context is nil")
	}
	if err := request.Validate(); err != nil {
		return preparedOfflineExperiment{}, err
	}
	if _, err := loadReleaseMigrationBundle(executable, buildversion.MigrationsSHA256); err != nil {
		return preparedOfflineExperiment{}, err
	}
	timeout := time.Duration(request.InferenceTimeoutSeconds) * time.Second
	client := ollama.New(
		request.OllamaEndpoint, request.Brain.Model, "", timeout,
		request.Brain.NativeContextLimit,
	)
	selection := llm.ProviderIdentitySelection{
		Model:              request.Brain.Model,
		NativeContextLimit: request.Brain.NativeContextLimit,
	}
	discovered, err := llm.RequireDiscoveredProviderIdentityEvidence(
		ctx, client, selection, offlineProviderDiscoveryScopeV1,
	)
	if err != nil {
		return preparedOfflineExperiment{}, fmt.Errorf("discover live cognition brain: %w", err)
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(
		discovered.Evidence, selection,
	)
	if err != nil {
		return preparedOfflineExperiment{}, fmt.Errorf(
			"derive live cognition brain from raw evidence: %w", err,
		)
	}
	if discovered.Attestation.ValidateFor(expected) != nil ||
		discovered.Observation.ValidateEvidence(discovered.Evidence) != nil {
		return preparedOfflineExperiment{}, fmt.Errorf(
			"discovered cognition brain differs from its raw evidence",
		)
	}
	if err := llm.ValidateExactPreparedProvider(client, expected); err != nil {
		return preparedOfflineExperiment{}, fmt.Errorf(
			"validate live cognition provider contract: %w", err,
		)
	}
	sampling, err := cognitionpolicy.NewSamplingIdentity(
		request.Brain.NativeContextLimit, request.Budget.ContextBytes,
		request.Budget.Station.MaxOutputTokens,
	)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	samplingSHA256, err := sampling.SHA256()
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	challenge, err := llm.DeriveProviderIdentityObservationChallenge(
		"cognition-brain-bootstrap:"+samplingSHA256, expected,
	)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	provider, err := llm.RequireProviderIdentityObservation(
		ctx, client, llm.ProviderIdentityObservationRequest{
			Expectation: expected, ChallengeSHA256: challenge,
		},
	)
	if err != nil {
		return preparedOfflineExperiment{}, fmt.Errorf("observe live cognition brain: %w", err)
	}
	host, err := cognitionpolicy.AttestLocalHostHardware()
	if err != nil {
		return preparedOfflineExperiment{}, fmt.Errorf("attest cognition host: %w", err)
	}
	return prepareOfflineExperiment(
		request, discovered, provider, host, executable, buildversion.Commit,
		buildversion.SourceSHA256, buildversion.MigrationsSHA256, buildversion.Version,
	)
}
