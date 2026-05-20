package omni

import (
	"fmt"
	"io"
	"sync"
	"time"
)

type activitySpinner struct {
	out     io.Writer
	label   string
	stopCh  chan struct{}
	doneCh  chan struct{}
	stopMux sync.Once
}

type activityIndicator struct {
	out     io.Writer
	label   string
	enabled bool
	mu      sync.Mutex
	spinner *activitySpinner
}

func startActivityIndicator(out io.Writer, label string) *activityIndicator {
	indicator := &activityIndicator{out: out, label: label, enabled: true}
	indicator.Resume()
	return indicator
}

func (i *activityIndicator) Pause() {
	if i == nil {
		return
	}
	i.mu.Lock()
	spinner := i.spinner
	i.spinner = nil
	i.mu.Unlock()
	if spinner != nil {
		spinner.Stop()
	}
}

func (i *activityIndicator) Resume() {
	if i == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if !i.enabled || i.spinner != nil {
		return
	}
	i.spinner = startActivitySpinner(i.out, i.label)
}

func (i *activityIndicator) Stop() {
	if i == nil {
		return
	}
	i.mu.Lock()
	i.enabled = false
	spinner := i.spinner
	i.spinner = nil
	i.mu.Unlock()
	if spinner != nil {
		spinner.Stop()
	}
}

func startActivitySpinner(out io.Writer, label string) *activitySpinner {
	spinner := &activitySpinner{
		out:    out,
		label:  label,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	go spinner.run()
	return spinner
}

func (s *activitySpinner) Stop() {
	s.stopMux.Do(func() {
		close(s.stopCh)
		<-s.doneCh
	})
}

func (s *activitySpinner) run() {
	defer close(s.doneCh)

	frames := []string{"|", "/", "-", "\\"}
	ticker := time.NewTicker(120 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-s.stopCh:
			fmt.Fprint(s.out, "\r\033[2K")
			return
		case <-ticker.C:
			fmt.Fprintf(s.out, "\r%s %s", s.label, frames[i%len(frames)])
			i++
		}
	}
}
