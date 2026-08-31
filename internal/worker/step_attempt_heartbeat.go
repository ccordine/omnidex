package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const (
	stepAttemptRenewInterval      = 25 * time.Second
	stepAttemptExpirySafetyMargin = 5 * time.Second
)

var errStepExecutionAuthorityLost = errors.New("step execution authority was lost")

func (s *Service) watchStepAttemptLease(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	leaseDeadline time.Time,
) (context.Context, func() error) {
	leaseCtx, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	fenceDuration, err := stepAttemptFenceDuration(leaseDeadline)
	if err != nil {
		cancel()
		result <- err
		return leaseCtx, func() error { return <-result }
	}
	renewAfter := stepAttemptRenewDelay(fenceDuration)
	expired := make(chan struct{})
	var expireOnce sync.Once
	deadline := time.AfterFunc(fenceDuration, func() {
		expireOnce.Do(func() {
			close(expired)
			cancel()
		})
	})
	renewal := time.NewTimer(renewAfter)
	go func() {
		defer cancel()
		defer deadline.Stop()
		defer renewal.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				select {
				case <-expired:
					result <- fmt.Errorf(
						"%w: exact step attempt reached its database lease fence",
						errStepExecutionAuthorityLost,
					)
				default:
					result <- nil
				}
				return
			case <-renewal.C:
				renewedDeadline, err := s.repo.RenewStepAttempt(leaseCtx, authority)
				if err != nil {
					select {
					case <-expired:
						result <- fmt.Errorf(
							"%w: exact step attempt reached its database lease fence: %v",
							errStepExecutionAuthorityLost, err,
						)
					default:
						result <- fmt.Errorf(
							"%w: renew exact step attempt: %v", errStepExecutionAuthorityLost, err,
						)
					}
					return
				}
				fenceDuration, err = stepAttemptFenceDuration(renewedDeadline)
				if err != nil {
					result <- err
					return
				}
				if !deadline.Reset(fenceDuration) {
					deadline.Stop()
					result <- fmt.Errorf(
						"%w: exact step attempt reached its database lease fence during renewal",
						errStepExecutionAuthorityLost,
					)
					return
				}
				renewal.Reset(stepAttemptRenewDelay(fenceDuration))
			}
		}
	}()
	return leaseCtx, func() error {
		cancel()
		return <-result
	}
}

func stepAttemptFenceDuration(leaseDeadline time.Time) (time.Duration, error) {
	if leaseDeadline.IsZero() {
		return 0, fmt.Errorf(
			"%w: exact step attempt lease deadline is unavailable",
			errStepExecutionAuthorityLost,
		)
	}
	leaseValidFor := time.Until(leaseDeadline)
	if leaseValidFor <= stepAttemptExpirySafetyMargin {
		return 0, fmt.Errorf(
			"%w: exact step attempt lease has only %s remaining; more than %s is required",
			errStepExecutionAuthorityLost, leaseValidFor, stepAttemptExpirySafetyMargin,
		)
	}
	return leaseValidFor - stepAttemptExpirySafetyMargin, nil
}

func stepAttemptRenewDelay(fenceDuration time.Duration) time.Duration {
	if fenceDuration/2 < stepAttemptRenewInterval {
		return fenceDuration / 2
	}
	return stepAttemptRenewInterval
}

func (s *Service) finishStepAttemptWatch(
	workErr, leaseErr error,
) error {
	if workErr == nil {
		if leaseErr != nil {
			s.logf("step operation committed before authority watcher stopped: %v", leaseErr)
		}
		return nil
	}
	if leaseErr != nil {
		return errors.Join(workErr, leaseErr)
	}
	return workErr
}
