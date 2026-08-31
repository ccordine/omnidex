package api

import (
	"context"
	"fmt"
	"log"
	"time"
)

const scrumAutoWorkScanInterval = 10 * time.Second

type scrumAutoWorkLockHeldKey struct{}
type scrumAutoWorkHandoffSuppressedKey struct{}

type scrumAutoWorkCandidate struct {
	projectID int64
	cardID    string
	queuedAt  time.Time
}

// StartScrumAutoWorkLoop keeps server-side auto-work moving even when no board is open.
func (s *Server) StartScrumAutoWorkLoop(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("postgres repository is required to start the Scrum auto-work loop")
	}
	if ctx == nil {
		return fmt.Errorf("context is required to start the Scrum auto-work loop")
	}
	if err := s.refreshScrumAutoWorkAsync(ctx); err != nil {
		return err
	}
	go func() {
		ticker := time.NewTicker(scrumAutoWorkScanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.refreshScrumAutoWorkAsync(ctx); err != nil && ctx.Err() == nil {
					log.Printf("scrum auto-work scan rejected: %v", err)
				}
			}
		}
	}()
	return nil
}

// RefreshScrumAutoWorkAsync reconciles all project boards and starts the next global auto-work item.
func (s *Server) RefreshScrumAutoWorkAsync() error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("postgres repository is required to refresh Scrum auto-work")
	}
	if s.lifecycleContext == nil {
		return ErrRealtimeLifecycleUnavailable
	}
	return s.refreshScrumAutoWorkAsync(s.lifecycleContext)
}

func (s *Server) refreshScrumAutoWorkAsync(parent context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("postgres repository is required to schedule Scrum auto-work")
	}
	if parent == nil {
		return fmt.Errorf("parent context is required to schedule Scrum auto-work")
	}
	if err := parent.Err(); err != nil {
		return fmt.Errorf("schedule Scrum auto-work: %w", err)
	}
	s.scrumAutoWorkAsyncMu.Lock()
	if s.scrumAutoWorkAsyncRunning {
		s.scrumAutoWorkAsyncPending = true
		s.scrumAutoWorkAsyncMu.Unlock()
		return nil
	}
	s.scrumAutoWorkAsyncRunning = true
	s.scrumAutoWorkAsyncMu.Unlock()
	go func() {
		for {
			ctx, cancel := context.WithTimeout(parent, scrumPlayAutoRunTimeout)
			if err := s.refreshScrumAutoWork(ctx); err != nil && parent.Err() == nil {
				log.Printf("scrum global auto-work refresh: %v", err)
			}
			cancel()

			s.scrumAutoWorkAsyncMu.Lock()
			if s.scrumAutoWorkAsyncPending && parent.Err() == nil {
				s.scrumAutoWorkAsyncPending = false
				s.scrumAutoWorkAsyncMu.Unlock()
				continue
			}
			s.scrumAutoWorkAsyncPending = false
			s.scrumAutoWorkAsyncRunning = false
			s.scrumAutoWorkAsyncMu.Unlock()
			return
		}
	}()
	return nil
}

func (s *Server) refreshScrumAutoWork(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("postgres repository is required to refresh Scrum auto-work")
	}
	if ctx == nil {
		return fmt.Errorf("context is required to refresh Scrum auto-work")
	}
	s.scrumAutoWorkMu.Lock()
	defer s.scrumAutoWorkMu.Unlock()
	ctx = context.WithValue(ctx, scrumAutoWorkLockHeldKey{}, true)

	if running, err := s.repo.HasRunningScrumPlay(ctx); err != nil {
		return err
	} else if running {
		if err := s.refreshRunningScrumPlayProjects(ctx); err != nil {
			return err
		}
		if running, err := s.repo.HasRunningScrumPlay(ctx); err != nil {
			return err
		} else if running {
			return nil
		}
	}

	candidates, err := s.globalScrumAutoWorkCandidates(ctx)
	if err != nil {
		return err
	}
	for _, candidate := range candidates {
		if err := s.refreshScrumPlayQueueForProject(ctx, candidate.projectID, "global auto-work"); err != nil {
			return fmt.Errorf("reconcile global auto-work project=%d card=%s: %w", candidate.projectID, candidate.cardID, err)
		}
		if running, err := s.repo.HasRunningScrumPlay(ctx); err != nil {
			return err
		} else if running {
			return nil
		}
		board, err := s.scrumBoardMetadataFromProject(ctx, candidate.projectID)
		if err != nil {
			return fmt.Errorf("load global auto-work project=%d card=%s: %w", candidate.projectID, candidate.cardID, err)
		}
		r := scrumRequestForProject(ctx, candidate.projectID)
		if _, err := s.startNextScrumAutoWork(r, candidate.projectID, board); err != nil {
			return fmt.Errorf("start global auto-work project=%d card=%s: %w", candidate.projectID, candidate.cardID, err)
		}
		running, err := s.repo.HasRunningScrumPlay(ctx)
		if err != nil {
			return err
		}
		if !running {
			return fmt.Errorf("global auto-work candidate project=%d card=%s did not start a job", candidate.projectID, candidate.cardID)
		}
		return nil
	}
	return nil
}

func (s *Server) refreshRunningScrumPlayProjects(ctx context.Context) error {
	projectID, found, err := s.repo.RunningScrumPlayProjectID(ctx)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err := s.refreshScrumPlayQueueForProject(ctx, projectID, "global running reconcile"); err != nil {
		return fmt.Errorf("reconcile running Scrum project %d: %w", projectID, err)
	}
	return nil
}

func (s *Server) globalScrumAutoWorkCandidates(ctx context.Context) ([]scrumAutoWorkCandidate, error) {
	stored, found, err := s.repo.FindGlobalScrumAutoWorkCandidate(ctx)
	if err != nil || !found {
		return nil, err
	}
	return []scrumAutoWorkCandidate{{
		projectID: stored.ProjectID,
		cardID:    stored.CardID,
		queuedAt:  stored.QueuedAt,
	}}, nil
}

func scrumAutoWorkLockHeld(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	held, _ := ctx.Value(scrumAutoWorkLockHeldKey{}).(bool)
	return held
}

func scrumAutoWorkHandoffSuppressed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	suppressed, _ := ctx.Value(scrumAutoWorkHandoffSuppressedKey{}).(bool)
	return suppressed
}

func (s *Server) scrumGlobalPlayActive(ctx context.Context) (bool, error) {
	if s == nil || s.repo == nil {
		return false, fmt.Errorf("postgres repository is required to check global Scrum play")
	}
	if ctx == nil {
		return false, fmt.Errorf("context is required to check global Scrum play")
	}
	running, err := s.repo.HasRunningScrumPlay(ctx)
	if err != nil {
		return false, err
	}
	return running, nil
}
