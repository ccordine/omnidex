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
	"github.com/gryph/omnidex/internal/webresearch"
	"github.com/gryph/omnidex/internal/websearch"
)

func TestResolveObjectiveRoleplayResearchUsesSharedEvidenceSieveAndOneResponseAcrossUnrelatedFixtures(t *testing.T) {
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
			irrelevantCandidate, irrelevantDocument := objectiveRoleplayResearchDocumentFixture(
				t, fmt.Sprintf("https://noise.example/%d", index), "Unrelated result", "Steam and flour notes unrelated to this question.",
			)
			candidate, document := objectiveRoleplayResearchDocumentFixture(
				t, fixture.url, fixture.title, fixture.content,
			)
			acquisition := &scriptedObjectiveRoleplayAcquisition{
				query: fixture.question, candidates: []websearch.Candidate{irrelevantCandidate, candidate},
				documents: []websearch.Document{irrelevantDocument, document},
			}
			relevance := &scriptedObjectiveRoleplayRelevanceStation{selected: []websearch.CandidateID{candidate.ID}}
			station := &scriptedObjectiveRoleplayGroundedStation{answer: fixture.answer}
			result, err := resolveObjectiveRoleplayResearch(
				context.Background(), authority, research, narrative, acquisition,
				relevance, station,
			)
			if err != nil {
				t.Fatal(err)
			}
			if acquisition.discoverCalls != 1 || acquisition.fetchCalls != 1 ||
				relevance.calls != 1 || station.calls != 1 ||
				result.ModelCalls != 3 {
				t.Fatalf("calls discover=%d fetch=%d relevance=%d response=%d result=%d",
					acquisition.discoverCalls, acquisition.fetchCalls,
					relevance.calls, station.calls, result.ModelCalls)
			}
			if relevance.question != fixture.question ||
				station.input.ExactQuestion != fixture.question {
				t.Fatalf(
					"dedicated research question changed: relevance=%q response=%q",
					relevance.question, station.input.ExactQuestion,
				)
			}
			if !strings.Contains(result.Rendered, fixture.url) || !strings.Contains(result.Rendered, "[1]") ||
				result.Text != fixture.answer || result.Research.Question != fixture.question ||
				len(result.Evidence) != 1 || result.Evidence[0].SourceSHA256 != document.ContentSHA256 {
				t.Fatalf("grounded result lost code-owned provenance: %#v", result)
			}
			if !reflect.DeepEqual(acquisition.selected, []websearch.CandidateID{irrelevantCandidate.ID, candidate.ID}) {
				t.Fatalf("code-owned fetch set = %v", acquisition.selected)
			}
			if len(station.input.RealWorldEvidence) != 1 ||
				station.input.RealWorldEvidence[0].ID != result.Evidence[0].Capsule.ID ||
				station.input.RoleplayIdentity.CharacterName != narrative.Viewpoint.Name ||
				!reflect.DeepEqual(station.input.Context, authority.Context) {
				t.Fatalf("final roleplay research projection is not minimal and selected: %#v", station.input)
			}
			job, err := assemblyline.NewRoleplayGroundedParagraphInventoryJob(station.input)
			if err != nil {
				t.Fatal(err)
			}
			renderedPrompt, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{
				authority.Instruction, "/research", narrative.Scene.Description,
				"fictional_narrative_state", "Unrelated result", "Steam and flour notes",
				"Curious", "Explain clearly",
			} {
				if strings.Contains(renderedPrompt, forbidden) {
					t.Fatalf("rendered grounded prompt leaked %q: %s", forbidden, renderedPrompt)
				}
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
			query: "Mars orbital period", candidates: []websearch.Candidate{candidate}, documents: []websearch.Document{document},
		}
		relevance := &scriptedObjectiveRoleplayRelevanceStation{selected: []websearch.CandidateID{candidate.ID}}
		_, err := resolveObjectiveRoleplayResearch(
			context.Background(), changed, research, narrative, acquisition,
			relevance,
			&scriptedObjectiveRoleplayGroundedStation{answer: "unused"},
		)
		if err == nil || acquisition.discoverCalls != 0 || acquisition.fetchCalls != 0 ||
			relevance.calls != 0 {
			t.Fatalf("mismatched authority reached sieve: err=%v discover=%d fetch=%d relevance=%d",
				err, acquisition.discoverCalls, acquisition.fetchCalls, relevance.calls)
		}
	}
}

func TestObjectiveRoleplayResearchResponseRequiresAggregateReuseProof(t *testing.T) {
	t.Parallel()
	fixtures := []struct {
		name       string
		receipt    objectiveStationReceipt
		wantError  bool
		modelCalls int
	}{
		{name: "fully restored", receipt: objectiveStationReceipt{Reused: true}, modelCalls: 1},
		{name: "zero without reuse proof", receipt: objectiveStationReceipt{}, wantError: true},
		{name: "fresh calls marked reused", receipt: objectiveStationReceipt{Calls: 1, Reused: true}, wantError: true},
	}
	for index, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			question := "Which mineral dominates this sample?"
			authority, research, narrative := objectiveRoleplayResearchFixture(index+10, question)
			candidate, document := objectiveRoleplayResearchDocumentFixture(
				t, "https://geology.example/sample", "Sample analysis",
				"The sample is dominated by quartz.",
			)
			acquisition := &scriptedObjectiveRoleplayAcquisition{
				query: question, candidates: []websearch.Candidate{candidate},
				documents: []websearch.Document{document},
			}
			relevance := &scriptedObjectiveRoleplayRelevanceStation{
				selected: []websearch.CandidateID{candidate.ID},
			}
			response := &scriptedObjectiveRoleplayGroundedStation{
				answer:  "The sample is dominated by quartz.",
				receipt: fixture.receipt, useReceipt: true,
			}
			result, err := resolveObjectiveRoleplayResearch(
				context.Background(), authority, research, narrative,
				acquisition, relevance, response,
			)
			if fixture.wantError {
				if err == nil || !strings.Contains(err.Error(), "roleplay research response") {
					t.Fatalf("receipt=%+v error=%v", fixture.receipt, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.ModelCalls != fixture.modelCalls {
				t.Fatalf("model calls=%d want %d", result.ModelCalls, fixture.modelCalls)
			}
		})
	}
}

func TestObjectiveRoleplayResearchAcceptsFullyRestoredWebLedgerThroughTurnBoundary(t *testing.T) {
	t.Parallel()
	question := "Which mineral dominates this restored sample?"
	authority, research, narrative := objectiveRoleplayResearchFixture(40, question)
	candidate, document := objectiveRoleplayResearchDocumentFixture(
		t, "https://geology.example/restored-sample", "Restored sample analysis",
		"The restored sample is dominated by quartz.",
	)
	answer, err := resolveObjectiveRoleplayResearch(
		t.Context(), authority, research, narrative,
		&scriptedObjectiveRoleplayAcquisition{
			query: question, candidates: []websearch.Candidate{candidate},
			documents: []websearch.Document{document},
		},
		&scriptedObjectiveRoleplayRelevanceStation{
			selected:   []websearch.CandidateID{candidate.ID},
			receipt:    webresearch.SemanticCallReceipt{Reused: true},
			useReceipt: true,
		},
		&scriptedObjectiveRoleplayGroundedStation{
			answer:  "The restored sample is dominated by quartz.",
			receipt: objectiveStationReceipt{Reused: true}, useReceipt: true,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := answer.WebCallLedger.ValidateForMaximum(
		"restored roleplay research", maximumObjectiveRoleplayResearchModelCalls,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runObjectiveRoleplayResearchTurn(
		t.Context(), authority,
		func(context.Context, turnAuthority) (objectiveRoleplayResearchAnswer, error) {
			return answer, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if answer.ModelCalls != 0 || !receipt.Reused || !result.Complete ||
		result.ModelCalls != 0 || result.Output != answer.Rendered {
		t.Fatalf("receipt=%+v answer=%+v result=%+v", receipt, answer, result)
	}
}

func TestObjectiveRoleplayResearchRejectsForgedRelevanceReceipts(t *testing.T) {
	t.Parallel()
	for _, receipt := range []webresearch.SemanticCallReceipt{
		{},
		{Calls: 1, Reused: true},
	} {
		receipt := receipt
		t.Run(fmt.Sprintf("calls_%d_reused_%t", receipt.Calls, receipt.Reused), func(t *testing.T) {
			t.Parallel()
			question := "Which mineral dominates this sample?"
			authority, research, narrative := objectiveRoleplayResearchFixture(41, question)
			candidate, document := objectiveRoleplayResearchDocumentFixture(
				t, "https://geology.example/forged-sample", "Sample analysis",
				"The sample is dominated by quartz.",
			)
			_, err := resolveObjectiveRoleplayResearch(
				t.Context(), authority, research, narrative,
				&scriptedObjectiveRoleplayAcquisition{
					query: question, candidates: []websearch.Candidate{candidate},
					documents: []websearch.Document{document},
				},
				&scriptedObjectiveRoleplayRelevanceStation{
					selected: []websearch.CandidateID{candidate.ID},
					receipt:  receipt, useReceipt: true,
				},
				&scriptedObjectiveRoleplayGroundedStation{answer: "unused"},
			)
			if err == nil {
				t.Fatalf("forged relevance receipt %+v was accepted", receipt)
			}
		})
	}
}

func TestRoleplayResearchPropagatesSecondProjectionTruncationAuthority(t *testing.T) {
	item := objectiveWebEvidenceFixture(
		t, "https://science.example/large-roleplay-source", "Large roleplay source",
		strings.Repeat("Bounded real-world evidence. ", 160),
	)
	projected, err := projectObjectiveRoleplayResearchEvidence(
		[]webresearch.Evidence{item},
		[]webresearch.ProjectedEvidence{{
			EvidenceID: item.ID, CandidateID: item.CandidateID,
			Title: item.Title, Snippet: item.Snippet, Content: item.Content,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || !projected[0].Truncated ||
		!strings.HasSuffix(projected[0].Capsule.Text, objectiveEvidenceTruncationMarker) {
		t.Fatalf("roleplay second projection truncation authority was lost: %#v", projected)
	}
}

type scriptedObjectiveRoleplayAcquisition struct {
	query                     string
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
	if request.Query != fixture.query {
		return websearch.CandidateReport{}, fmt.Errorf("query changed")
	}
	return websearch.CandidateReport{
		Query: request.Query, Candidates: append([]websearch.Candidate(nil), fixture.candidates...),
		Diagnostics: []websearch.ProviderDiagnostic{{
			Provider: websearch.ProviderGoogle, Outcome: websearch.DiscoverySucceeded,
			CandidateCount: len(fixture.candidates),
		}},
	}, nil
}

func (fixture *scriptedObjectiveRoleplayAcquisition) Fetch(
	_ context.Context,
	request websearch.FetchRequest,
) (websearch.DocumentReport, error) {
	fixture.fetchCalls++
	fixture.selected = append([]websearch.CandidateID(nil), request.CandidateIDs...)
	diagnostics := make([]websearch.DocumentDiagnostic, len(fixture.documents))
	for index, document := range fixture.documents {
		diagnostics[index] = websearch.DocumentDiagnostic{
			CandidateID: document.CandidateID, URL: document.URL, Outcome: websearch.FetchSucceeded,
		}
	}
	return websearch.DocumentReport{
		Documents: append([]websearch.Document(nil), fixture.documents...), Diagnostics: diagnostics,
	}, nil
}

type scriptedObjectiveRoleplayRelevanceStation struct {
	selected   []websearch.CandidateID
	question   string
	receipt    webresearch.SemanticCallReceipt
	useReceipt bool
	calls      int
}

func (station *scriptedObjectiveRoleplayRelevanceStation) Select(
	_ context.Context,
	call webresearch.RelevanceCall,
) (webresearch.RelevanceDecision, error) {
	station.calls++
	station.question = call.Question
	if call.Question == "" || len(call.Candidates) == 0 {
		return webresearch.RelevanceDecision{}, fmt.Errorf("relevance received invalid authority")
	}
	receipt := webresearch.SemanticCallReceipt{Calls: 1}
	if station.useReceipt {
		receipt = station.receipt
	}
	var ledger webresearch.SemanticCallLedger
	if err := ledger.Record(
		"scripted roleplay relevance",
		receipt,
		1,
	); err != nil {
		return webresearch.RelevanceDecision{}, err
	}
	return webresearch.RelevanceDecision{
		Outcome:       webresearch.RelevanceSelected,
		CandidateIDs:  append([]websearch.CandidateID(nil), station.selected...),
		SemanticCalls: receipt.Calls,
		CallLedger:    ledger,
	}, nil
}

type scriptedObjectiveRoleplayGroundedStation struct {
	answer     string
	receipt    objectiveStationReceipt
	useReceipt bool
	calls      int
	input      assemblyline.RoleplayGroundedResponseInput
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
	receipt := objectiveStationReceipt{Calls: 2}
	if station.useReceipt {
		receipt = station.receipt
	}
	return assemblyline.RoleplayGroundedResponseDecision{
		Schema: assemblyline.RoleplayGroundedResponseSchemaV1,
		Paragraphs: []assemblyline.RoleplayGroundedParagraph{{
			Text: station.answer, EvidenceIDs: []string{input.RealWorldEvidence[0].ID},
		}},
	}, receipt, nil
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
		Instruction:      "/research " + fmt.Sprintf("%q", question),
		ModelInstruction: question, ModelRedactedInstruction: question,
		ModelArtifactPaths: []string{},
		ChannelID:          model.ChannelID(research.ChannelID), ChannelMode: model.ChannelModeRoleplay,
		RoleplayViewpointCharacterID:    model.RoleplayCharacterID(characterID),
		RoleplaySimulationPreparationID: preparationID,
		RoleplayWorldID:                 worldID, RoleplaySceneID: sceneID, RoleplaySceneRevision: 1,
		RoleplayInputKind:               roleplay.SimulationTurnExternalCommand,
		RoleplayParticipantCharacterIDs: []model.RoleplayCharacterID{model.RoleplayCharacterID(characterID)},
		RoleplayNarrativeFingerprint:    fingerprint,
		RoleplayGenerationConfig: func() *roleplay.CharacterGenerationConfig {
			config := roleplayGenerationFixture(suffix)
			return &config
		}(),
	}
	authority.RoleplayResponders = []roleplay.SimulationResponderRoute{{
		Position: 0, CharacterID: characterID,
		GenerationConfig:     *authority.RoleplayGenerationConfig,
		NarrativeFingerprint: fingerprint,
	}}
	contextSource := "The active viewpoint is Ada in the observatory."
	contextText := "Ada is answering from the observatory."
	authority.Context = assemblyline.ObjectiveContext{Capsules: []assemblyline.ObjectiveContextCapsule{{
		Sources: []assemblyline.ObjectiveContextSource{{
			Namespace: "roleplay_scene", CandidateID: "CTX_1",
			ContentSHA256: assemblyline.ExactObjectiveContextSHA(contextSource),
		}},
		Content: contextText, ContentSHA256: assemblyline.ExactObjectiveContextSHA(contextText),
	}}}
	narrative := roleplay.NarrativeSimulationProjection{
		Schema: roleplay.NarrativeSimulationProjectionSchemaV1,
		Scene: roleplay.NarrativeScene{
			Title:               "Observatory kitchen",
			Description:         "The unrelated crown archive remains locked beneath the northern tower.",
			ActiveCharacterName: "Ada",
			Initiative: roleplay.SimulationInitiativeClock{
				Round: 1, Turn: 1, FictionalTimeTick: 0,
			},
		},
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
