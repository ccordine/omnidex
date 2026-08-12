package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/queue"
)

// VerifiedProductionSemanticReplay can be constructed only by the product
// verifier after it rederives the semantic projection from embedded authority.
type VerifiedProductionSemanticReplay struct {
	verified        cognitionreplay.VerifiedBase
	preregistration matrixReplayPreregistration
}

func (verified VerifiedProductionSemanticReplay) SHA256() string {
	return verified.verified.SHA256()
}

func (verified VerifiedProductionSemanticReplay) RequireSeriousExecution() error {
	if verified.verified.SHA256() == "" || verified.preregistration.validate() != nil ||
		verified.preregistration.variant != VariantFullCognition {
		return fmt.Errorf("production semantic replay is unverified")
	}
	return nil
}

func VerifyProductionSemanticReplayFor(
	raw []byte,
	preregistered matrixReplayPreregistration,
) (VerifiedProductionSemanticReplay, error) {
	if err := preregistered.validate(); err != nil {
		return VerifiedProductionSemanticReplay{}, err
	}
	if preregistered.variant != VariantFullCognition {
		return VerifiedProductionSemanticReplay{}, fmt.Errorf(
			"production semantic replay requires a preregistered full-cognition coordinate",
		)
	}
	verified, err := cognitionreplay.VerifyBase(raw)
	if err != nil {
		return VerifiedProductionSemanticReplay{}, err
	}
	projection, err := verifyProductionSemanticProjection(verified)
	if err != nil {
		return VerifiedProductionSemanticReplay{}, err
	}
	if !reflect.DeepEqual(projection.bundle.Authority, preregistered.authority) ||
		preregistered.binds(
			projection.bundle.Authority, projection.episode.Manifest.EpisodeID,
			projection.episode.Manifest.EpisodeStartedAt,
		) != nil || preregistered.bindsExecution(projection.episode.Manifest) != nil ||
		projection.trace.Header.EpisodeStartedAt.Before(preregistered.registeredAt) {
		return VerifiedProductionSemanticReplay{}, fmt.Errorf(
			"semantic replay differs from preregistered production authority",
		)
	}
	return VerifiedProductionSemanticReplay{
		verified: projection.verified, preregistration: preregistered,
	}, nil
}

type productionSemanticProjectionVerification struct {
	verified cognitionreplay.VerifiedBase
	bundle   PublicInferenceBundle
	episode  SealedEpisode
	trace    productionTrace
}

func verifyProductionSemanticProjection(
	verified cognitionreplay.VerifiedBase,
) (productionSemanticProjectionVerification, error) {
	manifest := verified.Manifest()
	authority := manifest.ProjectionAuthority
	if manifest.SemanticStatus != cognitionreplay.SemanticProjection || authority == nil {
		return productionSemanticProjectionVerification{}, fmt.Errorf("replay is not a production semantic projection")
	}
	var bundle PublicInferenceBundle
	if err := decodeSemanticAuthorityBlob(
		verified, authority.PublicBundle, &bundle, "embedded public inference bundle",
	); err != nil {
		return productionSemanticProjectionVerification{}, err
	}
	var episode SealedEpisode
	if err := decodeSemanticAuthorityBlob(
		verified, authority.SealedEpisode, &episode, "embedded sealed episode",
	); err != nil {
		return productionSemanticProjectionVerification{}, err
	}
	var header semanticReplayTraceHeader
	if err := decodeSemanticAuthorityBlob(
		verified, authority.ProductionTrace, &header, "embedded production trace header",
	); err != nil {
		return productionSemanticProjectionVerification{}, err
	}
	records, err := semanticReplayRecordsFromVerified(verified)
	if err != nil {
		return productionSemanticProjectionVerification{}, err
	}
	page, err := header.page(records)
	if err != nil {
		return productionSemanticProjectionVerification{}, err
	}
	trace := productionTrace{Header: page, Records: records}
	if err := validateSemanticReplayPublicAuthority(bundle, episode); err != nil {
		return productionSemanticProjectionVerification{}, err
	}
	if err := validateSemanticReplayTraceAuthority(bundle, episode, trace); err != nil {
		return productionSemanticProjectionVerification{}, err
	}
	supplement, err := verifyEmbeddedSemanticReplayEvidence(
		verified, bundle, trace, authority.Sidecars,
	)
	if err != nil {
		return productionSemanticProjectionVerification{}, err
	}
	expected, err := buildProductionSemanticReplay(bundle, episode, trace, supplement)
	if err != nil {
		return productionSemanticProjectionVerification{}, err
	}
	expectedArtifact, err := cognitionreplay.ExportSemanticProjection(expected)
	if err != nil {
		return productionSemanticProjectionVerification{}, fmt.Errorf(
			"export exact semantic replay rederivation: %w", err,
		)
	}
	if expectedArtifact.SHA256 != verified.SHA256() {
		return productionSemanticProjectionVerification{}, fmt.Errorf("semantic replay projection differs from exact typed rederivation")
	}
	return productionSemanticProjectionVerification{
		verified: verified, bundle: bundle, episode: episode, trace: trace,
	}, nil
}

func decodeSemanticAuthorityBlob(
	verified cognitionreplay.VerifiedBase,
	ref cognitionreplay.ProjectionContentAuthority,
	target any,
	label string,
) error {
	raw, err := verified.ProjectionContent(ref)
	if err != nil {
		return fmt.Errorf("%s is missing or changed: %w", label, err)
	}
	if err := decodeStrictJSON(bytes.TrimSuffix(raw, []byte{'\n'}), target, label); err != nil {
		return err
	}
	want, err := json.Marshal(target)
	if err != nil || !bytes.Equal(raw, append(want, '\n')) {
		return fmt.Errorf("%s is not exact canonical replay JSON", label)
	}
	return nil
}

func semanticReplayRecordsFromVerified(
	verified cognitionreplay.VerifiedBase,
) ([]queue.CognitionSealedTraceRecord, error) {
	sources := verified.Sources()
	records := make([]queue.CognitionSealedTraceRecord, len(sources))
	for index, source := range sources {
		raw, exists := verified.Blob(source.Payload)
		if !exists || source.Payload.MediaType != "application/json" {
			return nil, fmt.Errorf("semantic source %d exact JSON payload is missing", index+1)
		}
		records[index] = queue.CognitionSealedTraceRecord{
			Kind: source.Kind, CallOrdinal: source.CallOrdinal, Phase: source.Phase,
			Sequence: source.Sequence, ID: source.ID, SHA256: source.Payload.SHA256,
			Payload: append(json.RawMessage(nil), raw...),
		}
	}
	return records, nil
}
