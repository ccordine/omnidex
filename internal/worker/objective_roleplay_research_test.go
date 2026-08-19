package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/websearch"
)

func TestResolveObjectiveRoleplayResearchUsesOneResponseCallAcrossUnrelatedFixtures(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name, question, url, title, content, answer string
	}{
		{
			name: "astronomy", question: "How long is a Martian year?",
			url: "https://science.example/mars-year", title: "Mars year",
			content: "A Martian year lasts about 687 Earth days.",
			answer:  "From this observatory, I'd reckon Mars takes about 687 Earth days to circle the Sun.",
		},
		{
			name: "baking", question: "Why is steam useful when bread first enters an oven?",
			url: "https://baking.example/steam", title: "Steam and bread",
			content: "Steam delays crust setting and supports early oven spring.",
			answer:  "I'd use steam early: it delays the crust setting and gives the loaf room for oven spring.",
		},
	}
	for index, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			authority, research, narrative := objectiveRoleplayResearchFixture(index, fixture.question)
			candidate, document := objectiveRoleplayResearchDocumentFixture(t, fixture.url, fixture.title, fixture.content)
			acquisition := &scriptedObjectiveRoleplayAcquisition{
				question: fixture.question, candidates: []websearch.Candidate{candidate},
				documents: []websearch.Document{document},
			}
			station := &scriptedObjectiveRoleplayGroundedStation{answer: fixture.answer}
			result, err := resolveObjectiveRoleplayResearch(
				context.Background(), authority, research, narrative, acquisition, station,
			)
			if err != nil {
				t.Fatal(err)
			}
			if acquisition.discoverCalls != 1 || acquisition.fetchCalls != 1 || station.calls != 1 ||
				result.ModelCalls != 1 {
				t.Fatalf("calls discover=%d fetch=%d response=%d result=%d",
					acquisition.discoverCalls, acquisition.fetchCalls, station.calls, result.ModelCalls)
			}
			if !strings.Contains(result.Rendered, fixture.url) || !strings.Contains(result.Rendered, "[1]") ||
				result.Text != fixture.answer || result.Research.Question != fixture.question ||
				len(result.Evidence) != 1 || result.Evidence[0].SourceSHA256 != document.ContentSHA256 {
				t.Fatalf("grounded result lost code-owned provenance: %#v", result)
			}
			if !reflect.DeepEqual(acquisition.selected, []websearch.CandidateID{candidate.ID}) {
				t.Fatalf("deterministic selection = %v, want %v", acquisition.selected, []websearch.CandidateID{candidate.ID})
			}
			job, err := assemblyline.NewRoleplayGroundedResponseJob(station.input)
			if err != nil {
				t.Fatal(err)
			}
			renderedPrompt, _, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(renderedPrompt, authority.Instruction) || strings.Contains(renderedPrompt, "/research") {
				t.Fatalf("rendered grounded prompt leaked external command bytes: %s", renderedPrompt)
			}
		})
	}
}

func TestObjectiveRoleplayResearchRejectsCharacterAndNamespaceMismatchBeforeAcquisition(t *testing.T) {
	t.Parallel()
	authority, research, narrative := objectiveRoleplayResearchFixture(0, "How long is a Martian year?")
	candidate, document := objectiveRoleplayResearchDocumentFixture(
		t, "https://science.example/mars-year", "Mars year", "A Martian year lasts about 687 Earth days.",
	)
	for _, mutate := range []func(*turnAuthority){
		func(value *turnAuthority) {
			value.RoleplayViewpointCharacterID = model.RoleplayCharacterID("rpc_" + strings.Repeat("9", 32))
		},
		func(value *turnAuthority) { value.ChannelMode = model.ChannelModeAssistant },
		func(value *turnAuthority) { value.RoleplayInputKind = roleplay.SimulationTurnProse },
	} {
		changed := authority
		mutate(&changed)
		acquisition := &scriptedObjectiveRoleplayAcquisition{
			question: research.Question, candidates: []websearch.Candidate{candidate}, documents: []websearch.Document{document},
		}
		_, err := resolveObjectiveRoleplayResearch(
			context.Background(), changed, research, narrative, acquisition,
			&scriptedObjectiveRoleplayGroundedStation{answer: "unused"},
		)
		if err == nil || acquisition.discoverCalls != 0 || acquisition.fetchCalls != 0 {
			t.Fatalf("mismatched authority reached acquisition: err=%v discover=%d fetch=%d",
				err, acquisition.discoverCalls, acquisition.fetchCalls)
		}
	}
}

type scriptedObjectiveRoleplayAcquisition struct {
	question                  string
	candidates                []websearch.Candidate
	documents                 []websearch.Document
	selected                  []websearch.CandidateID
	discoverCalls, fetchCalls int
}

func (*scriptedObjectiveRoleplayAcquisition) Limits() websearch.AcquisitionLimits {
	return websearch.AcquisitionLimits{MaxDocuments: 2}
}

func (fixture *scriptedObjectiveRoleplayAcquisition) Discover(
	_ context.Context,
	request websearch.QueryRequest,
) (websearch.CandidateReport, error) {
	fixture.discoverCalls++
	if request.Query != fixture.question {
		return websearch.CandidateReport{}, fmt.Errorf("query changed")
	}
	return websearch.CandidateReport{
		Query: request.Query, Candidates: append([]websearch.Candidate(nil), fixture.candidates...),
	}, nil
}

func (fixture *scriptedObjectiveRoleplayAcquisition) Fetch(
	_ context.Context,
	request websearch.FetchRequest,
) (websearch.DocumentReport, error) {
	fixture.fetchCalls++
	fixture.selected = append([]websearch.CandidateID(nil), request.CandidateIDs...)
	return websearch.DocumentReport{Documents: append([]websearch.Document(nil), fixture.documents...)}, nil
}

type scriptedObjectiveRoleplayGroundedStation struct {
	answer string
	calls  int
	input  assemblyline.RoleplayGroundedResponseInput
}

func (station *scriptedObjectiveRoleplayGroundedStation) RespondGrounded(
	_ context.Context,
	input assemblyline.RoleplayGroundedResponseInput,
) (assemblyline.RoleplayGroundedResponseDecision, objectiveStationReceipt, error) {
	station.calls++
	station.input = input
	if len(input.RealWorldEvidence) != 1 || strings.Contains(input.ExactQuestion, "/research") {
		return assemblyline.RoleplayGroundedResponseDecision{}, objectiveStationReceipt{}, fmt.Errorf("unsafe response input")
	}
	return assemblyline.RoleplayGroundedResponseDecision{
		Schema: assemblyline.RoleplayGroundedResponseSchemaV1,
		Paragraphs: []assemblyline.RoleplayGroundedParagraph{{
			Text: station.answer, EvidenceIDs: []string{input.RealWorldEvidence[0].ID},
		}},
	}, objectiveStationReceipt{Calls: 1}, nil
}

func objectiveRoleplayResearchFixture(
	index int,
	question string,
) (turnAuthority, roleplay.ResearchTurnAuthority, roleplay.NarrativeSimulationProjection) {
	suffix := fmt.Sprintf("%032x", index+1)
	worldID := "rpw_" + suffix
	sceneID := "rps_" + suffix
	characterID := "rpc_" + suffix
	preparationID := "rpt_" + suffix
	grantID := "rpg_" + suffix
	fingerprint := strings.Repeat(fmt.Sprintf("%x", index+1), 64)
	if len(fingerprint) > 64 {
		fingerprint = fingerprint[:64]
	}
	questionDigest := sha256.Sum256([]byte(question))
	research := roleplay.ResearchTurnAuthority{
		Schema:        roleplay.ResearchTurnAuthoritySchemaV1,
		PreparationID: preparationID, ChannelID: "roleplay-channel", UserMessageID: int64(index + 1),
		WorldID: worldID, SceneID: sceneID, SceneRevision: 1, CharacterID: characterID,
		Capability: roleplay.CapabilityWebResearch, CapabilityGrantID: grantID,
		Question: question, QuestionSHA256: hex.EncodeToString(questionDigest[:]),
		NarrativeFingerprint: fingerprint, Authority: roleplay.AuthorityRealWorld,
		CapabilityIssuedAt: time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC),
		CreatedAt:          time.Date(2026, 8, 18, 12, 1, 0, 0, time.UTC),
	}
	authority := turnAuthority{
		JobID: int64(index + 1), Pipeline: model.PipelineChat,
		Instruction: "/research " + fmt.Sprintf("%q", question),
		ChannelID:   model.ChannelID(research.ChannelID), ChannelMode: model.ChannelModeRoleplay,
		RoleplayViewpointCharacterID:    model.RoleplayCharacterID(characterID),
		RoleplaySimulationPreparationID: preparationID,
		RoleplayWorldID:                 worldID, RoleplaySceneID: sceneID, RoleplaySceneRevision: 1,
		RoleplayInputKind:               roleplay.SimulationTurnExternalCommand,
		RoleplayParticipantCharacterIDs: []model.RoleplayCharacterID{model.RoleplayCharacterID(characterID)},
		RoleplayNarrativeFingerprint:    fingerprint,
	}
	narrative := roleplay.NarrativeSimulationProjection{
		Schema:       roleplay.NarrativeSimulationProjectionSchemaV1,
		Scene:        roleplay.NarrativeScene{Title: "Observatory kitchen", Description: "A quiet room.", ActiveCharacterName: "Ada"},
		Participants: []string{"Ada"},
		Viewpoint: roleplay.NarrativePersona{
			Name: "Ada", Summary: "A careful scholar.", Voice: "Measured",
			Traits: []string{"Curious"}, Goals: []string{"Explain clearly"},
		},
	}
	return authority, research, narrative
}

func objectiveRoleplayResearchDocumentFixture(
	t *testing.T,
	rawURL, title, content string,
) (websearch.Candidate, websearch.Document) {
	t.Helper()
	candidateID, err := websearch.CandidateIDForURL(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	contentDigest := sha256.Sum256([]byte(content))
	contentSHA := hex.EncodeToString(contentDigest[:])
	documentDigest := sha256.Sum256([]byte("web-document.v1\x00" + rawURL + "\x00" + contentSHA))
	candidate := websearch.Candidate{
		ID: candidateID, URL: rawURL, Title: title, Snippet: content,
		Sources: []websearch.CandidateSource{{
			Provider: websearch.ProviderGoogle, SearchURL: "https://www.google.com/search?q=fixture", Rank: 1,
		}},
	}
	document := websearch.Document{
		ID:          websearch.DocumentID("document_" + hex.EncodeToString(documentDigest[:])),
		CandidateID: candidateID, URL: rawURL, Title: title, Snippet: content, Content: content,
		ContentSHA256: contentSHA, ObservedAt: time.Date(2026, 8, 18, 12, 2, 0, 0, time.UTC),
	}
	return candidate, document
}
