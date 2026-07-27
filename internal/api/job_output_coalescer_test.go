package api

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestJobOutputCoalescerRejectsSignalsAfterStop(t *testing.T) {
	coalescer := newJobOutputCoalescer(time.Second, func(int64) {})
	coalescer.Stop()
	if err := coalescer.Signal(1); !errors.Is(err, ErrJobOutputCoalescerStopped) {
		t.Fatalf("Signal() error=%v want ErrJobOutputCoalescerStopped", err)
	}
}

func TestServerRejectsJobOutputCoalescerWithoutLifecycleContext(t *testing.T) {
	server := &Server{}
	if _, err := server.ensureJobOutputCoalescer(); !errors.Is(err, ErrRealtimeLifecycleUnavailable) {
		t.Fatalf("ensureJobOutputCoalescer() error=%v want ErrRealtimeLifecycleUnavailable", err)
	}
}

func TestJobOutputCoalescerCollapsesRapidSignals(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	done := make(chan struct{}, 1)
	coalescer := newJobOutputCoalescer(15*time.Millisecond, func(jobID int64) {
		if jobID != 42 {
			t.Errorf("jobID=%d want 42", jobID)
		}
		mu.Lock()
		calls++
		mu.Unlock()
		done <- struct{}{}
	})

	for index := 0; index < 100; index++ {
		coalescer.Signal(42)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("coalesced output was not flushed")
	}
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
}

func TestJobOutputCoalescerFlushNowCancelsPendingTimer(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	coalescer := newJobOutputCoalescer(time.Second, func(jobID int64) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	coalescer.Signal(7)
	coalescer.FlushNow(7)
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
}

func TestPublishJobProgressUsesTypedJobsTopic(t *testing.T) {
	server := &Server{realtimeHub: NewRealtimeHub()}
	subscription, err := server.realtimeHub.Subscribe([]string{realtimeTopicJobs}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Unsubscribe()

	server.publishJobProgress(9, "output", "Agent produced output")

	var message realtimeMessage
	select {
	case raw := <-subscription.Messages:
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("decode realtime message: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for job progress")
	}
	if message.EventName != "job-progress" || message.JobID != 9 || message.Phase != "output" {
		t.Fatalf("unexpected message: %+v", message)
	}
}
