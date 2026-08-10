package cognitiongauntlet

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type liveStalePortController struct {
	port           liveStalePort
	attempt        model.StepAttemptAuthority
	checkpointPath string
	rejectionPath  string
	pause          func() error
	mu             sync.Mutex
	triggered      bool
}

func newLiveStalePortController(
	port liveStalePort,
	attempt model.StepAttemptAuthority,
	checkpointPath string,
	rejectionPath string,
) (*liveStalePortController, error) {
	probe := &liveStalePortController{
		port: port, attempt: attempt, checkpointPath: checkpointPath,
		rejectionPath: rejectionPath,
		pause:         func() error { return syscall.Kill(os.Getpid(), syscall.SIGSTOP) },
	}
	if port.Validate() != nil || !validTakeoverAttempt(attempt) ||
		checkpointPath == "" || rejectionPath == "" || checkpointPath == rejectionPath {
		return nil, fmt.Errorf("live stale-port probe authority is incomplete")
	}
	return probe, nil
}

func (probe *liveStalePortController) before(port liveStalePort, command any) (bool, error) {
	if probe == nil || port != probe.port {
		return false, nil
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.triggered {
		return false, nil
	}
	commandSHA, err := digestJSON(command)
	if err != nil {
		return false, err
	}
	checkpoint := liveStalePortCheckpoint{
		Schema: liveStalePortCheckpointSchemaV1, Port: port, PID: os.Getpid(),
		Attempt: probe.attempt, CommandSHA256: commandSHA, EnteredAt: time.Now().UTC(),
	}
	if err := checkpoint.Validate(); err != nil {
		return false, err
	}
	if err := sealScenarioArtifact(
		probe.checkpointPath, checkpoint, "live stale-port checkpoint",
	); err != nil {
		return false, err
	}
	probe.triggered = true
	return true, probe.pause()
}

func (probe *liveStalePortController) after(
	port liveStalePort,
	command any,
	err error,
) error {
	commandSHA, digestErr := digestJSON(command)
	if digestErr != nil {
		return errors.Join(err, digestErr)
	}
	rejected := errors.Is(err, queue.ErrStaleStepAttempt)
	if port == liveStaleEnvironmentApply {
		rejected = errors.Is(err, cognition.ErrAuthorityDenied)
	}
	if !rejected {
		return errors.Join(err, fmt.Errorf("live stale-port %q did not reject its expired actor", port))
	}
	rejection := liveStalePortRejection{
		Schema: liveStalePortRejectionSchemaV1, Port: port, PID: os.Getpid(),
		Attempt: probe.attempt, CommandSHA256: commandSHA,
		ErrorClass: port.expectedError(), RejectedAt: time.Now().UTC(),
	}
	if sealErr := sealScenarioArtifact(
		probe.rejectionPath, rejection, "live stale-port rejection",
	); sealErr != nil {
		return errors.Join(err, sealErr)
	}
	return err
}

func (probe *liveStalePortController) afterPolicy(
	command any,
	result cognitionpolicy.CallResult,
	err error,
) error {
	commandSHA, digestErr := digestJSON(command)
	if digestErr != nil {
		return errors.Join(err, digestErr)
	}
	if !errors.Is(err, queue.ErrStaleStepAttempt) {
		return errors.Join(err, fmt.Errorf("live stale policy finish did not reject its expired actor"))
	}
	rejection := liveStalePortRejection{
		Schema: liveStalePortRejectionSchemaV1, Port: liveStalePolicyFinish,
		PID: os.Getpid(), Attempt: probe.attempt, CommandSHA256: commandSHA,
		ErrorClass: liveStaleErrorAttempt, RejectedAt: time.Now().UTC(),
		ProviderRequestDispatched: result.ProviderRequestDispatched,
		ProviderUsagePresent:      result.ProviderUsagePresent,
		ProviderUsage:             result.ProviderUsage, ProviderDoneReason: result.ProviderDoneReason,
	}
	if validateErr := rejection.Validate(); validateErr != nil {
		return errors.Join(err, validateErr)
	}
	if sealErr := sealScenarioArtifact(
		probe.rejectionPath, rejection, "live stale-port rejection",
	); sealErr != nil {
		return errors.Join(err, sealErr)
	}
	return err
}
