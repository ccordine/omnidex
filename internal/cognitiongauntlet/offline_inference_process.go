package cognitiongauntlet

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gryph/omnidex/internal/cognitiontransport"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
	buildversion "github.com/gryph/omnidex/internal/version"
	"github.com/jackc/pgx/v5/pgxpool"
)

func RunOfflineInferenceProcess(ctx context.Context, configPath string) error {
	var config inferenceProcessConfig
	if err := loadStrictJSONFile(configPath, &config, "offline inference process configuration"); err != nil {
		return err
	}
	if config.Schema != inferenceProcessConfigSchemaV1 || config.DatabaseURL == "" ||
		config.DatabaseSchema == "" || config.TimeoutSeconds <= 0 ||
		!validDigest(config.ExecutableSHA256) || !validDigest(config.SourceSHA256) ||
		!validCommitIdentity(config.OmnidexCommit) {
		return fmt.Errorf("offline inference process configuration is invalid")
	}
	if err := config.Control.Validate(); err != nil {
		return err
	}
	if err := validateCurrentProcessIdentity(
		config.ExecutableSHA256, config.OmnidexCommit, config.SourceSHA256,
		buildversion.Commit, buildversion.SourceSHA256,
	); err != nil {
		return err
	}
	bundle, err := LoadPublicInferenceBundle(config.PublicBundlePath)
	if err != nil {
		return err
	}
	if bundle.Authority.RatGeneration.Runtime.SourceSHA256 != config.SourceSHA256 ||
		bundle.Authority.RatGeneration.Runtime.ExecutableSHA256 != config.ExecutableSHA256 {
		return fmt.Errorf("offline inference bundle changed the attested build authority")
	}
	contaminatedOracle, err := loadContaminatedInferenceGrant(config, bundle)
	if err != nil {
		return err
	}
	if _, err := os.Stat(config.EpisodePath); !os.IsNotExist(err) {
		return fmt.Errorf("offline inference episode output already exists or is inaccessible")
	}
	poolConfig, err := pgxpool.ParseConfig(config.DatabaseURL)
	if err != nil {
		return err
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = config.DatabaseSchema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return err
	}
	defer pool.Close()
	timeout := time.Duration(config.TimeoutSeconds) * time.Second
	transportClient, err := cognitiontransport.NewClient(
		config.EnvironmentURL, config.EnvironmentToken, &http.Client{Timeout: timeout},
	)
	if err != nil {
		return err
	}
	brain := bundle.Authority.RatGeneration.Fixed.Brain
	modelClient := ollama.New(
		config.OllamaEndpoint, brain.Model, "", timeout, brain.NativeContextLimit,
	)
	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	heartbeat := maintainInferenceLease(runCtx, queue.New(pool), config.Attempt, cancel)
	fullRequest := PublicFullCognitionRunRequest{
		Attempt: config.Attempt, Pool: pool, Client: modelClient,
		Environment: transportClient, Completion: transportClient,
		EpisodeSealPath: config.EpisodePath, OmnidexCommit: config.OmnidexCommit,
		LedgerSchemaVersion:     config.LedgerSchemaVersion,
		WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: config.ProjectionPolicyVersion,
	}
	var result PublicFullCognitionRunResult
	var ablation PublicAblationRunResult
	var runErr error
	if bundle.Authority.Variant == VariantFullCognition {
		switch config.Control.Mode {
		case inferenceRunToTerminal:
			result, runErr = RunPublicFullCognition(runCtx, bundle, fullRequest)
		case inferenceStopBeforeNextCall:
			result, runErr = runControlledPublicFullCognition(runCtx, bundle, fullRequest, config.Control)
		default:
			runErr = fmt.Errorf("offline inference process mode %q is not registered", config.Control.Mode)
		}
	} else if config.Control.Mode != inferenceRunToTerminal {
		runErr = fmt.Errorf("controlled takeover is unavailable for ablation variant %q", bundle.Authority.Variant)
	} else {
		ablationRequest := PublicAblationRunRequest{
			Actor: bindingAttemptRef(config.Attempt), Client: modelClient,
			Environment: transportClient, Completion: transportClient,
			EpisodeSealPath: config.EpisodePath, OmnidexCommit: config.OmnidexCommit,
			LedgerSchemaVersion:     config.LedgerSchemaVersion,
			WorkingSetPolicyVersion: config.WorkingSetPolicyVersion,
			ProjectionPolicyVersion: config.ProjectionPolicyVersion,
		}
		if contaminatedOracle != nil {
			ablationRequest.ContaminatedOracle = contaminatedOracle
		}
		ablation, runErr = RunPublicAblation(runCtx, bundle, ablationRequest)
	}
	cancel(nil)
	heartbeatErr := <-heartbeat
	if heartbeatErr != nil {
		return heartbeatErr
	}
	if runErr != nil {
		return runErr
	}
	if bundle.Authority.Variant == VariantFullCognition {
		return result.Validate()
	}
	return ablation.Validate()
}
