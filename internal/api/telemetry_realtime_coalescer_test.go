package api

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestServerRejectsTelemetryCoalescerWithoutLifecycleContext(t *testing.T) {
	server := &Server{}
	if _, err := server.ensureTelemetryRealtimeCoalescer(); !errors.Is(err, ErrRealtimeLifecycleUnavailable) {
		t.Fatalf("ensureTelemetryRealtimeCoalescer() error=%v want ErrRealtimeLifecycleUnavailable", err)
	}
}

func TestTelemetryRealtimeCoalescerBatchesMetricsQueriesAndRetainsUrgentTrigger(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	var received telemetryNotifyPayload
	done := make(chan struct{}, 1)
	coalescer := newTelemetryRealtimeCoalescer(15*time.Millisecond, func(payload telemetryNotifyPayload) {
		mu.Lock()
		calls++
		received = payload
		mu.Unlock()
		done <- struct{}{}
	})
	defer coalescer.Stop()

	for index := 0; index < 50; index++ {
		if err := coalescer.Signal(telemetryNotifyPayload{EventType: "step_output"}); err != nil {
			t.Fatal(err)
		}
	}
	urgent := "verify_test_fail"
	if err := coalescer.Signal(telemetryNotifyPayload{EventType: urgent, Message: "verification failed"}); err != nil {
		t.Fatal(err)
	}
	if err := coalescer.Signal(telemetryNotifyPayload{EventType: "step_output"}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("metrics refresh did not flush")
	}
	time.Sleep(30 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if calls != 1 || received.EventType != urgent {
		t.Fatalf("calls=%d trigger=%+v", calls, received)
	}
}

func TestTelemetryRealtimeCoalescerRejectsSignalsAfterStop(t *testing.T) {
	coalescer := newTelemetryRealtimeCoalescer(time.Second, func(telemetryNotifyPayload) {})
	coalescer.Stop()
	if err := coalescer.Signal(telemetryNotifyPayload{EventType: "step_output"}); err == nil {
		t.Fatal("expected stopped coalescer to reject signal")
	}
}
