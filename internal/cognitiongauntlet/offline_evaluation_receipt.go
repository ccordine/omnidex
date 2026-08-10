package cognitiongauntlet

import (
	"fmt"
	"time"
)

func (inference offlinePromotionInference) executionInference() offlineExecutionInference {
	return offlineExecutionInference{
		authority: inference.config.executionAuthority(), executable: inference.executable,
		executableSHA256: inference.executableSHA256, paths: inference.paths,
		bundle: inference.bundle, episode: inference.episode,
		privateCredential: inference.privateOracleCredential,
		databaseSchema:    inference.databaseSchema, generatorPID: inference.generatorPID,
		generatorExitedAt: inference.generatorExitedAt, host: inference.host,
		inferencePID: inference.inferencePID, inferenceStartedAt: inference.inferenceStartedAt,
		inferenceExitedAt: inference.inferenceExitedAt,
	}
}

func buildOfflinePromotionReceipt(
	inference offlineExecutionInference,
	evaluation Evaluation,
	evaluationArtifactSHA256 string,
	evaluatorPID int,
	evaluatorStartedAt time.Time,
	completedAt time.Time,
) (OfflinePromotionReceipt, error) {
	if err := inference.authority.Validate(); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if err := evaluation.Validate(); err != nil {
		return OfflinePromotionReceipt{}, err
	}
	if evaluation.EpisodeSealSHA256 != inference.episode.SealSHA256 ||
		!validDigest(evaluationArtifactSHA256) {
		return OfflinePromotionReceipt{}, fmt.Errorf("offline evaluation changed its sealed episode")
	}
	publicSHA256, err := inference.bundle.Authority.SHA256()
	if err != nil {
		return OfflinePromotionReceipt{}, err
	}
	receipt := OfflinePromotionReceipt{
		Schema:                   OfflinePromotionReceiptSchemaV1,
		PublicRunAuthoritySHA256: publicSHA256,
		EpisodeSealSHA256:        inference.episode.SealSHA256,
		EvaluationOracleSHA256:   evaluation.OracleSHA256,
		EvaluationArtifactSHA256: evaluationArtifactSHA256,
		ExecutableSHA256:         inference.executableSHA256,
		SourceSHA256:             inference.authority.RatGeneration.Runtime.SourceSHA256,
		MigrationsSHA256:         inference.authority.RatGeneration.Runtime.MigrationsSHA256,
		RuntimeVersion:           inference.authority.RatGeneration.Runtime.Version,
		OmnidexCommit:            inference.authority.OmnidexCommit,
		DatabaseSchema:           inference.databaseSchema,
		GeneratorPID:             inference.generatorPID, GeneratorExitedAt: inference.generatorExitedAt,
		Host: inference.host, InferencePID: inference.inferencePID,
		InferenceStartedAt: inference.inferenceStartedAt,
		InferenceExitedAt:  inference.inferenceExitedAt,
		EvaluatorPID:       evaluatorPID, EvaluatorStartedAt: evaluatorStartedAt,
		CompletedAt: completedAt,
	}
	return receipt, receipt.Validate()
}
