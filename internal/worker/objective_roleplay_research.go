package worker

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/webresearch"
)

const (
	maxObjectiveRoleplayResearchParagraphs     = 4
	maxObjectiveRoleplayResearchParagraphBytes = 2 * 1024
	maxObjectiveRoleplayEvidenceSemanticCalls  = maxObjectiveWebRelevantCandidates * exactSemanticLeafCalls
	maximumObjectiveRoleplayResearchModelCalls = maxObjectiveRoleplayEvidenceSemanticCalls +
		(1+maxObjectiveRoleplayResearchParagraphs*(maxObjectiveWebRelevantCandidates+2))*exactSemanticLeafCalls
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
	stations, err := newRoutedWebEvidenceStations(func(id station.ID) webresearch.PortableRuntime {
		return runtimeWebPortableRuntime(r, id)
	})
	if err != nil {
		return objectiveRoleplayResearchAnswer{}, err
	}
	return resolveObjectiveRoleplayResearch(
		ctx, authority, research, narrative, r.svc.webSearch,
		func(ctx context.Context, acquired webresearch.AcquiredEvidence) (queue.WebEvidenceRecord, error) {
			return r.svc.repo.RecordWebEvidence(ctx, authority.JobID, acquired)
		},
		stations.relevance,
		portableObjectiveRoleplayGroundedStation{runtime: r},
	)
}

func resolveObjectiveRoleplayResearch(
	ctx context.Context,
	authority turnAuthority,
	research roleplay.ResearchTurnAuthority,
	narrative roleplay.NarrativeSimulationProjection,
	acquisition webresearch.Acquisition,
	recordEvidence func(context.Context, webresearch.AcquiredEvidence) (queue.WebEvidenceRecord, error),
	relevance webresearch.RelevanceStation,
	response objectiveRoleplayGroundedStation,
) (objectiveRoleplayResearchAnswer, error) {
	result := objectiveRoleplayResearchAnswer{}
	if ctx == nil || acquisition == nil || recordEvidence == nil || relevance == nil || response == nil {
		return result, fmt.Errorf(
			"roleplay research requires context, code-owned acquisition, relevance, and one response station",
		)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := validateObjectiveRoleplayResearchTurn(authority, research); err != nil {
		return result, err
	}
	if err := roleplay.ValidateResearchNarrativeProjection(narrative); err != nil {
		return result, err
	}
	gathered, err := webresearch.GatherRelevantEvidence(
		ctx,
		webresearch.EvidenceRequest{
			ID:       webresearch.ObjectiveID(objectiveTurnID(authority)),
			Question: authority.ModelInstruction, Context: assemblyline.CloneObjectiveContext(authority.Context),
			InitialQuery:       research.Question,
			KnownArtifactPaths: append([]string{}, authority.ModelArtifactPaths...),
		},
		objectiveWebEvidenceConfig(), acquisition, relevance,
	)
	result.ModelCalls = gathered.SemanticCalls
	if err != nil {
		return result, err
	}
	if gathered.SemanticCalls < 0 || gathered.SemanticCalls > maxObjectiveRoleplayEvidenceSemanticCalls {
		return result, fmt.Errorf(
			"roleplay research evidence sieve reported %d calls outside 0..%d",
			gathered.SemanticCalls, maxObjectiveRoleplayEvidenceSemanticCalls,
		)
	}
	acquired, err := webresearch.CaptureEvidence(
		gathered, objectiveWebEvidenceConfig().MaxProjectionBytes, authority.ModelArtifactPaths,
	)
	if err != nil {
		return result, err
	}
	sources, err := recordEvidence(ctx, acquired)
	if err != nil {
		return result, err
	}
	if sources.JobID != authority.JobID {
		return result, fmt.Errorf("recorded web evidence belongs to a different job")
	}
	projected, err := projectObjectiveRoleplayResearchEvidence(sources)
	if err != nil {
		return result, err
	}
	capsules := make([]assemblyline.GroundedEvidenceCapsule, len(projected))
	for index, item := range projected {
		capsules[index] = item.Capsule
	}
	input := assemblyline.RoleplayGroundedResponseInput{
		ExactQuestion: authority.ModelInstruction,
		RoleplayIdentity: assemblyline.RoleplayResponseIdentity{
			CharacterName: narrative.Viewpoint.Name,
			Summary:       narrative.Viewpoint.Summary,
			Voice:         narrative.Viewpoint.Voice,
		},
		RoleplayUserTurn: assemblyline.RoleplayUserTurnProjection{
			PersonaKind: roleplay.UserPersonaNarrator, PersonaName: roleplay.NarratorPersonaName,
			ContributionKind: roleplay.UserContributionCommand,
		},
		Context:            assemblyline.CloneObjectiveContext(authority.Context),
		RealWorldEvidence:  capsules,
		KnownArtifactPaths: append([]string{}, authority.ModelArtifactPaths...),
	}
	decision, dispatches, err := response.RespondGrounded(ctx, input)
	result.ModelCalls += dispatches
	if err != nil {
		return result, err
	}
	maximumResponseCalls := (1 + maxObjectiveRoleplayResearchParagraphs*(len(capsules)+2)) *
		exactSemanticLeafCalls
	if dispatches < 0 || dispatches > maximumResponseCalls {
		return result, fmt.Errorf("roleplay research response reported %d calls outside 0..%d", dispatches, maximumResponseCalls)
	}
	if err := decision.ValidateFor(input); err != nil {
		return result, err
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
		return result, err
	}
	result.Research, result.Artifact, result.Sources = research, artifact, sources
	return result, nil
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
		!slices.Contains(authority.RoleplayParticipantCharacterIDs, model.RoleplayCharacterID(research.CharacterID)) {
		return fmt.Errorf("roleplay research differs from the claimed prepared active-character turn")
	}
	return nil
}
