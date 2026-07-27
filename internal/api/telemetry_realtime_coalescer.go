package api

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

type telemetryRealtimeCoalescer struct {
	mu         sync.Mutex
	window     time.Duration
	publish    func(telemetryNotifyPayload)
	timer      *time.Timer
	generation uint64
	running    bool
	dirty      bool
	stopped    bool
	trigger    telemetryNotifyPayload
}

func newTelemetryRealtimeCoalescer(window time.Duration, publish func(telemetryNotifyPayload)) *telemetryRealtimeCoalescer {
	if window <= 0 {
		panic("telemetry realtime coalescer window must be positive")
	}
	if publish == nil {
		panic("telemetry realtime publish callback is required")
	}
	return &telemetryRealtimeCoalescer{window: window, publish: publish}
}

func (c *telemetryRealtimeCoalescer) Signal(trigger telemetryNotifyPayload) error {
	trigger.EventType = strings.TrimSpace(trigger.EventType)
	if trigger.EventType == "" {
		return fmt.Errorf("telemetry realtime signal requires an event type")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return fmt.Errorf("telemetry realtime coalescer is stopped")
	}
	if !c.dirty || !queue.IsTelemetryStruggleEvent(c.trigger.EventType) || queue.IsTelemetryStruggleEvent(trigger.EventType) {
		c.trigger = trigger
	}
	c.dirty = true
	if c.timer == nil && !c.running {
		c.scheduleLocked()
	}
	return nil
}

func (c *telemetryRealtimeCoalescer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	c.dirty = false
	c.generation++
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

func (c *telemetryRealtimeCoalescer) scheduleLocked() {
	c.generation++
	generation := c.generation
	c.timer = time.AfterFunc(c.window, func() {
		c.flush(generation)
	})
}

func (c *telemetryRealtimeCoalescer) flush(generation uint64) {
	c.mu.Lock()
	if c.stopped || c.generation != generation || !c.dirty || c.running {
		c.mu.Unlock()
		return
	}
	trigger := c.trigger
	c.timer = nil
	c.running = true
	c.dirty = false
	c.mu.Unlock()

	c.publish(trigger)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	if c.stopped {
		return
	}
	if c.dirty {
		c.scheduleLocked()
	}
}
