package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const (
	progressHeartbeatInitial  = 8 * time.Second
	progressHeartbeatInterval = 15 * time.Second
)

func (s *Service) startProgressHeartbeat(ctx context.Context, authority model.StepAttemptAuthority, operation string) func() {
	if s == nil || authority.StepID <= 0 {
		return func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		initial := time.NewTimer(progressHeartbeatInitial)
		defer initial.Stop()
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-initial.C:
		}
		emit := func() {
			s.emitStepEvent(authority, "operation_heartbeat", fmt.Sprintf("operation=%s elapsed=%s", safeLine(operation, "work"), time.Since(started).Truncate(time.Second)))
		}
		emit()
		ticker := time.NewTicker(progressHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				emit()
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(stop) })
		<-done
	}
}
