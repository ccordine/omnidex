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
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/webresearch"
)

const (
	maxObjectiveRoleplayResearchParagraphs     = 4
	maxObjectiveRoleplayResearchParagraphBytes = 2 * 1024
	objectiveRoleplayResearchModelCalls        = 3
)

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
	identityGuard := &webModelIdentityGuard{}
	stations, err := newRoutedWebEvidenceStations(func(id station.ID) webresearch.PortableRuntime {
		return runtimeWebPortableRuntime(r, id, identityGuard)
	})
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	return resolveObjectiveRoleplayResearch(
		ctx, authority, research, narrative, r.svc.webSearch,
		stations.terms, stations.relevance,
		portableObjectiveRoleplayGroundedStation{runtime: r},
	)
}

func resolveObjectiveRoleplayResearch(
	ctx context.Context,
	authority turnAuthority,
	research roleplay.ResearchTurnAuthority,
	narrative roleplay.NarrativeSimulationProjection,
	acquisition webresearch.Acquisition,
	terms webresearch.SearchTermsStation,
	relevance webresearch.RelevanceStation,
	response objectiveRoleplayGroundedStation,
) (objectiveRoleplayResearchAnswer, error) {
	if ctx == nil || acquisition == nil || terms == nil || relevance == nil || response == nil {
		return objectiveRoleplayResearchAnswer{}, fmt.Errorf(
			"roleplay research requires context, code-owned acquisition, search terms, relevance, and one response station",
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
	gathered, err := webresearch.GatherRelevantEvidence(
		ctx,
		webresearch.EvidenceRequest{
			ID:       webresearch.ObjectiveID(objectiveTurnID(authority, assemblyline.ObjectiveKindExternalAnswer)),
			Question: research.Question, Context: assemblyline.CloneObjectiveContext(authority.Context),
			// An empty initial query intentionally requires the bounded terms-only
			// station before code invokes fixed acquisition providers.
			InitialQuery: "",
		},
		objectiveWebEvidenceConfig(), acquisition, terms, relevance,
	)
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	if gathered.SearchTermsCalls != 1 || gathered.RelevanceCalls != 1 {
		return objectiveRoleplayResearchAnswer{}, fmt.Errorf(
			"roleplay research evidence sieve requires one search-term and one relevance call; received %d and %d",
			gathered.SearchTermsCalls, gathered.RelevanceCalls,
		)
	}
	projected, err := projectObjectiveRoleplayResearchEvidence(gathered.Evidence, gathered.Projected)
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	capsules := make([]assemblyline.GroundedEvidenceCapsule, len(projected))
	for index, item := range projected {
		capsules[index] = item.Capsule
	}
	input := assemblyline.RoleplayGroundedResponseInput{
		ExactQuestion: research.Question,
		RoleplayIdentity: assemblyline.RoleplayResponseIdentity{
			CharacterName: narrative.Viewpoint.Name,
			Summary:       narrative.Viewpoint.Summary,
			Voice:         narrative.Viewpoint.Voice,
		},
		RoleplayUserTurn: assemblyline.RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionCommand,
		},
		Context:           assemblyline.CloneObjectiveContext(authority.Context),
		RealWorldEvidence: capsules,
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
		paragraphs, gathered.Evidence,
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
		EvidenceIDs: ids,
		ModelCalls:  gathered.SearchTermsCalls + gathered.RelevanceCalls + receipt.Calls,
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

func projectObjectiveRoleplayResearchEvidence(
	evidence []webresearch.Evidence,
	projection []webresearch.ProjectedEvidence,
) ([]objectiveEvidence, error) {
	byID := make(map[webresearch.EvidenceID]webresearch.Evidence, len(evidence))
	for _, item := range evidence {
		byID[item.ID] = item
	}
	projected := make([]objectiveEvidence, len(projection))
	for index, bounded := range projection {
		item, exists := byID[bounded.EvidenceID]
		if !exists || item.CandidateID != bounded.CandidateID {
			return nil, fmt.Errorf("roleplay research bounded evidence lost exact acquisition authority")
		}
		text, capsuleTruncated, err := boundedObjectiveEvidenceText(
			maxObjectiveEvidenceTextBytes, bounded.Title, bounded.Snippet, bounded.Content,
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
		projected[index].Truncated = item.Truncated || bounded.Truncated || capsuleTruncated
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
