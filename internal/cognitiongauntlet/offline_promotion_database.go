package cognitiongauntlet

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	labyrinthhost "github.com/gryph/omnidex/internal/labyrinth/host"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

type offlinePromotionDatabase struct {
	adminPool     *pgxpool.Pool
	pool          *pgxpool.Pool
	hostAdminPool *pgxpool.Pool
	repository    *queue.Repository
	schema        string
	hostSchema    string
	inferenceRole string
	inferenceURL  string
	hostRole      string
	hostURL       string
	attempt       model.StepAttemptAuthority
}

func prepareOfflinePromotionDatabase(
	ctx context.Context,
	config OfflinePromotionConfig,
	migrations queue.MigrationBundle,
) (*offlinePromotionDatabase, error) {
	return prepareOfflineExecutionDatabase(ctx, config.executionAuthority(), migrations)
}

func prepareOfflineExecutionDatabase(
	ctx context.Context,
	authority offlineExecutionAuthority,
	migrations queue.MigrationBundle,
) (*offlinePromotionDatabase, error) {
	if err := authority.Validate(); err != nil {
		return nil, err
	}
	admin, err := pgxpool.New(ctx, authority.DatabaseURL)
	if err != nil {
		return nil, err
	}
	runtimeSchema, err := randomProcessIdentity("cognition_run_")
	if err != nil {
		admin.Close()
		return nil, err
	}
	hostSchema, err := randomProcessIdentity("cognition_host_")
	if err != nil {
		admin.Close()
		return nil, err
	}
	inferenceRole, err := randomProcessIdentity("cognition_infer_")
	if err != nil {
		admin.Close()
		return nil, err
	}
	inferencePassword, err := randomProcessIdentity("credential-")
	if err != nil {
		admin.Close()
		return nil, err
	}
	hostRole, err := randomProcessIdentity("cognition_host_")
	if err != nil {
		admin.Close()
		return nil, err
	}
	hostPassword, err := randomProcessIdentity("credential-")
	if err != nil {
		admin.Close()
		return nil, err
	}
	if err := createOfflineDatabaseAuthorities(
		ctx, admin, runtimeSchema, hostSchema,
		inferenceRole, inferencePassword, hostRole, hostPassword,
	); err != nil {
		admin.Close()
		return nil, err
	}
	pool, err := promotionPool(ctx, authority.DatabaseURL, runtimeSchema)
	if err != nil {
		admin.Close()
		return nil, err
	}
	// The trusted host transaction qualifies private host tables but must also
	// see the isolated runtime schema to fence the exact queue lease atomically.
	hostAdminPool, err := promotionPool(ctx, authority.DatabaseURL, hostSchema+","+runtimeSchema)
	if err != nil {
		pool.Close()
		admin.Close()
		return nil, err
	}
	database := &offlinePromotionDatabase{
		adminPool: admin, pool: pool, hostAdminPool: hostAdminPool,
		repository: queue.New(pool), schema: runtimeSchema, hostSchema: hostSchema,
		inferenceRole: inferenceRole, hostRole: hostRole,
	}
	if err := database.installRuntime(ctx, migrations); err != nil {
		database.close()
		return nil, err
	}
	if err := database.repository.ProvisionStepAttemptAuthorizerRole(ctx, hostRole); err != nil {
		database.close()
		return nil, err
	}
	if err := database.installHost(ctx); err != nil {
		database.close()
		return nil, err
	}
	if err := grantOfflineRuntimeAuthority(ctx, admin, runtimeSchema, inferenceRole); err != nil {
		database.close()
		return nil, err
	}
	if err := grantOfflineHostAuthority(ctx, admin, hostSchema, hostRole); err != nil {
		database.close()
		return nil, err
	}
	database.inferenceURL, err = restrictedDatabaseURL(
		authority.DatabaseURL, inferenceRole, inferencePassword,
	)
	if err != nil {
		database.close()
		return nil, err
	}
	database.hostURL, err = restrictedDatabaseURL(authority.DatabaseURL, hostRole, hostPassword)
	if err != nil {
		database.close()
		return nil, err
	}
	if err := database.claim(ctx, authority.Budget.WorkingSetBytes); err != nil {
		database.close()
		return nil, err
	}
	return database, nil
}

func (database *offlinePromotionDatabase) installHost(ctx context.Context) error {
	store, err := labyrinthhost.NewStoreInSchema(database.hostAdminPool, database.hostSchema)
	if err != nil {
		return err
	}
	return store.InstallSchema(ctx)
}

func (database *offlinePromotionDatabase) installRuntime(
	ctx context.Context,
	migrations queue.MigrationBundle,
) error {
	return database.repository.EnsureSchema(ctx, migrations)
}

func (database *offlinePromotionDatabase) claim(ctx context.Context, workingSetBytes int) error {
	job, err := database.repository.EnqueueJob(
		ctx, "Offline sealed cognition gauntlet inference.", model.PipelineAssistant, []byte(`{}`),
	)
	if err != nil {
		return err
	}
	worker, err := randomProcessIdentity("gauntlet-inference-")
	if err != nil {
		return err
	}
	claim, err := database.repository.ClaimNextStep(ctx, worker)
	if err != nil {
		return err
	}
	if claim == nil || claim.Job.ID != job.ID {
		return fmt.Errorf("isolated cognition claim returned another job")
	}
	budget, err := fullCognitionWorkingSetBudget(workingSetBytes)
	if err != nil {
		return err
	}
	if _, err := database.repository.CreateCurrentWorkingSet(ctx, claim.Authority, budget); err != nil {
		return err
	}
	database.attempt = claim.Authority
	return nil
}

func (database *offlinePromotionDatabase) revokeInference(ctx context.Context) error {
	return revokeOfflineInferenceLogin(ctx, database.adminPool, database.inferenceRole)
}

func (database *offlinePromotionDatabase) enableInference(ctx context.Context) error {
	return enableOfflineInferenceLogin(ctx, database.adminPool, database.inferenceRole)
}

func (database *offlinePromotionDatabase) revokeHost(ctx context.Context) error {
	return revokeOfflineProcessLogin(ctx, database.adminPool, database.hostRole, "host")
}

func (database *offlinePromotionDatabase) enableHost(ctx context.Context) error {
	return enableOfflineProcessLogin(ctx, database.adminPool, database.hostRole, "host")
}

func (database *offlinePromotionDatabase) close() {
	if database == nil {
		return
	}
	database.hostAdminPool.Close()
	database.pool.Close()
	database.adminPool.Close()
}

func randomProcessIdentity(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value), nil
}
