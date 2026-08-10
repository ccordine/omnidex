package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func (r *nativeRuntimeV3) runMemoryReview() error {
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	verification, err := r.readVerificationArtifact()
	if err != nil {
		return err
	}
	if verification.Verdict != artifacts.VerificationVerdictPass || verificationHasBlockingFindings(verification) {
		summary := "memory review suppressed: independent verification did not pass cleanly"
		r.svc.emitStepEvent(r.claim.Authority, "memory_review_suppressed", "verdict="+safeLine(verification.Verdict, "missing"))
		return r.complete("memory_review", summary, summary)
	}
	if _, directCoding := buildV3CodingCoordinatorPlan(intent); directCoding {
		candidates, err := r.svc.repo.ListCurrentMemoryCandidates(r.ctx, r.claim.Job.ID, "candidate", 1)
		if err != nil {
			return err
		}
		if len(candidates) > 0 {
			return fmt.Errorf("deterministic coding route produced an unauthorized memory candidate %d", candidates[0].ID)
		}
		summary := "coding memory review: no memory candidates permitted"
		r.svc.emitStepEvent(r.claim.Authority, "coding_memory_absent", "candidates=0 model_calls=0")
		return r.complete("memory_review", summary, summary)
	}
	candidates, err := r.svc.repo.ListCurrentMemoryCandidates(r.ctx, r.claim.Job.ID, "candidate", 24)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return r.complete("memory_review", "memory review: no pending candidates", "memory review: no pending candidates")
	}
	promoted := make([]string, 0, len(candidates))
	rejected := make([]string, 0, len(candidates))
	tags := memoryScopeTags(r.claim.Job, splitCSVTags(r.contexts["tags"]))
	for _, candidate := range candidates {
		decision := reviewMemoryCandidate(candidate)
		if decision == model.MemoryCandidateStatusRejected {
			rejected = append(rejected, candidate.Content)
			if err := r.svc.repo.RejectCurrentMemoryCandidateByStepAttempt(r.ctx, r.claim.Authority, candidate); err != nil {
				return fmt.Errorf("reject memory candidate %d: %w", candidate.ID, err)
			}
			continue
		}
		embed, err := r.svc.llm.Embedding(r.ctx, candidate.Content)
		if err != nil {
			return fmt.Errorf("embed approved memory candidate %d: %w", candidate.ID, err)
		}
		trustTag := model.MemoryTrustTagApproved
		if decision == model.MemoryCandidateStatusDurable {
			trustTag = model.MemoryTrustTagDurable
		}
		enrichedTags := appendUnique(tags, candidate.CandidateKind, "reviewed", trustTag)
		if _, err := r.svc.repo.PromoteCurrentMemoryCandidateByStepAttempt(r.ctx, r.claim.Authority, queue.MemoryCandidatePromotion{
			Candidate: candidate,
			Tier:      decision,
			Tags:      enrichedTags,
			Embedding: embed,
		}); err != nil {
			return fmt.Errorf("promote memory candidate %d: %w", candidate.ID, err)
		}
		promoted = append(promoted, candidate.Content)
	}
	summary := strings.Join([]string{fmt.Sprintf("promoted=%d", len(promoted)), fmt.Sprintf("rejected=%d", len(rejected))}, "\n")
	return r.complete("memory_review", summary, summary)
}

func (r *nativeRuntimeV3) runFinalize() error {
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	draft, err := r.readResponseDraftArtifact()
	if err != nil {
		return err
	}
	verificationArtifact, err := r.readVerificationArtifact()
	if err != nil {
		return err
	}
	records, err := r.svc.repo.ListCurrentEvidenceByJob(r.ctx, r.claim.Job.ID, 256)
	if err != nil {
		return fmt.Errorf("list evidence for finalization: %w", err)
	}
	final := strings.TrimSpace(draft.Response)
	if final == "" {
		return fmt.Errorf("v3 finalization requires a non-empty response draft")
	}
	if err := validateV3Finalization(intent, verificationArtifact, records, final); err != nil {
		return err
	}
	return r.complete("response", final, final)
}
