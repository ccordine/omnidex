package cognitiongauntlet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

type offlinePromotionHost struct {
	baseURL              string
	token                string
	pid                  int
	started              time.Time
	exited               time.Time
	database             *offlinePromotionDatabase
	child                *offlineChildProcess
	exits                <-chan offlineHostExit
	ready                string
	role                 string
	scenario             cognition.ScenarioRef
	scenarioConfigSHA256 string
	readySHA256          string
	mu                   sync.Mutex
	closed               bool
}

type offlineHostExit struct {
	pid int
	err error
	at  time.Time
}

func startOfflinePromotionHost(
	ctx context.Context,
	config OfflinePromotionConfig,
	database *offlinePromotionDatabase,
	bundle PublicInferenceBundle,
	hostScenarioPath string,
	executable string,
	executableSHA256 string,
	temporary string,
) (*offlinePromotionHost, error) {
	return startOfflineExecutionHost(
		ctx, config.executionAuthority(), database, bundle, hostScenarioPath,
		config.Paths().PublicBundle, executable, executableSHA256, temporary,
	)
}

func startOfflineExecutionHost(
	ctx context.Context,
	authority offlineExecutionAuthority,
	database *offlinePromotionDatabase,
	bundle PublicInferenceBundle,
	hostScenarioPath string,
	publicBundlePath string,
	executable string,
	executableSHA256 string,
	temporary string,
) (*offlinePromotionHost, error) {
	if ctx == nil || database == nil {
		return nil, fmt.Errorf("offline host launch authority is invalid")
	}
	token, err := randomProcessIdentity("environment-token-")
	if err != nil {
		return nil, err
	}
	processPath := filepath.Join(temporary, "host-process.json")
	readyPath := filepath.Join(temporary, "host-ready.json")
	process := newHostProcessConfigForExecution(
		authority, database, bundle, hostScenarioPath, publicBundlePath,
		readyPath, token, executableSHA256,
	)
	if err := process.Validate(); err != nil {
		return nil, err
	}
	configSHA256, err := digestJSON(process)
	if err != nil {
		return nil, err
	}
	if err := writePrivateProcessFile(processPath, process, "offline host process configuration"); err != nil {
		return nil, err
	}
	if err := database.enableHost(ctx); err != nil {
		return nil, err
	}
	child, err := startOfflineChild(ctx, executable, "host", processPath, executableSHA256)
	if err != nil {
		_ = database.revokeHost(context.Background())
		return nil, err
	}
	exits := make(chan offlineHostExit, 1)
	go func() {
		pid, waitErr := child.wait()
		exits <- offlineHostExit{pid: pid, err: waitErr, at: time.Now().UTC()}
	}()
	ready, err := waitForOfflineHostReady(ctx, process, child.pid(), exits)
	if err != nil {
		_ = child.signal(syscall.SIGKILL)
		_ = database.revokeHost(context.Background())
		return nil, err
	}
	readySHA256, err := digestJSON(ready)
	if err != nil {
		_ = child.signal(syscall.SIGKILL)
		_ = database.revokeHost(context.Background())
		return nil, err
	}
	for _, path := range []string{processPath, hostScenarioPath} {
		if err := os.Remove(path); err != nil {
			_ = child.signal(syscall.SIGKILL)
			_ = database.revokeHost(context.Background())
			return nil, fmt.Errorf("remove consumed offline host authority: %w", err)
		}
	}
	return &offlinePromotionHost{
		baseURL: ready.BaseURL, token: token, pid: child.pid(), started: ready.StartedAt,
		database: database, child: child, exits: exits, ready: readyPath, role: ready.CurrentRole,
		scenario:             ready.Scenario,
		scenarioConfigSHA256: configSHA256, readySHA256: readySHA256,
	}, nil
}

func (host *offlinePromotionHost) close(ctx context.Context) error {
	if host == nil {
		return nil
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.closed {
		return nil
	}
	host.closed = true
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var stopErr error
	select {
	case exited := <-host.exits:
		host.exited = exited.at
		if exited.err == nil {
			stopErr = fmt.Errorf("offline host exited before controller shutdown")
		} else {
			stopErr = fmt.Errorf("offline host exited before controller shutdown: %w", exited.err)
		}
	default:
		if err := host.child.signal(syscall.SIGTERM); err != nil {
			stopErr = fmt.Errorf("stop offline host: %w", err)
		} else {
			select {
			case exited := <-host.exits:
				host.exited = exited.at
				stopErr = exited.err
			case <-waitCtx.Done():
				_ = host.child.signal(syscall.SIGKILL)
				stopErr = fmt.Errorf("wait for offline host shutdown: %w", waitCtx.Err())
			}
		}
	}
	revokeErr := host.database.revokeHost(context.Background())
	removeErr := os.Remove(host.ready)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(stopErr, revokeErr, removeErr)
}

func (host *offlinePromotionHost) receipt() (OfflineHostReceipt, error) {
	if host == nil {
		return OfflineHostReceipt{}, fmt.Errorf("offline host receipt source is nil")
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	receipt := OfflineHostReceipt{
		Schema: offlineHostReceiptSchemaV1, PID: host.pid, Role: host.role, Scenario: host.scenario,
		ConfigSHA256: host.scenarioConfigSHA256, ReadySHA256: host.readySHA256,
		StartedAt: host.started, ExitedAt: host.exited,
	}
	if err := receipt.Validate(); err != nil {
		return OfflineHostReceipt{}, err
	}
	return receipt, nil
}
