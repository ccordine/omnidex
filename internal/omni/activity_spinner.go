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
	started time.Time
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
	started time.Time
}

func startActivityIndicator(out io.Writer, label string) *activityIndicator {
	indicator := &activityIndicator{out: out, label: label, enabled: true, started: time.Now()}
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
	if i.started.IsZero() {
		i.started = time.Now()
	}
	i.spinner = startActivitySpinner(i.out, i.label, i.started)
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

func startActivitySpinner(out io.Writer, label string, started time.Time) *activitySpinner {
	spinner := &activitySpinner{
		out:     out,
		label:   label,
		started: started,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
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
			fmt.Fprintf(s.out, "\r%s %s %s Esc to interrupt", s.label, frames[i%len(frames)], formatActivityElapsed(time.Since(s.started)))
			i++
		}
	}
}

func formatActivityElapsed(elapsed time.Duration) string {
	if elapsed < 0 {
		elapsed = 0
	}
	totalSeconds := int(elapsed.Round(time.Second).Seconds())
	minutes := totalSeconds / 60
	seconds := totalSeconds % 60
	if minutes > 0 {
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}
