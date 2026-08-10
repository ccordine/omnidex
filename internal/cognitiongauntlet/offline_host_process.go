package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitiontransport"
	"github.com/gryph/omnidex/internal/labyrinth"
	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/queue"
	buildversion "github.com/gryph/omnidex/internal/version"
	"github.com/jackc/pgx/v5"
)

func RunOfflineHostProcess(ctx context.Context, configPath string) error {
	if ctx == nil {
		return fmt.Errorf("offline cognition host context is nil")
	}
	var config hostProcessConfig
	if err := loadStrictJSONFile(configPath, &config, "offline host process configuration"); err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
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
	scenario, err := loadPrivateHostScenario(config.HostScenarioPath)
	if err != nil {
		return err
	}
	if scenario.Ref() != config.Scenario || bundle.Authority.Scenario != config.Scenario {
		return fmt.Errorf("offline host scenario differs from public bootstrap authority")
	}
	pool, err := promotionPool(
		ctx, config.DatabaseURL, config.HostSchema+","+config.DatabaseSchema,
	)
	if err != nil {
		return err
	}
	defer pool.Close()
	var currentRole string
	if err := pool.QueryRow(ctx, `SELECT current_user`).Scan(&currentRole); err != nil {
		return err
	}
	if currentRole != config.ExpectedRole {
		return fmt.Errorf("offline host database role differs from sealed authority")
	}
	hostStore, err := labyrinthhost.NewStoreInSchema(pool, config.HostSchema)
	if err != nil {
		return err
	}
	repository := queue.New(pool)
	episode, err := PublicVariantEpisodeRef(bundle.Authority)
	if err != nil {
		return err
	}
	resolver := func(_ context.Context, reference cognition.ScenarioRef) (labyrinth.Scenario, error) {
		if reference != scenario.Ref() {
			return labyrinth.Scenario{}, fmt.Errorf("sealed offline host rejected another scenario")
		}
		return scenario, nil
	}
	authorize := standaloneTransactionAuthorizer(pool, repository)
	environment, err := labyrinthhost.NewSurfaceEnvironment(
		hostStore, episode, resolver, authorize,
		transactionAttemptAuthorizer(repository), bundle.Authority.SurfaceVersion,
	)
	if err != nil {
		return err
	}
	authenticator, err := cognitiontransport.NewBearerAuthenticator(config.EnvironmentToken)
	if err != nil {
		return err
	}
	handler, err := cognitiontransport.NewHandler(environment, environment, authenticator)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	ready := hostProcessReady{
		Schema: hostProcessReadySchemaV1, PID: os.Getpid(),
		BaseURL: "http://" + listener.Addr().String(), CurrentRole: currentRole,
		Scenario: scenario.Ref(), StartedAt: time.Now().UTC(),
	}
	if err := ready.Validate(config); err != nil {
		_ = listener.Close()
		return err
	}
	if err := sealScenarioArtifact(config.ReadyPath, ready, "offline host readiness"); err != nil {
		_ = listener.Close()
		return err
	}
	serve := make(chan error, 1)
	go func() { serve <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown offline host: %w", err)
		}
		err := <-serve
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve offline host: %w", err)
		}
		return nil
	case err := <-serve:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("offline host stopped before controller shutdown")
		}
		return fmt.Errorf("serve offline host: %w", err)
	}
}

func standaloneTransactionAuthorizer(
	pool interface {
		BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
	},
	repository *queue.Repository,
) func(context.Context, cognition.AttemptRef) error {
	return func(ctx context.Context, actor cognition.AttemptRef) error {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if err := transactionAttemptAuthorizer(repository)(ctx, tx, actor); err != nil {
			return err
		}
		return tx.Rollback(ctx)
	}
}

func (config hostProcessConfig) Validate() error {
	if config.Schema != hostProcessConfigSchemaV1 || config.DatabaseURL == "" ||
		!validDigest(config.ExecutableSHA256) || !validDigest(config.SourceSHA256) ||
		!validCommitIdentity(config.OmnidexCommit) {
		return fmt.Errorf("offline host process configuration is invalid")
	}
	if err := config.Scenario.Validate(); err != nil {
		return err
	}
	for label, value := range map[string]string{
		"runtime schema": config.DatabaseSchema, "host schema": config.HostSchema,
		"expected role": config.ExpectedRole, "environment token": config.EnvironmentToken,
	} {
		if err := requireExact(value, "offline host "+label, 512); err != nil {
			return err
		}
	}
	for _, path := range []string{
		config.HostScenarioPath, config.PublicBundlePath, config.ReadyPath,
	} {
		if path == "" || filepath.Clean(path) != path {
			return fmt.Errorf("offline host artifact path is inexact")
		}
	}
	parsed, err := url.Parse(config.DatabaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" {
		return fmt.Errorf("offline host database URL is invalid")
	}
	if _, err := os.Lstat(config.ReadyPath); !os.IsNotExist(err) {
		return fmt.Errorf("offline host readiness output already exists or is inaccessible")
	}
	return nil
}
