package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	OfflineExperimentRequestSchemaV1 = "omnidex.offline-cognition-experiment-request.v1"
)

type OfflineExperimentMode string

const (
	OfflineExperimentRun      OfflineExperimentMode = "run"
	OfflineExperimentTakeover OfflineExperimentMode = "takeover"
)

type OfflineBrainRequest struct {
	Model              string `json:"model"`
	NativeContextLimit int    `json:"native_context_limit"`
}

type OfflineExperimentRequest struct {
	Schema                  string                `json:"schema"`
	Mode                    OfflineExperimentMode `json:"mode"`
	Variant                 Variant               `json:"variant"`
	Suite                   Suite                 `json:"suite"`
	Seed                    uint64                `json:"seed"`
	Surface                 Surface               `json:"surface"`
	Budget                  RunBudget             `json:"budget"`
	DatabaseURL             string                `json:"database_url"`
	OllamaEndpoint          string                `json:"ollama_endpoint"`
	InferenceTimeoutSeconds int                   `json:"inference_timeout_seconds"`
	Repetition              int                   `json:"repetition"`
	PublicOutputDirectory   string                `json:"public_output_directory"`
	PrivateOutputDirectory  string                `json:"private_output_directory"`
	Brain                   OfflineBrainRequest   `json:"brain"`
	AfterSuccessfulActions  *uint32               `json:"after_successful_actions,omitempty"`
}

func (request OfflineExperimentRequest) Validate() error {
	if request.Schema != OfflineExperimentRequestSchemaV1 ||
		(request.Mode != OfflineExperimentRun && request.Mode != OfflineExperimentTakeover) ||
		request.Seed == 0 {
		return fmt.Errorf("offline cognition experiment request authority is invalid")
	}
	if request.Budget.Schema != RunBudgetSchemaStructuralV1 {
		return fmt.Errorf("offline cognition request requires the structural v1 budget authority")
	}
	if request.Variant != VariantFullCognition && !executableAblation(request.Variant) {
		return fmt.Errorf("offline cognition experiment variant %q is not executable", request.Variant)
	}
	if request.Variant == VariantRawShell && request.Surface != SurfaceFilesystem {
		return fmt.Errorf("offline raw-shell experiment requires the filesystem surface")
	}
	if request.Mode == OfflineExperimentTakeover && request.Variant != VariantFullCognition {
		return fmt.Errorf("offline takeover currently requires full cognition")
	}
	if _, err := ResolveOfflineScenarioSpecV1(request.Suite, request.Seed, request.Budget); err != nil {
		return err
	}
	if _, err := request.Surface.Version(); err != nil {
		return err
	}
	if err := request.Budget.Validate(); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"model": request.Brain.Model, "database URL": request.DatabaseURL,
		"Ollama endpoint":          request.OllamaEndpoint,
		"public output directory":  request.PublicOutputDirectory,
		"private output directory": request.PrivateOutputDirectory,
	} {
		if err := requireExact(value, "offline experiment "+label, 4096); err != nil {
			return err
		}
	}
	if request.Brain.NativeContextLimit <= 0 || request.InferenceTimeoutSeconds <= 0 ||
		request.InferenceTimeoutSeconds > 24*60*60 || request.Repetition <= 0 ||
		request.Repetition > 10_000 {
		return fmt.Errorf("offline cognition brain, timeout, or repetition is invalid")
	}
	if request.Mode == OfflineExperimentRun && request.AfterSuccessfulActions != nil {
		return fmt.Errorf("offline run request cannot carry takeover authority")
	}
	if request.Mode == OfflineExperimentTakeover && request.AfterSuccessfulActions == nil {
		return fmt.Errorf("offline takeover request requires an exact action boundary")
	}
	return nil
}

func (brain OfflineBrainRequest) build(
	budget RunBudget,
	provider llm.ObservedProviderIdentity,
	host cognitionpolicy.HostHardwareAttestation,
) (BrainFingerprint, error) {
	if err := provider.Attestation.Validate(); err != nil ||
		provider.Attestation.Model != brain.Model ||
		provider.Attestation.NativeContextLimit != brain.NativeContextLimit {
		return BrainFingerprint{}, fmt.Errorf("live provider identity changed the selected brain")
	}
	if err := host.Validate(); err != nil {
		return BrainFingerprint{}, err
	}
	sampling, err := cognitionpolicy.NewSamplingIdentity(
		brain.NativeContextLimit, budget.ContextBytes, budget.Station.MaxOutputTokens,
	)
	if err != nil {
		return BrainFingerprint{}, err
	}
	ref, err := cognitionpolicy.NewBrainRef(
		provider.Attestation.Model, provider.Attestation.Digest,
		provider.Attestation.Quantization, provider.Attestation.Backend,
		provider.Attestation.BackendVersion,
		"host-attestation:"+host.AttestationSHA256, sampling,
	)
	if err != nil {
		return BrainFingerprint{}, err
	}
	bootstrap, err := cognitionpolicy.BootstrapProviderIdentityRequest(ref)
	if err != nil {
		return BrainFingerprint{}, err
	}
	if err := provider.ValidateFor(bootstrap); err != nil {
		return BrainFingerprint{}, fmt.Errorf("live provider identity changed the bootstrap challenge")
	}
	attested, err := cognitionpolicy.NewAttestedBrain(
		ref, provider.Attestation, provider.Observation, host,
	)
	if err != nil {
		return BrainFingerprint{}, err
	}
	return brainFingerprintFromAttested(attested)
}
