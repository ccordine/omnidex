package cognitiongauntlet

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/queue"
)

func verifyLiveStaleProbeArtifacts(
	resume OfflineResumeConfig,
	base OfflinePromotionConfig,
	receipt LiveStaleProbeReceipt,
) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	for _, proof := range receipt.Probes {
		config := liveStaleProbeArtifactConfig(base, proof.Port)
		paths := config.Paths()
		bundle, err := LoadPublicInferenceBundle(paths.PublicBundle)
		if err != nil {
			return err
		}
		publicSHA, err := bundle.Authority.SHA256()
		if err != nil || publicSHA != receipt.PublicRunAuthoritySHA256 ||
			bundle.Authority.RatGeneration != resume.RatGeneration ||
			bundle.Authority.Budget != resume.Budget {
			return fmt.Errorf("live Resume port %q changed public authority", proof.Port)
		}
		episode, err := LoadSealedEpisode(paths.Episode)
		if err != nil || episode.SealSHA256 != proof.EpisodeSealSHA256 ||
			episode.Manifest.EpisodeID != proof.Episode.ID {
			return fmt.Errorf("live Resume port %q changed its sealed episode: %w", proof.Port, err)
		}
		promotion, promotionSHA, err := loadOfflinePromotionReceiptArtifact(paths.Receipt)
		if err != nil || promotionSHA != proof.PromotionReceiptSHA256 ||
			promotion.DatabaseSchema != proof.DatabaseSchema ||
			promotion.InferenceStartedAt != proof.InferenceStartedAt ||
			promotion.InferenceExitedAt != proof.InferenceExitedAt ||
			promotion.EvaluatorStartedAt != proof.EvaluatorStartedAt ||
			promotion.CompletedAt != proof.EvaluatorCompletedAt {
			return fmt.Errorf("live Resume port %q changed its promotion receipt: %w", proof.Port, err)
		}
		evaluation, err := promotion.VerifyEvaluationArtifact(paths.Evaluation)
		if err != nil || promotion.EvaluationArtifactSHA256 != proof.EvaluationSHA256 ||
			!resumeEvaluationQualified(evaluation) {
			return fmt.Errorf("live Resume port %q changed its private evaluation: %w", proof.Port, err)
		}
		state, err := reopenLiveStaleDurableState(
			context.Background(), resume.DatabaseURL, proof, episode,
		)
		if err != nil || !reflect.DeepEqual(state, proof.StateAfter) ||
			!reflect.DeepEqual(proof.StateBefore, proof.StateAfter) {
			return fmt.Errorf("live Resume port %q durable state changed: %w", proof.Port, err)
		}
	}
	return nil
}

func reopenLiveStaleDurableState(
	ctx context.Context,
	databaseURL string,
	proof LiveStalePortProof,
	episode SealedEpisode,
) (LiveStaleDurableState, error) {
	runtimePool, err := promotionPool(ctx, databaseURL, proof.DatabaseSchema)
	if err != nil {
		return LiveStaleDurableState{}, err
	}
	defer runtimePool.Close()
	hostPool, err := promotionPool(
		ctx, databaseURL, proof.HostSchema+","+proof.DatabaseSchema,
	)
	if err != nil {
		return LiveStaleDurableState{}, err
	}
	defer hostPool.Close()
	database := &offlinePromotionDatabase{
		pool: runtimePool, hostAdminPool: hostPool,
		repository: queue.New(runtimePool), schema: proof.DatabaseSchema,
		hostSchema: proof.HostSchema,
	}
	return captureLiveStaleDurableState(ctx, database, episode)
}
