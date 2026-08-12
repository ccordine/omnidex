package cognitionreplay

const (
	AblationProjectionAuthoritySchemaV1 = "omnidex.replay-ablation-authority.v1"
	AblationSourceRegistryIDV1          = "omnidex.replay.ablation-source-registry.v1"
)

// AblationProjectionClass preserves the product evidence classification. It
// does not qualify a generic replay for serious execution.
type AblationProjectionClass string

const (
	AblationProjectionSerious       AblationProjectionClass = "serious"
	AblationProjectionBenchmarkOnly AblationProjectionClass = "benchmark_only"
	AblationProjectionContaminated  AblationProjectionClass = "contaminated"
)

type AblationSemanticProjectionInput struct {
	TerminalAuthority           TerminalAuthority
	PublicWorldSHA256           string
	PublicWorldSchema           string
	PublicAuthoritySHA256       string
	ClaimedClass                AblationProjectionClass
	PrivateOverlayRequired      bool
	PublicBundleAuthority       ProjectionContentAuthority
	SealedEpisodeAuthority      ProjectionContentAuthority
	AblationEvidenceAuthority   ProjectionContentAuthority
	BrainBootstrapAuthority     ProjectionContentAuthority
	ProviderActivationAuthority ProjectionContentAuthority
	Sidecars                    []ProjectionSidecarAuthority
	Sources                     []SourceRecord
	Events                      []Event
	Checkpoints                 []KnowledgeCheckpoint
	ChunkedBlobs                []ChunkedBlobBinding
	Blobs                       []Blob
}

func (input AblationSemanticProjectionInput) baseInput() BaseInput {
	return BaseInput{
		TerminalAuthority: input.TerminalAuthority,
		PublicWorldSHA256: input.PublicWorldSHA256, PublicWorldSchema: input.PublicWorldSchema,
		PublicAuthoritySHA256: input.PublicAuthoritySHA256,
		Sources:               input.Sources, Events: input.Events, Checkpoints: input.Checkpoints,
		ChunkedBlobs: input.ChunkedBlobs, Blobs: input.Blobs,
	}
}

type AblationProjectionAuthority struct {
	Schema                 string                       `json:"schema"`
	RegistryID             string                       `json:"registry_id"`
	RegistrySHA256         string                       `json:"registry_sha256"`
	ClaimedClass           AblationProjectionClass      `json:"claimed_class"`
	PrivateOverlayRequired bool                         `json:"private_overlay_required"`
	PublicBundle           ProjectionContentAuthority   `json:"public_bundle"`
	SealedEpisode          ProjectionContentAuthority   `json:"sealed_episode"`
	AblationEvidence       ProjectionContentAuthority   `json:"ablation_evidence"`
	BrainBootstrap         ProjectionContentAuthority   `json:"brain_bootstrap"`
	ProviderActivation     ProjectionContentAuthority   `json:"provider_activation"`
	Sidecars               []ProjectionSidecarAuthority `json:"sidecars"`
}

func ablationProjectionContentValues(
	value *AblationProjectionAuthority,
) []ProjectionContentAuthority {
	if value == nil {
		return nil
	}
	result := []ProjectionContentAuthority{
		value.PublicBundle, value.SealedEpisode, value.AblationEvidence,
		value.BrainBootstrap, value.ProviderActivation,
	}
	for _, sidecar := range value.Sidecars {
		result = append(result, sidecar.Content)
	}
	return result
}

func (class AblationProjectionClass) publicContentRole() (ChunkedBlobRole, bool) {
	switch class {
	case AblationProjectionSerious, AblationProjectionBenchmarkOnly,
		AblationProjectionContaminated:
		return ChunkedBlobPublicAgentKnowledge, true
	default:
		return "", false
	}
}

func cloneAblationProjectionAuthority(
	value *AblationProjectionAuthority,
) *AblationProjectionAuthority {
	if value == nil {
		return nil
	}
	copy := *value
	copy.PublicBundle = cloneProjectionContent(copy.PublicBundle)
	copy.SealedEpisode = cloneProjectionContent(copy.SealedEpisode)
	copy.AblationEvidence = cloneProjectionContent(copy.AblationEvidence)
	copy.BrainBootstrap = cloneProjectionContent(copy.BrainBootstrap)
	copy.ProviderActivation = cloneProjectionContent(copy.ProviderActivation)
	copy.Sidecars = cloneProjectionSidecars(copy.Sidecars)
	return &copy
}
