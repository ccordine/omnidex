package api

import (
	"context"
	"log"
	"net/http"
	"sort"
	"time"
)

const scrumAutoWorkScanInterval = 10 * time.Second

type scrumAutoWorkCandidate struct {
	projectID int64
	cardID    string
	queuedAt  time.Time
}

// StartScrumAutoWorkLoop keeps server-side auto-work moving even when no board is open.
func (s *Server) StartScrumAutoWorkLoop(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	go func() {
		s.RefreshScrumAutoWorkAsync()
		ticker := time.NewTicker(scrumAutoWorkScanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.RefreshScrumAutoWorkAsync()
			}
		}
	}()
}

// RefreshScrumAutoWorkAsync reconciles all project boards and starts the next global auto-work item.
func (s *Server) RefreshScrumAutoWorkAsync() {
	if s == nil || s.repo == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), scrumPlayAutoRunTimeout)
		defer cancel()
		if err := s.refreshScrumAutoWork(ctx); err != nil {
			log.Printf("scrum global auto-work refresh: %v", err)
		}
	}()
}

func (s *Server) refreshScrumAutoWork(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}
	if paused, err := s.repo.IsAIPaused(ctx); err != nil {
		return err
	} else if paused {
		return nil
	}
	s.scrumAutoWorkMu.Lock()
	defer s.scrumAutoWorkMu.Unlock()

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
	r := (&http.Request{}).WithContext(ctx)
	for _, candidate := range candidates {
		if err := s.refreshScrumPlayQueueForProject(ctx, candidate.projectID, "global auto-work"); err != nil {
			log.Printf("scrum global auto-work project=%d card=%s: %v", candidate.projectID, candidate.cardID, err)
			continue
		}
		if running, err := s.repo.HasRunningScrumPlay(ctx); err != nil {
			return err
		} else if running {
			return nil
		}
		if board, err := s.scrumBoardFromProject(ctx, candidate.projectID); err == nil {
			if refreshed, err := s.kickoffAutoPlayThrough(r, candidate.projectID, board); err == nil && s.findRunningScrumCard(refreshed) != nil {
				return nil
			}
		}
	}
	return nil
}

func (s *Server) refreshRunningScrumPlayProjects(ctx context.Context) error {
	projectIDs, err := s.repo.ListRunningScrumPlayProjectIDs(ctx)
	if err != nil {
		return err
	}
	for _, projectID := range projectIDs {
		if err := s.refreshScrumPlayQueueForProject(ctx, projectID, "global running reconcile"); err != nil {
			log.Printf("scrum global running reconcile project=%d: %v", projectID, err)
		}
	}
	return nil
}

func (s *Server) globalScrumAutoWorkCandidates(ctx context.Context) ([]scrumAutoWorkCandidate, error) {
	candidates := []scrumAutoWorkCandidate{}
	const pageSize = 250
	for offset := 0; ; offset += pageSize {
		projects, err := s.repo.ListProjects(ctx, pageSize, offset)
		if err != nil {
			return nil, err
		}
		for _, project := range projects {
			autoWork := s.scrumAutoWorkConfig(ctx, project.ID)
			if !autoWork.Enabled {
				continue
			}
			board, err := s.scrumBoardFromProject(ctx, project.ID)
			if err != nil {
				log.Printf("scrum global auto-work load project=%d: %v", project.ID, err)
				continue
			}
			if s.findRunningScrumCard(board) != nil {
				continue
			}
			reviewCfg := s.scrumAutoReviewConfig(ctx, project.ID)
			if scrumAutoPlayThroughCompleteWithReview(board, reviewCfg.Enabled) {
				continue
			}
			next := s.nextAutoWorkScrumCard(board, autoWork)
			if next == nil {
				continue
			}
			candidates = append(candidates, scrumAutoWorkCandidate{
				projectID: project.ID,
				cardID:    next.ID,
				queuedAt:  scrumAutoWorkQueuedAt(*next),
			})
		}
		if len(projects) < pageSize {
			break
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].queuedAt.Equal(candidates[j].queuedAt) {
			if candidates[i].projectID == candidates[j].projectID {
				return candidates[i].cardID < candidates[j].cardID
			}
			return candidates[i].projectID < candidates[j].projectID
		}
		return candidates[i].queuedAt.Before(candidates[j].queuedAt)
	})
	return candidates, nil
}

func scrumAutoWorkQueuedAt(card ScrumCard) time.Time {
	for _, raw := range []string{card.UpdatedAt, card.CreatedAt} {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func (s *Server) scrumGlobalPlayActive(ctx context.Context) bool {
	if s == nil || s.repo == nil {
		return false
	}
	running, err := s.repo.HasRunningScrumPlay(ctx)
	if err != nil {
		log.Printf("scrum global running check: %v", err)
		return true
	}
	return running
}
