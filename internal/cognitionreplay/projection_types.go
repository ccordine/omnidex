package cognitionreplay

// SemanticProjectionInput carries a deterministic but unqualified semantic
// projection. Generic replay verification never promotes it to serious
// execution evidence; a production-specific verifier must rederive it.
type SemanticProjectionInput struct {
	TerminalAuthority        TerminalAuthority
	PublicWorldSHA256        string
	PublicWorldSchema        string
	PublicAuthoritySHA256    string
	PublicBundleAuthority    ProjectionContentAuthority
	SealedEpisodeAuthority   ProjectionContentAuthority
	ProductionTraceAuthority ProjectionContentAuthority
	Sidecars                 []ProjectionSidecarAuthority
	Sources                  []SourceRecord
	Events                   []Event
	Checkpoints              []KnowledgeCheckpoint
	ChunkedBlobs             []ChunkedBlobBinding
	Blobs                    []Blob
}

func (input SemanticProjectionInput) baseInput() BaseInput {
	return BaseInput{
		TerminalAuthority:     input.TerminalAuthority,
		PublicWorldSHA256:     input.PublicWorldSHA256,
		PublicWorldSchema:     input.PublicWorldSchema,
		PublicAuthoritySHA256: input.PublicAuthoritySHA256,
		Sources:               input.Sources,
		Events:                input.Events,
		Checkpoints:           input.Checkpoints,
		ChunkedBlobs:          input.ChunkedBlobs,
		Blobs:                 input.Blobs,
	}
}

type ProjectionSidecarAuthority struct {
	Kind    string                     `json:"kind"`
	ID      string                     `json:"id"`
	Content ProjectionContentAuthority `json:"content"`
}

type ProjectionAuthority struct {
	Schema          string                       `json:"schema"`
	PublicBundle    ProjectionContentAuthority   `json:"public_bundle"`
	SealedEpisode   ProjectionContentAuthority   `json:"sealed_episode"`
	ProductionTrace ProjectionContentAuthority   `json:"production_trace"`
	Sidecars        []ProjectionSidecarAuthority `json:"sidecars"`
}

const ProjectionAuthoritySchemaV2 = "omnidex.replay-production-authority.v2"
