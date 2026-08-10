package cognitiongauntlet

import (
	"context"
	"fmt"
	"time"
)

func evaluateOfflineTakeover(
	ctx context.Context,
	config OfflinePromotionConfig,
	database *offlinePromotionDatabase,
	bundle PublicInferenceBundle,
	paths OfflinePromotionPaths,
	privateOracleCredential string,
	executable string,
	executableSHA string,
	temporary string,
) (Evaluation, string, int, time.Time, SealedEpisode, error) {
	episode, err := LoadSealedEpisode(paths.Episode)
	if err != nil {
		return Evaluation{}, "", 0, time.Time{}, SealedEpisode{}, err
	}
	public := PublicFullCognitionRunResult{Authority: bundle.Authority, Episode: episode}
	if err := public.Validate(); err != nil {
		return Evaluation{}, "", 0, time.Time{}, SealedEpisode{}, err
	}
	if err := completeOfflineInferenceStep(ctx, database); err != nil {
		return Evaluation{}, "", 0, time.Time{}, SealedEpisode{}, err
	}
	processPath := takeoverProcessPath(temporary, "evaluator-process")
	process := newEvaluatorProcessConfig(config, paths, privateOracleCredential, executableSHA)
	if err := writePrivateProcessFile(processPath, process, "takeover evaluator process configuration"); err != nil {
		return Evaluation{}, "", 0, time.Time{}, SealedEpisode{}, err
	}
	startedAt := time.Now().UTC()
	pid, err := runOfflineChild(ctx, executable, "evaluate", processPath, executableSHA)
	if err != nil {
		return Evaluation{}, "", 0, time.Time{}, SealedEpisode{}, err
	}
	evaluation, artifactSHA256, err := LoadEvaluationArtifact(paths.Evaluation)
	if err != nil {
		return Evaluation{}, "", 0, time.Time{}, SealedEpisode{}, err
	}
	if evaluation.EpisodeSealSHA256 != episode.SealSHA256 {
		return Evaluation{}, "", 0, time.Time{}, SealedEpisode{}, fmt.Errorf(
			"takeover evaluator returned another episode authority",
		)
	}
	publicSHA, err := bundle.Authority.SHA256()
	if err != nil {
		return Evaluation{}, "", 0, time.Time{}, SealedEpisode{}, err
	}
	if episode.Manifest.PublicRunAuthoritySHA256 != publicSHA {
		return Evaluation{}, "", 0, time.Time{}, SealedEpisode{}, fmt.Errorf(
			"takeover episode changed its public run authority",
		)
	}
	return evaluation, artifactSHA256, pid, startedAt, episode, nil
}
