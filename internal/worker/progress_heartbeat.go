package worker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	progressHeartbeatInitial  = 8 * time.Second
	progressHeartbeatInterval = 15 * time.Second
)

func (s *Service) startProgressHeartbeat(ctx context.Context, stepID int64, operation string) func() {
	if s == nil || stepID <= 0 {
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
			s.emitStepEvent(stepID, "operation_heartbeat", fmt.Sprintf("operation=%s elapsed=%s", safeLine(operation, "work"), time.Since(started).Truncate(time.Second)))
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
