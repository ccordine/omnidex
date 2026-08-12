package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/labyrinth"
)

// ExportProductionSemanticReplay is the sole serious Labyrinth replay adapter.
// It accepts the bounded queue pager, the exact public pre-evaluation bundle,
// and the separately sealed public episode. It never reads recorder logs or
// evaluator-private artifacts.
func ExportProductionSemanticReplay(
	ctx context.Context,
	reader ProductionSemanticReplayReader,
	bundle PublicInferenceBundle,
	episode SealedEpisode,
	sidecars ProductionSemanticReplaySidecars,
) (cognitionreplay.Artifact, error) {
	if ctx == nil || reader == nil {
		return cognitionreplay.Artifact{}, fmt.Errorf(
			"semantic replay requires context and the production sealed-trace pager",
		)
	}
	if err := validateSemanticReplayPublicAuthority(bundle, episode); err != nil {
		return cognitionreplay.Artifact{}, err
	}
	if err := sidecars.validate(); err != nil {
		return cognitionreplay.Artifact{}, err
	}
	trace, err := readProductionTrace(ctx, reader, episode.Manifest.EpisodeID)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	if err := validateSemanticReplayTraceAuthority(bundle, episode, trace); err != nil {
		return cognitionreplay.Artifact{}, err
	}
	supplement, err := readProductionSemanticReplayEvidence(
		ctx, reader, bundle, trace, sidecars,
	)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	projection, err := buildProductionSemanticReplay(bundle, episode, trace, supplement)
	if err != nil {
		return cognitionreplay.Artifact{}, err
	}
	artifact, err := cognitionreplay.ExportSemanticProjection(projection)
	if err != nil {
		return cognitionreplay.Artifact{}, fmt.Errorf(
			"export production semantic replay: %w", err,
		)
	}
	return artifact, nil
}

func validateSemanticReplayPublicAuthority(
	bundle PublicInferenceBundle,
	episode SealedEpisode,
) error {
	if err := bundle.Validate(); err != nil {
		return fmt.Errorf("semantic replay public bundle: %w", err)
	}
	if err := episode.Validate(); err != nil {
		return fmt.Errorf("semantic replay public episode: %w", err)
	}
	if err := validateProductionSemanticReplayTimes(
		episode.Manifest.EpisodeStartedAt,
		episode.Manifest.SealedAt,
		episode.Manifest.SealedAt,
	); err != nil {
		return fmt.Errorf("semantic replay public episode: %w", err)
	}
	if bundle.Authority.Variant != VariantFullCognition ||
		episode.Manifest.Variant != VariantFullCognition {
		return fmt.Errorf("semantic replay adapter supports only the full-cognition production variant")
	}
	authoritySHA, err := bundle.Authority.SHA256()
	if err != nil {
		return err
	}
	manifest := episode.Manifest
	if manifest.PublicRunAuthoritySHA256 != authoritySHA ||
		manifest.Scenario != bundle.Authority.Scenario ||
		manifest.RatGeneration != bundle.Authority.RatGeneration ||
		manifest.StationBudget != bundle.Authority.Budget.Station {
		return fmt.Errorf("semantic replay episode differs from its exact public bundle")
	}
	return nil
}

func validateSemanticReplayTraceAuthority(
	bundle PublicInferenceBundle,
	episode SealedEpisode,
	trace productionTrace,
) error {
	if err := validateSemanticProductionTraceDigest(trace); err != nil {
		return err
	}
	header, manifest := trace.Header, episode.Manifest
	if header.EpisodeID != manifest.EpisodeID ||
		header.Seal.FinalRevision != manifest.FinalRevision ||
		!header.EpisodeStartedAt.Equal(manifest.EpisodeStartedAt) ||
		!header.SealedAt.Equal(manifest.SealedAt) ||
		!header.SealedAt.Equal(header.Seal.CreatedAt) ||
		header.GraphSHA256 != header.Seal.ObligationGraphSHA256 ||
		header.LedgerVersion != header.Seal.LedgerVersion ||
		header.WorkingSetVersion != header.Seal.WorkingSetVersion {
		return fmt.Errorf("semantic replay queue trace differs from the sealed public episode")
	}
	if manifest.Scenario.SHA256 != bundle.Authority.Scenario.SHA256 {
		return fmt.Errorf("semantic replay public world digest changed")
	}
	if err := validateSemanticReplayOutcome(header.Seal.Outcome, manifest.Outcome); err != nil {
		return err
	}
	if err := validateSemanticEpisodeTraceProjection(
		episode, trace.Records, header.TraceSHA256,
	); err != nil {
		return err
	}
	return validateSemanticEpisodeDerivedAuthority(episode, trace)
}

func semanticReplayTerminal(trace productionTrace, episode SealedEpisode) (
	cognitionreplay.TerminalAuthority,
	error,
) {
	return cognitionreplay.NewSealedEpisodeTerminal(cognitionreplay.SealedEpisodeTerminal{
		EpisodeID: string(trace.Header.EpisodeID), EpisodeSealSHA256: episode.SealSHA256,
		TraceSHA256:      trace.Header.TraceSHA256,
		EpisodeStartedAt: trace.Header.EpisodeStartedAt, SealedAt: trace.Header.SealedAt,
	})
}

func semanticReplayRevision(value cognition.WorldRevision) *cognitionreplay.PublicRevision {
	if value.Number == 0 {
		return nil
	}
	return &cognitionreplay.PublicRevision{Number: value.Number, SHA256: value.SHA256}
}

const semanticReplayPublicWorldSchema = labyrinth.PublicWorldSchemaV1
