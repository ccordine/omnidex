package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/queue"
)

// ProductionSemanticReplayReader is the bounded durable-evidence boundary for
// a Full replay. The next adapter rail must consume every referenced page;
// merely implementing this interface never qualifies an artifact.
type ProductionSemanticReplayReader interface {
	sealedTraceReader
	ReadCognitionModelResponseEvidence(
		context.Context, cognition.EpisodeID, string, int, int,
	) (queue.CognitionPolicyEvidencePage, error)
	ReadCognitionProviderGenerationEvidence(
		context.Context, cognition.EpisodeID, string, int, int,
	) (queue.CognitionPolicyEvidencePage, error)
	ReadCognitionProviderResponseCapture(
		context.Context, cognition.EpisodeID, string, int, int,
	) (queue.CognitionPolicyEvidencePage, error)
	ReadCognitionProviderIdentityEvidenceManifest(
		context.Context, cognition.EpisodeID, string,
	) (queue.CognitionProviderIdentityEvidenceManifest, error)
	ReadCognitionProviderIdentityEvidenceBody(
		context.Context, cognition.EpisodeID, string, int,
		queue.CognitionProviderIdentityBodyKind, int, int,
	) (queue.CognitionProviderIdentityEvidenceBodyPage, error)
}

// ProductionSemanticReplaySidecars are the exact raw files named by the
// sealed episode's provider bootstrap and activation trace authorities.
type ProductionSemanticReplaySidecars struct {
	RuntimeBrainBootstrapEvidence     []byte
	RuntimeProviderActivationEvidence []byte
}

func (value ProductionSemanticReplaySidecars) validate() error {
	if len(value.RuntimeBrainBootstrapEvidence) == 0 ||
		len(value.RuntimeBrainBootstrapEvidence) > maxRuntimeBrainBootstrapEvidenceBytes ||
		len(value.RuntimeProviderActivationEvidence) == 0 ||
		len(value.RuntimeProviderActivationEvidence) > maxRuntimeProviderActivationEvidenceBytes {
		return fmt.Errorf("semantic production replay requires both bounded runtime provider sidecars")
	}
	return nil
}

var _ ProductionSemanticReplayReader = (*queue.Repository)(nil)
