package api

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrJobOutputCoalescerStopped = errors.New("job output coalescer is stopped")

type jobOutputSignal struct {
	timer      *time.Timer
	running    bool
	dirty      bool
	generation uint64
}

type jobOutputCoalescer struct {
	mu      sync.Mutex
	window  time.Duration
	flush   func(jobID int64)
	pending map[int64]*jobOutputSignal
	stopped bool
}

func newJobOutputCoalescer(window time.Duration, flush func(jobID int64)) *jobOutputCoalescer {
	if window <= 0 {
		panic("job output coalescer window must be positive")
	}
	if flush == nil {
		panic("job output coalescer flush callback is required")
	}
	return &jobOutputCoalescer{
		window:  window,
		flush:   flush,
		pending: make(map[int64]*jobOutputSignal),
	}
}

func (c *jobOutputCoalescer) Signal(jobID int64) error {
	if jobID <= 0 {
		return fmt.Errorf("job output signal requires a positive job id")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return ErrJobOutputCoalescerStopped
	}
	state := c.pending[jobID]
	if state == nil {
		state = &jobOutputSignal{}
		c.pending[jobID] = state
	}
	state.dirty = true
	if state.timer == nil && !state.running {
		c.scheduleLocked(jobID, state)
	}
	return nil
}

func (c *jobOutputCoalescer) FlushNow(jobID int64) bool {
	if jobID <= 0 {
		return false
	}
	c.mu.Lock()
	if c.stopped {
		c.mu.Unlock()
		return false
	}
	state := c.pending[jobID]
	if state == nil || (!state.dirty && !state.running) {
		c.mu.Unlock()
		return false
	}
	state.generation++
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	if state.running {
		state.dirty = true
		c.mu.Unlock()
		return true
	}
	state.running = true
	state.dirty = false
	c.mu.Unlock()

	c.flush(jobID)
	c.finish(jobID, state)
	return true
}

func (c *jobOutputCoalescer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	c.stopped = true
	for _, state := range c.pending {
		state.generation++
		if state.timer != nil {
			state.timer.Stop()
			state.timer = nil
		}
	}
	c.pending = make(map[int64]*jobOutputSignal)
}

func (c *jobOutputCoalescer) scheduleLocked(jobID int64, state *jobOutputSignal) {
	state.generation++
	generation := state.generation
	state.timer = time.AfterFunc(c.window, func() {
		c.flushScheduled(jobID, generation)
	})
}

func (c *jobOutputCoalescer) flushScheduled(jobID int64, generation uint64) {
	c.mu.Lock()
	state := c.pending[jobID]
	if c.stopped || state == nil || state.generation != generation {
		c.mu.Unlock()
		return
	}
	state.timer = nil
	if state.running || !state.dirty {
		c.mu.Unlock()
		return
	}
	state.running = true
	state.dirty = false
	c.mu.Unlock()

	c.flush(jobID)
	c.finish(jobID, state)
}

func (c *jobOutputCoalescer) finish(jobID int64, state *jobOutputSignal) {
	c.mu.Lock()
	defer c.mu.Unlock()
	current := c.pending[jobID]
	if current != state {
		return
	}
	state.running = false
	if state.dirty {
		c.scheduleLocked(jobID, state)
		return
	}
	delete(c.pending, jobID)
}
