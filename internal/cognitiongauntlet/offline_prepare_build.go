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
)

type preparedOfflineExperiment struct {
	mode      OfflineExperimentMode
	promotion OfflinePromotionConfig
	takeover  OfflineTakeoverConfig
}

func prepareOfflineExperiment(
	request OfflineExperimentRequest,
	provider llm.ProviderIdentityAttestation,
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
	brain, err := request.Brain.build(request.Budget, provider, host)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	fixed := FixedExperiment{
		Brain: brain, ContextCeilingBytes: request.Budget.ContextBytes,
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
	spec, err := configuredMicrogauntletSpec(request)
	if err != nil {
		return preparedOfflineExperiment{}, err
	}
	promotion := OfflinePromotionConfig{
		Schema: OfflinePromotionConfigSchemaV1, DatabaseURL: request.DatabaseURL,
		OllamaEndpoint:          request.OllamaEndpoint,
		InferenceTimeoutSeconds: request.InferenceTimeoutSeconds, Spec: spec,
		Variant: request.Variant, Surface: request.Surface,
		RatGeneration: generation, RuntimeFingerprint: fingerprint,
		Repetition: request.Repetition, PublicOutputDirectory: request.PublicOutputDirectory,
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
			Schema: OfflineTakeoverConfigSchemaV1, Promotion: promotion,
			AfterSuccessfulActions: *request.AfterSuccessfulActions,
		}
		if err := prepared.takeover.Validate(); err != nil {
			return preparedOfflineExperiment{}, err
		}
	}
	return prepared, nil
}

func configuredMicrogauntletSpec(request OfflineExperimentRequest) (MicrogauntletSpec, error) {
	spec, err := initialMicrogauntletSpec(request.Suite)
	if err != nil {
		return MicrogauntletSpec{}, err
	}
	spec.CaseID = "configured-" + string(request.Suite) + "-v1"
	spec.Generator.Seed = request.Seed
	spec.Budget = request.Budget
	if err := spec.Validate(); err != nil {
		return MicrogauntletSpec{}, err
	}
	return spec, nil
}

func initialMicrogauntletSpec(suite Suite) (MicrogauntletSpec, error) {
	for _, spec := range InitialMicrogauntletsV1() {
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
	if _, err := releaseMigrationBundle(executable, buildversion.MigrationsSHA256); err != nil {
		return preparedOfflineExperiment{}, err
	}
	timeout := time.Duration(request.InferenceTimeoutSeconds) * time.Second
	client := ollama.New(
		request.OllamaEndpoint, request.Brain.Model, "", timeout,
		request.Brain.NativeContextLimit,
	)
	provider, err := llm.RequireDiscoveredProviderIdentity(
		ctx, client, llm.ProviderIdentitySelection{
			Model:              request.Brain.Model,
			NativeContextLimit: request.Brain.NativeContextLimit,
		},
	)
	if err != nil {
		return preparedOfflineExperiment{}, fmt.Errorf("discover live cognition brain: %w", err)
	}
	host, err := cognitionpolicy.AttestLocalHostHardware()
	if err != nil {
		return preparedOfflineExperiment{}, fmt.Errorf("attest cognition host: %w", err)
	}
	return prepareOfflineExperiment(
		request, provider, host, executable, buildversion.Commit,
		buildversion.SourceSHA256, buildversion.MigrationsSHA256, buildversion.Version,
	)
}
