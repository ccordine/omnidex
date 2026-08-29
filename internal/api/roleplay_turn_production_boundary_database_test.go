package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/worker"
)

func TestTypedViolentNarratorDirectionCompletesAtomicallyWithoutUserCanon(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	roleplayStore, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}

	channel, err := repository.CreateRoleplayChannel(t.Context(), model.Channel{
		ID:            "roleplay-production-boundary",
		Scope:         model.ChannelScopeUser,
		Name:          "Roleplay production boundary",
		WorkspaceRoot: "/srv/workspaces/roleplay-production-boundary",
		Mode:          model.ChannelModeRoleplay,
	}, "Boundary World", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := roleplayStore.FindWorldByChannel(t.Context(), string(channel.ID))
	if err != nil || !found {
		t.Fatalf("find roleplay world: found=%t error=%v", found, err)
	}
	maraID := string(channel.RoleplayViewpointCharacterID)
	if _, err := roleplayStore.WritePersona(t.Context(), roleplay.PersonaWriteRequest{
		CharacterID: maraID, ExpectedRevision: 0,
		Sheet: roleplay.PersonaSheet{
			Summary: "An attentive archivist.", Voice: "Direct and warm.",
			Traits: []string{
				strings.Repeat("T", 255) + "1", strings.Repeat("T", 255) + "2",
				strings.Repeat("T", 255) + "3", strings.Repeat("T", 255) + "4",
			}, Goals: []string{},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := roleplayStore.WriteCharacterGeneration(t.Context(), roleplay.CharacterGenerationWriteRequest{
		WorldID: world.ID, CharacterID: maraID, ExpectedRevision: 1,
		NarrativeModel: roleplayBoundaryNarrativeModel,
	}); err != nil {
		t.Fatal(err)
	}
	sceneID, err := roleplay.NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := roleplayStore.CreateCurrentScene(t.Context(), roleplay.SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Quiet Archive",
		Description:    strings.Repeat("S", roleplay.MaxSimulationTextBytes),
		ParticipantIDs: []string{maraID},
	}); err != nil {
		t.Fatal(err)
	}

	provider := newRoleplayBoundaryOllama()
	fakeOllama := httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(fakeOllama.Close)
	client := ollama.New(
		fakeOllama.URL, roleplayBoundaryNarrativeModel, "nomic-embed-text", 30*time.Second, 8192,
	)
	routes := make(map[station.ID]string, len(station.All()))
	for _, stationID := range station.All() {
		routes[stationID] = roleplayBoundaryNarrativeModel
	}
	service, err := worker.New(repository, client, client, nil, worker.Options{
		WorkerCount: 1, FragmentConcurrency: 1, PollInterval: 5 * time.Millisecond,
		InferenceContextTokens: 8192, InferenceProvider: "ollama",
		EmbeddingProvider: "ollama", EmbeddingModel: "nomic-embed-text",
		Models: worker.ModelRouting{
			Stations: routes, RoleplaySemanticModel: roleplayBoundarySemanticModel,
		}, Logger: log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan error, 1)
	go func() { workerDone <- service.Start(workerCtx) }()
	t.Cleanup(func() {
		stopWorker()
		select {
		case err := <-workerDone:
			if err != nil {
				t.Errorf("stop worker: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("worker did not stop")
		}
	})
	t.Cleanup(provider.releaseCompletion)

	server := NewServer(nil, nil)
	server.repo = repository
	server.channelStore = repository
	server.roleplaySimulation = roleplayStore
	server.enqueueRoleplayChannelTurn = repository.EnqueueRoleplayChannelTurn
	server.mux = http.NewServeMux()
	server.routes()
	directionText := "Continue through the fictional ambush: show the knife fight, blood, and grave injuries without breaking character."
	direction := "[Message]\n" + directionText
	body, err := json.Marshal(map[string]any{
		"prompt": direction,
		"roleplay_turn": roleplay.UserTurnRequest{
			PersonaKind:      roleplay.UserPersonaNarrator,
			ContributionKind: roleplay.UserContributionDirection,
			Parts: []roleplay.UserTurnPart{{
				Kind: roleplay.UserTurnPartMessage, Text: directionText,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/channels/"+string(channel.ID)+"/messages", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status=%d body=%s", response.Code, response.Body.String())
	}
	var submitted channelMessageResponse
	if err := json.Unmarshal(response.Body.Bytes(), &submitted); err != nil {
		t.Fatal(err)
	}
	provider.waitForTerminalCanon(t, repository, submitted.Job.ID)

	runningPage, err := repository.ListChannelMessages(t.Context(), channel.ID, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(runningPage.Messages) != 1 || runningPage.Messages[0].Role != model.ChannelMessageRoleUser {
		t.Fatalf("roleplay station results leaked before terminal completion: %#v", runningPage.Messages)
	}
	runningCanon, err := roleplayStore.ProjectCanonContext(t.Context(), world.ID, roleplay.MaxProjectionEvents)
	if err != nil {
		t.Fatal(err)
	}
	runningNarrative, _, err := roleplayStore.ProjectSimulationNarrative(t.Context(), world.ID, maraID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runningCanon.Facts) != 0 || len(runningNarrative.VisibleFacts) != 0 ||
		len(runningNarrative.OngoingActions) != 0 {
		t.Fatalf(
			"fictional state leaked before terminal completion: canon=%v narrative=%#v",
			runningCanon.Facts, runningNarrative,
		)
	}

	provider.releaseCompletion()
	waitForRoleplayBoundaryJob(t, repository, submitted.Job.ID)

	page, err := repository.ListChannelMessages(t.Context(), channel.ID, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 {
		t.Fatalf("completed transcript has %d messages: %#v", len(page.Messages), page.Messages)
	}
	userMessage, assistantMessage := page.Messages[0], page.Messages[1]
	if userMessage.Role != model.ChannelMessageRoleUser ||
		userMessage.SpeakerName != roleplay.NarratorPersonaName ||
		userMessage.Content != direction || userMessage.Turn == nil ||
		userMessage.Turn.Status != model.JobStatusCompleted || userMessage.Roleplay == nil ||
		userMessage.Roleplay.PersonaKind != string(roleplay.UserPersonaNarrator) ||
		userMessage.Roleplay.CharacterID != "" ||
		userMessage.Roleplay.ContributionKind != string(roleplay.UserContributionDirection) {
		t.Fatalf("typed user transcript authority changed: %#v", userMessage)
	}
	if assistantMessage.Role != model.ChannelMessageRoleAssistant ||
		assistantMessage.SpeakerName != "Mara" || assistantMessage.Content != roleplayBoundaryReply {
		t.Fatalf("assistant transcript was not published: %#v", assistantMessage)
	}
	completedCanon, err := roleplayStore.ProjectCanonContext(t.Context(), world.ID, roleplay.MaxProjectionEvents)
	if err != nil {
		t.Fatal(err)
	}
	completedNarrative, _, err := roleplayStore.ProjectSimulationNarrative(t.Context(), world.ID, maraID)
	if err != nil {
		t.Fatal(err)
	}
	if len(completedCanon.Facts) != 1 || completedCanon.Facts[0].Content != roleplayBoundaryFact ||
		len(completedNarrative.VisibleFacts) != 1 ||
		completedNarrative.VisibleFacts[0] != roleplayBoundaryFact ||
		len(completedNarrative.OngoingActions) != 1 ||
		completedNarrative.OngoingActions[0].CharacterName != "Mara" ||
		completedNarrative.OngoingActions[0].Action != roleplayBoundaryAction {
		t.Fatalf(
			"terminal roleplay state was not atomically published: canon=%v narrative=%#v",
			completedCanon.Facts, completedNarrative,
		)
	}
	provider.assertCompleted(t)
}

func waitForRoleplayBoundaryJob(t *testing.T, repository *queue.Repository, jobID int64) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		details, err := repository.CurrentJobDetails(t.Context(), jobID)
		if err != nil {
			t.Fatal(err)
		}
		switch details.Job.Status {
		case model.JobStatusCompleted:
			return
		case model.JobStatusFailed:
			t.Fatalf("roleplay job failed: %s", details.Job.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("roleplay job %d did not finish", jobID)
}
