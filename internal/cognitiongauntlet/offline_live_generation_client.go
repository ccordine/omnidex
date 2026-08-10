package cognitiongauntlet

import (
	"context"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

const LiveGenerationCheckpointSchemaV1 = "omnidex.live-generation-checkpoint.v1"

type LiveGenerationCheckpoint struct {
	Schema         string                     `json:"schema"`
	PID            int                        `json:"pid"`
	Attempt        model.StepAttemptAuthority `json:"attempt"`
	PreparedSHA256 string                     `json:"prepared_sha256"`
	EnteredAt      time.Time                  `json:"entered_at"`
}

type pausingExactClient struct {
	llm.Client
	exact    llm.ExactPreparedContractClient
	observer llm.ProviderIdentityObserver
	attempt  model.StepAttemptAuthority
	path     string
	once     sync.Once
	pauseErr error
	pause    func() error
}

func newPausingExactClient(
	base llm.Client,
	attempt model.StepAttemptAuthority,
	path string,
) (llm.Client, error) {
	exact, err := llm.RequireExactPreparedContract(base)
	if err != nil {
		return nil, err
	}
	observer, ok := base.(llm.ProviderIdentityObserver)
	if !ok || !validTakeoverAttempt(attempt) || path == "" {
		return nil, fmt.Errorf("live-generation client authority is incomplete")
	}
	return &pausingExactClient{
		Client: base, exact: exact, observer: observer, attempt: attempt, path: path,
		pause: func() error { return syscall.Kill(os.Getpid(), syscall.SIGSTOP) },
	}, nil
}

func (client *pausingExactClient) RequireExactPreparedContract() error {
	return client.exact.RequireExactPreparedContract()
}

func (client *pausingExactClient) ValidateExactPreparedContract(prepared llm.PreparedModel) error {
	return client.exact.ValidateExactPreparedContract(prepared)
}

func (client *pausingExactClient) ValidateExactPreparedProvider(
	expected llm.ProviderIdentityExpectation,
) error {
	return client.exact.ValidateExactPreparedProvider(expected)
}

func (client *pausingExactClient) ObserveProviderIdentity(
	ctx context.Context,
	request llm.ProviderIdentityObservationRequest,
) (llm.ObservedProviderIdentity, error) {
	return client.observer.ObserveProviderIdentity(ctx, request)
}

func (client *pausingExactClient) GeneratePreparedExact(
	ctx context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	client.once.Do(func() {
		preparedSHA, err := preparedModelAuthoritySHA256(prepared)
		if err != nil {
			client.pauseErr = err
			return
		}
		checkpoint := LiveGenerationCheckpoint{
			Schema: LiveGenerationCheckpointSchemaV1, PID: os.Getpid(), Attempt: client.attempt,
			PreparedSHA256: preparedSHA, EnteredAt: time.Now().UTC(),
		}
		if err := checkpoint.Validate(); err != nil {
			client.pauseErr = err
			return
		}
		if err := sealScenarioArtifact(client.path, checkpoint, "live generation checkpoint"); err != nil {
			client.pauseErr = err
			return
		}
		client.pauseErr = client.pause()
	})
	if client.pauseErr != nil {
		return llm.PreparedGeneration{}, client.pauseErr
	}
	return client.exact.GeneratePreparedExact(ctx, prepared)
}

func (checkpoint LiveGenerationCheckpoint) Validate() error {
	if checkpoint.Schema != LiveGenerationCheckpointSchemaV1 || checkpoint.PID <= 0 ||
		!validTakeoverAttempt(checkpoint.Attempt) || !validDigest(checkpoint.PreparedSHA256) ||
		checkpoint.EnteredAt.IsZero() {
		return fmt.Errorf("live generation checkpoint is invalid")
	}
	return nil
}

func LoadLiveGenerationCheckpoint(path string) (LiveGenerationCheckpoint, error) {
	var checkpoint LiveGenerationCheckpoint
	if err := loadStrictJSONFile(path, &checkpoint, "live generation checkpoint"); err != nil {
		return LiveGenerationCheckpoint{}, err
	}
	if err := checkpoint.Validate(); err != nil {
		return LiveGenerationCheckpoint{}, err
	}
	return checkpoint, nil
}

func preparedModelAuthoritySHA256(prepared llm.PreparedModel) (string, error) {
	return digestJSON(prepared)
}
