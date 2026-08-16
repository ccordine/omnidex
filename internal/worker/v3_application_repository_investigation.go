package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/evidence"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func (session *directCodingSession) resolveApplicationRepositoryEvidenceNeed(
	need assemblyline.ApplicationEvidenceNeed,
) ([]assemblyline.ApplicationContextEvidence, error) {
	if err := need.Validate(); err != nil {
		return nil, err
	}
	if need.Kind != assemblyline.ApplicationEvidenceContextFact ||
		len(need.SourceClasses) != 1 ||
		need.SourceClasses[0] != assemblyline.ApplicationEvidenceRepository {
		return nil, fmt.Errorf("application evidence need %q has no repository context resolver", need.ID)
	}
	if session == nil || session.runtime == nil || session.repositoryIndex == nil {
		return nil, fmt.Errorf("application repository investigation requires immutable repository authority")
	}
	session.runtime.svc.emitStepEvent(
		session.runtime.claim.Authority,
		"application_evidence_need_opened",
		fmt.Sprintf("need=%s source=repository stop=%s", need.ID, need.StopCondition),
	)
	resolvedNeed := need
	resolvedNeed.SearchAnchors = []string{need.Question}
	acquisition, err := acquireObjectiveRepositoryEvidenceClosure(
		need.Question,
		func(query string) (repositoryretrieval.EvidencePack, error) {
			pack, buildErr := session.buildExistingRepositoryEvidence(query)
			if buildErr != nil {
				return repositoryretrieval.EvidencePack{}, buildErr
			}
			if recordErr := session.recordExistingRepositoryEvidence(query, pack); recordErr != nil {
				return repositoryretrieval.EvidencePack{}, recordErr
			}
			return pack, nil
		},
		func(unresolved string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			decision, receipt, callErr := session.runtime.resolveObjectiveRepositorySearchTerm(
				session.runtime.ctx, unresolved,
			)
			if callErr == nil {
				resolvedNeed.SearchAnchors = append([]string(nil), decision.Anchors...)
			}
			return decision, receipt, callErr
		},
		func(question string, candidates []objectiveEvidence) (
			assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error,
		) {
			return session.runtime.resolveObjectiveRepositoryRelevance(
				session.runtime.ctx, question, candidates,
			)
		},
	)
	if err != nil {
		return nil, err
	}
	if err := resolvedNeed.Validate(); err != nil {
		return nil, err
	}
	contextEvidence := make([]assemblyline.ApplicationContextEvidence, len(acquisition.Evidence))
	for index, selected := range acquisition.Evidence {
		value := strings.TrimSpace(selected.Capsule.Text)
		if value == "" || len(value) > assemblyline.MaxApplicationContextFactBytes {
			return nil, fmt.Errorf(
				"application evidence need %q selected source %q outside the compact fact boundary",
				need.ID, selected.SourceRef,
			)
		}
		contextEvidence[index] = assemblyline.ApplicationContextEvidence{
			Value: value, SourceID: selected.SourceRef,
			SourceSHA256: assemblyline.ExactObjectiveContextSHA(value),
		}
		if err := session.recordApplicationContextFact(resolvedNeed, selected, value); err != nil {
			return nil, err
		}
	}
	session.runtime.svc.emitStepEvent(
		session.runtime.claim.Authority,
		"application_evidence_need_resolved",
		fmt.Sprintf("need=%s facts=%d stop=%s", need.ID, len(contextEvidence), need.StopCondition),
	)
	return contextEvidence, nil
}

func (session *directCodingSession) recordApplicationContextFact(
	need assemblyline.ApplicationEvidenceNeed,
	selected objectiveEvidence,
	value string,
) error {
	if session == nil || session.runtime == nil {
		return fmt.Errorf("record application context fact requires runtime authority")
	}
	return session.runtime.writeEvidence(evidence.Record{
		Kind:       evidence.KindRepositoryEvidence,
		SourceType: selected.SourceType,
		SourceRef:  selected.SourceRef,
		Summary:    value,
		Hash:       assemblyline.ExactObjectiveContextSHA(value),
		Confidence: 1,
		Metadata: map[string]any{
			"application_evidence_need": need,
			"selected_evidence_id":      selected.Capsule.ID,
		},
	})
}
