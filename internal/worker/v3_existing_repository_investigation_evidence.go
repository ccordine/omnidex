package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/evidence"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func (session *directCodingSession) openExistingRepositoryEvidenceNeeds(
	requirements []string,
) error {
	if session == nil || session.runtime == nil {
		return fmt.Errorf("open repository evidence needs requires runtime authority")
	}
	for index, requirement := range requirements {
		need, err := assemblyline.NewApplicationRepositoryChangeOwnerNeed(index+1, requirement)
		if err != nil {
			return err
		}
		session.runtime.svc.emitStepEvent(
			session.runtime.claim.Authority,
			"application_evidence_need_opened",
			fmt.Sprintf("need=%s source=repository stop=%s", need.ID, need.StopCondition),
		)
	}
	return nil
}

func (session *directCodingSession) recordExistingRepositoryInvestigation(
	acquisition existingRepositoryEvidenceAcquisition,
) error {
	if err := acquisition.Need.Validate(); err != nil {
		return err
	}
	if err := acquisition.Pack.ValidateForRequest(
		repositoryretrieval.OperationSemanticExcerpts, acquisition.Query,
	); err != nil {
		return fmt.Errorf("record repository investigation: %w", err)
	}
	if session == nil || session.runtime == nil {
		return fmt.Errorf("record repository investigation requires runtime authority")
	}
	return session.runtime.writeEvidence(evidence.Record{
		Kind:       evidence.KindRepositoryEvidence,
		SourceType: "repository",
		SourceRef:  acquisition.Pack.ID,
		Summary: fmt.Sprintf(
			"Repository evidence need %s acquired %d symbols and %d relations.",
			acquisition.Need.ID, len(acquisition.Pack.Symbols), len(acquisition.Pack.Relations),
		),
		Confidence: 1,
		Metadata: map[string]any{
			"application_evidence_need": acquisition.Need,
			"snapshot_id":               acquisition.Pack.SnapshotID,
			"analysis_id":               acquisition.Pack.AnalysisID,
			"operation":                 repositoryretrieval.OperationSemanticExcerpts,
			"code_owned_query":          acquisition.Query,
			"pack_bytes":                evidencePackBytes(acquisition.Pack),
		},
	})
}

func (session *directCodingSession) recordExistingRepositoryInvestigationResolution(
	resolution existingRepositoryRequirementResolution,
) error {
	if err := resolution.Acquisition.Need.Validate(); err != nil {
		return err
	}
	surfaceInput := assemblyline.RepositoryChangeSurfaceInput{
		ResearchNeed: resolution.Acquisition.RequirementQuote,
		Requirements: []string{resolution.Acquisition.RequirementQuote},
		Evidence:     resolution.Acquisition.Pack,
	}
	unresolved, err := resolution.Surface.UnresolvedRequirements(surfaceInput)
	if err != nil {
		return err
	}
	if len(resolution.Surface.Targets) == 0 || len(unresolved) != 0 {
		return fmt.Errorf(
			"application evidence need %q is not resolved",
			resolution.Acquisition.Need.ID,
		)
	}
	targets := make([]string, len(resolution.Surface.Targets))
	for index, target := range resolution.Surface.Targets {
		targets[index] = target.SymbolID
	}
	if err := session.runtime.writeEvidence(evidence.Record{
		Kind:       evidence.KindModelJudgment,
		SourceType: "repository_change_surface",
		SourceRef:  resolution.Acquisition.Pack.ID,
		Summary: fmt.Sprintf(
			"Repository evidence need %s resolved to %d current owning symbols.",
			resolution.Acquisition.Need.ID, len(targets),
		),
		Confidence: 1,
		Metadata: map[string]any{
			"application_evidence_need": resolution.Acquisition.Need,
			"target_symbol_ids":         targets,
			"evidence_pack_id":          resolution.Acquisition.Pack.ID,
		},
	}); err != nil {
		return err
	}
	session.runtime.svc.emitStepEvent(
		session.runtime.claim.Authority,
		"application_evidence_need_resolved",
		fmt.Sprintf(
			"need=%s facts=%d stop=%s",
			resolution.Acquisition.Need.ID, len(targets),
			resolution.Acquisition.Need.StopCondition,
		),
	)
	return nil
}
