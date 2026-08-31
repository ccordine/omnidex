package runtime

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/api"
	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/llmprovider"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/worker"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Runtime owns the one production HTTP and workload execution lifecycle.
type Runtime struct {
	context    context.Context
	cancel     context.CancelFunc
	pool       *pgxpool.Pool
	server     *api.Server
	worker     *worker.Service
	listenAddr string
}

// New constructs the production runtime from one configuration authority and
// installs the checked-in database/setup.sql definition before any service
// loop can consume the database.
func New(ctx context.Context, cfg config.Config, logger *log.Logger) (*Runtime, error) {
	if ctx == nil {
		return nil, fmt.Errorf("runtime requires context")
	}
	if strings.TrimSpace(cfg.DatabaseURL) == "" {
		return nil, fmt.Errorf("runtime database operation requires DATABASE_URL")
	}
	lifecycleContext, cancel := context.WithCancel(ctx)

	pool, err := db.ConnectRuntime(
		lifecycleContext, cfg.DatabaseURL, cfg.DatabaseSchema, database.SetupSQL(),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("connect runtime database: %w", err)
	}
	repo := queue.New(pool, cfg.ModelAuthority)

	transports := llmprovider.NewLazyFromConfig(cfg)
	server, err := api.NewServer(repo, transports.Embeddings, api.ServerOptions{
		LifecycleContext:     lifecycleContext,
		ProviderConfig:       cfg,
		CoreURL:              cfg.CoreURL,
		ListenAddr:           cfg.ListenAddr,
		HostAgentURL:         cfg.HostAgentURL,
		HostAgentToken:       cfg.HostAgentToken,
		IntegrationAPIToken:  cfg.IntegrationAPIToken,
		RealtimeStreamMaxAge: cfg.RealtimeStreamMaxAge,
		RealtimeHeartbeat:    cfg.RealtimeHeartbeat,
		RealtimeWriteTimeout: cfg.RealtimeWriteTimeout,
		RedisURL:             cfg.RedisURL,
		UISessionTTL:         cfg.UISessionTTL,
	})
	if err != nil {
		cancel()
		pool.Close()
		return nil, fmt.Errorf("construct HTTP server: %w", err)
	}
	workers, err := worker.New(
		repo,
		transports.Stations,
		nil,
		worker.Options{
			PollInterval:           cfg.WorkerPollInterval,
			InferenceContextTokens: cfg.InferenceContextTokens,
			Logger:                 logger,
		},
	)
	if err != nil {
		cancel()
		pool.Close()
		return nil, fmt.Errorf("construct workload service: %w", err)
	}

	return &Runtime{
		context: lifecycleContext, cancel: cancel, pool: pool, server: server, worker: workers,
		listenAddr: strings.TrimSpace(cfg.ListenAddr),
	}, nil
}

// Run starts the HTTP and workload loops under the same cancellation
// authority and stops the survivor when either loop returns.
func (runtime *Runtime) Run() error {
	if runtime == nil || runtime.context == nil || runtime.cancel == nil || runtime.server == nil ||
		runtime.worker == nil || runtime.listenAddr == "" {
		return fmt.Errorf("runtime is not fully constructed")
	}
	errorsByLoop := make(chan error, 2)
	go func() {
		errorsByLoop <- api.Run(runtime.context, runtime.listenAddr, runtime.server.Handler())
	}()
	go func() {
		errorsByLoop <- runtime.worker.Start(runtime.context)
	}()
	first := <-errorsByLoop
	runtime.cancel()
	second := <-errorsByLoop
	return errors.Join(first, second)
}

// Close releases the runtime database pool after Run returns.
func (runtime *Runtime) Close() {
	if runtime == nil {
		return
	}
	if runtime.cancel != nil {
		runtime.cancel()
	}
	if runtime.pool != nil {
		runtime.pool.Close()
	}
}
