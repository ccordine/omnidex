package worker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

const (
	directCodingProcessGroupTerminateGrace = time.Second
	directCodingProcessGroupKillGrace      = 2 * time.Second
	directCodingProcessGroupPollInterval   = 10 * time.Millisecond
)

type directCodingProcessWaitState struct {
	done bool
	err  error
}

func runDirectCodingVerificationProcess(ctx context.Context, process *exec.Cmd) error {
	if ctx == nil || process == nil {
		return fmt.Errorf("verification process requires one command and context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	process.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
	if err := process.Start(); err != nil {
		return err
	}
	processGroupID := process.Process.Pid
	waited := make(chan error, 1)
	go func() { waited <- process.Wait() }()
	select {
	case waitErr := <-waited:
		alive, groupErr := directCodingProcessGroupAlive(processGroupID)
		if groupErr != nil {
			return errors.Join(waitErr, groupErr)
		}
		if !alive {
			return waitErr
		}
		state := directCodingProcessWaitState{done: true, err: waitErr}
		terminationErr := terminateDirectCodingProcessGroup(
			processGroupID, waited, &state,
		)
		return errors.Join(
			waitErr,
			fmt.Errorf("verification command left a running process-group descendant"),
			terminationErr,
		)
	case <-ctx.Done():
		state := directCodingProcessWaitState{}
		terminationErr := terminateDirectCodingProcessGroup(
			processGroupID, waited, &state,
		)
		return errors.Join(state.err, terminationErr)
	}
}

func terminateDirectCodingProcessGroup(
	processGroupID int,
	waited <-chan error,
	state *directCodingProcessWaitState,
) error {
	if processGroupID <= 0 || state == nil {
		return fmt.Errorf("terminate verification process group: invalid process authority")
	}
	if err := signalDirectCodingProcessGroup(processGroupID, syscall.SIGTERM); err != nil {
		return err
	}
	complete, err := awaitDirectCodingProcessGroupExit(
		processGroupID, waited, state, directCodingProcessGroupTerminateGrace,
	)
	if err != nil || complete {
		return err
	}
	if err := signalDirectCodingProcessGroup(processGroupID, syscall.SIGKILL); err != nil {
		return err
	}
	complete, err = awaitDirectCodingProcessGroupExit(
		processGroupID, waited, state, directCodingProcessGroupKillGrace,
	)
	if err != nil {
		return err
	}
	if !complete {
		return fmt.Errorf(
			"verification process group %d remained after bounded SIGKILL wait",
			processGroupID,
		)
	}
	return nil
}

func signalDirectCodingProcessGroup(processGroupID int, signal syscall.Signal) error {
	err := syscall.Kill(-processGroupID, signal)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return fmt.Errorf(
		"signal verification process group %d with %s: %w",
		processGroupID, signal, err,
	)
}

func awaitDirectCodingProcessGroupExit(
	processGroupID int,
	waited <-chan error,
	state *directCodingProcessWaitState,
	timeout time.Duration,
) (bool, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(directCodingProcessGroupPollInterval)
	defer ticker.Stop()
	for {
		if !state.done {
			select {
			case state.err = <-waited:
				state.done = true
			default:
			}
		}
		alive, err := directCodingProcessGroupAlive(processGroupID)
		if err != nil {
			return false, err
		}
		if state.done && !alive {
			return true, nil
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			return false, nil
		}
	}
}

func directCodingProcessGroupAlive(processGroupID int) (bool, error) {
	err := syscall.Kill(-processGroupID, 0)
	switch {
	case err == nil, errors.Is(err, syscall.EPERM):
		return true, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	default:
		return false, fmt.Errorf(
			"inspect verification process group %d: %w", processGroupID, err,
		)
	}
}
