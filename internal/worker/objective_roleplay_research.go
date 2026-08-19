package worker

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/webresearch"
	"github.com/gryph/omnidex/internal/websearch"
)

const (
	maxObjectiveRoleplayResearchDocuments      = 2
	maxObjectiveRoleplayResearchParagraphs     = 4
	maxObjectiveRoleplayResearchParagraphBytes = 2 * 1024
)

type objectiveRoleplayResearchAcquisition interface {
	Limits() websearch.AcquisitionLimits
	Discover(context.Context, websearch.QueryRequest) (websearch.CandidateReport, error)
	Fetch(context.Context, websearch.FetchRequest) (websearch.DocumentReport, error)
}

func (r *nativeRuntimeV3) acquireObjectiveRoleplayResearch(
	ctx context.Context,
	authority turnAuthority,
) (objectiveRoleplayResearchAnswer, error) {
	if ctx == nil || r == nil || r.svc == nil || r.svc.repo == nil ||
		r.claim == nil || r.svc.webSearch == nil {
		return objectiveRoleplayResearchAnswer{}, fmt.Errorf(
			"roleplay research requires repository and web acquisition authority",
		)
	}
	if authority.JobID != r.claim.Job.ID || authority.Instruction != r.claim.Job.Instruction {
		return objectiveRoleplayResearchAnswer{}, fmt.Errorf(
			"roleplay research authority does not match the claimed job",
		)
	}
	research, err := r.svc.repo.LoadRoleplayResearchTurn(ctx, authority.JobID)
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	if err := validateObjectiveRoleplayResearchTurn(authority, research); err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	narrative, _, err := r.svc.repo.ProjectRoleplayResearchNarrative(ctx, research, authority.JobID)
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	return resolveObjectiveRoleplayResearch(
		ctx, authority, research, narrative, r.svc.webSearch,
		portableObjectiveRoleplayGroundedStation{runtime: r},
	)
}

func resolveObjectiveRoleplayResearch(
	ctx context.Context,
	authority turnAuthority,
	research roleplay.ResearchTurnAuthority,
	narrative roleplay.NarrativeSimulationProjection,
	acquisition objectiveRoleplayResearchAcquisition,
	response objectiveRoleplayGroundedStation,
) (objectiveRoleplayResearchAnswer, error) {
	if ctx == nil || acquisition == nil || response == nil {
		return objectiveRoleplayResearchAnswer{}, fmt.Errorf(
			"roleplay research requires context, acquisition, and one response station",
		)
	}
	if err := ctx.Err(); err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	if err := validateObjectiveRoleplayResearchTurn(authority, research); err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	if err := roleplay.ValidateResearchNarrativeProjection(narrative); err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	documents, err := acquireObjectiveRoleplayResearchDocuments(ctx, acquisition, research.Question)
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	webEvidence, err := webresearch.EvidenceFromDocuments(documents)
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	projected, err := projectObjectiveRoleplayResearchEvidence(webEvidence)
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	capsules := make([]assemblyline.GroundedEvidenceCapsule, len(projected))
	for index, item := range projected {
		capsules[index] = item.Capsule
	}
	input := assemblyline.RoleplayGroundedResponseInput{
		ExactQuestion:           research.Question,
		FictionalNarrativeState: roleplay.CloneResearchNarrativeProjection(narrative),
		RealWorldEvidence:       capsules,
	}
	decision, receipt, err := response.RespondGrounded(ctx, input)
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	if receipt.Calls != 1 {
		return objectiveRoleplayResearchAnswer{}, fmt.Errorf(
			"roleplay research response requires exactly one semantic call; received %d", receipt.Calls,
		)
	}
	paragraphs := make([]webresearch.GroundedParagraph, len(decision.Paragraphs))
	for index, paragraph := range decision.Paragraphs {
		ids := make([]webresearch.EvidenceID, len(paragraph.EvidenceIDs))
		for evidenceIndex, id := range paragraph.EvidenceIDs {
			ids[evidenceIndex] = webresearch.EvidenceID(id)
		}
		paragraphs[index] = webresearch.GroundedParagraph{
			Text: paragraph.Text, EvidenceIDs: ids,
		}
	}
	artifact, err := webresearch.BuildGroundedCompletionArtifact(
		paragraphs, webEvidence,
		maxObjectiveRoleplayResearchParagraphs,
		maxObjectiveRoleplayResearchParagraphBytes,
	)
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	selected, ids, err := bindObjectiveRoleplayResearchCitations(projected, artifact)
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	plain := make([]string, len(artifact.Paragraphs))
	for index, paragraph := range artifact.Paragraphs {
		plain[index] = paragraph.Text
	}
	return objectiveRoleplayResearchAnswer{
		Research: research, Text: strings.Join(plain, "\n\n"),
		Rendered: artifact.Rendered, RenderedSHA256: artifact.SHA256,
		Paragraphs: cloneWebParagraphs(artifact.Paragraphs), Evidence: selected,
		EvidenceIDs: ids, ModelCalls: 1,
	}, nil
}

func validateObjectiveRoleplayResearchTurn(
	authority turnAuthority,
	research roleplay.ResearchTurnAuthority,
) error {
	if err := research.Validate(); err != nil {
		return err
	}
	exact := "/research " + strconv.Quote(research.Question)
	if authority.JobID < 1 || authority.ChannelMode != model.ChannelModeRoleplay ||
		authority.RoleplayInputKind != roleplay.SimulationTurnExternalCommand ||
		authority.Instruction != exact || authority.ChannelID != model.ChannelID(research.ChannelID) ||
		authority.RoleplaySimulationPreparationID != research.PreparationID ||
		authority.RoleplayWorldID != research.WorldID || authority.RoleplaySceneID != research.SceneID ||
		authority.RoleplaySceneRevision != research.SceneRevision ||
		authority.RoleplayViewpointCharacterID != model.RoleplayCharacterID(research.CharacterID) ||
		authority.RoleplayNarrativeFingerprint != research.NarrativeFingerprint ||
		!slices.Contains(authority.RoleplayParticipantCharacterIDs, model.RoleplayCharacterID(research.CharacterID)) {
		return fmt.Errorf("roleplay research differs from the claimed prepared active-character turn")
	}
	return nil
}

func acquireObjectiveRoleplayResearchDocuments(
	ctx context.Context,
	acquisition objectiveRoleplayResearchAcquisition,
	question string,
) ([]websearch.Document, error) {
	limits := acquisition.Limits()
	if limits.MaxDocuments < 1 || limits.MaxDocuments > 32 {
		return nil, fmt.Errorf("roleplay research acquisition has invalid document limits")
	}
	report, err := acquisition.Discover(ctx, websearch.QueryRequest{Query: question})
	if err != nil {
		return nil, fmt.Errorf("roleplay research discovery: %w", err)
	}
	if report.Query != question || len(report.Candidates) == 0 {
		return nil, fmt.Errorf("roleplay research discovery returned no exact-query candidates")
	}
	count := min(maxObjectiveRoleplayResearchDocuments, limits.MaxDocuments, len(report.Candidates))
	selected := make([]websearch.CandidateID, count)
	selectedSet := make(map[websearch.CandidateID]struct{}, count)
	for index := range count {
		candidate := report.Candidates[index]
		if err := websearch.ValidateCandidate(candidate); err != nil {
			return nil, fmt.Errorf("roleplay research candidate %d: %w", index, err)
		}
		if _, duplicate := selectedSet[candidate.ID]; duplicate {
			return nil, fmt.Errorf("roleplay research discovery duplicated candidate %q", candidate.ID)
		}
		selected[index] = candidate.ID
		selectedSet[candidate.ID] = struct{}{}
	}
	fetched, err := acquisition.Fetch(ctx, websearch.FetchRequest{
		Candidates:   append([]websearch.Candidate(nil), report.Candidates...),
		CandidateIDs: selected,
	})
	if err != nil {
		return nil, fmt.Errorf("roleplay research fetch: %w", err)
	}
	if len(fetched.Documents) == 0 || len(fetched.Documents) > count {
		return nil, fmt.Errorf("roleplay research fetch returned an invalid document count")
	}
	seen := make(map[websearch.CandidateID]struct{}, len(fetched.Documents))
	for index, document := range fetched.Documents {
		if _, allowed := selectedSet[document.CandidateID]; !allowed {
			return nil, fmt.Errorf("roleplay research document %d was not deterministically selected", index)
		}
		if _, duplicate := seen[document.CandidateID]; duplicate {
			return nil, fmt.Errorf("roleplay research fetch duplicated candidate %q", document.CandidateID)
		}
		if err := websearch.ValidateDocument(document); err != nil {
			return nil, fmt.Errorf("roleplay research document %d: %w", index, err)
		}
		seen[document.CandidateID] = struct{}{}
	}
	return append([]websearch.Document(nil), fetched.Documents...), nil
}

func projectObjectiveRoleplayResearchEvidence(
	evidence []webresearch.Evidence,
) ([]objectiveEvidence, error) {
	projected := make([]objectiveEvidence, len(evidence))
	for index, item := range evidence {
		text, err := boundedObjectiveEvidenceText(
			maxObjectiveEvidenceTextBytes, item.Title, item.Snippet, item.Content,
		)
		if err != nil {
			return nil, err
		}
		projected[index], err = newObjectiveEvidence(
			string(item.ID), text, "web_document", item.URL,
		)
		if err != nil {
			return nil, err
		}
		projected[index].SourceSHA256 = item.ContentSHA256
		projected[index].ObservedAt = item.ObservedAt
		projected[index].Truncated = item.Truncated
	}
	return projected, nil
}

func bindObjectiveRoleplayResearchCitations(
	projected []objectiveEvidence,
	artifact webresearch.Artifact,
) ([]objectiveEvidence, []string, error) {
	byID := make(map[string]int, len(projected))
	for index, item := range projected {
		byID[item.Capsule.ID] = index
	}
	selected := make([]objectiveEvidence, 0, len(artifact.Sources))
	ids := make([]string, 0, len(artifact.Sources))
	for _, source := range artifact.Sources {
		index, exists := byID[string(source.EvidenceID)]
		if !exists || projected[index].SourceRef != source.URL ||
			projected[index].SourceSHA256 != source.ContentSHA256 {
			return nil, nil, fmt.Errorf("roleplay research citation lost exact acquired evidence")
		}
		item := projected[index]
		for paragraphIndex, paragraph := range artifact.Paragraphs {
			if slices.Contains(paragraph.EvidenceIDs, source.EvidenceID) {
				item.ParagraphMask |= 1 << paragraphIndex
			}
		}
		if item.ParagraphMask == 0 {
			return nil, nil, fmt.Errorf("roleplay research citation has no paragraph binding")
		}
		if err := validateObjectiveEvidence(item); err != nil {
			return nil, nil, err
		}
		selected = append(selected, item)
		ids = append(ids, item.Capsule.ID)
	}
	return selected, ids, nil
}
