package worker

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5/pgxpool"
)

type researchWorkflowCase struct {
	name, question string
	paragraphs     []string
}

func researchWorkflowCases() []researchWorkflowCase {
	return []researchWorkflowCase{
		{"timetable", "What does the current timetable say about the journey?", []string{
			"The train departs at nine.", "The train arrives at noon.",
			"The train leaves from platform three.", "Advance reservations are required.",
		}},
		{"catalogue", "What publication details does the catalogue record?", []string{
			"The catalogue lists the second edition.", "North Press is the publisher.",
			"The book has a cloth binding.", "The book is written in French.",
		}},
	}
}

type researchWorkflowFixture struct {
	pool       *pgxpool.Pool
	repository *queue.Repository
	claim      *model.ClaimedStep
	authority  turnAuthority
	provider   *exactEvidenceStationClient
	http       *researchHTTPFixture
}

func newResearchWorkflowFixture(t *testing.T, fixture researchWorkflowCase) researchWorkflowFixture {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for research workflow evidence")
	}
	pool, _ := freshWorkerEvidenceRepository(t, databaseURL)
	config, err := modelconfig.Freeze(modelconfig.Config{
		"conversation_response_model": "fixture-roleplay", "web_relevance_model": "fixture-web",
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := queue.New(pool, config)
	ctx := context.Background()
	channel, err := repository.CreateRoleplayChannel(ctx, model.Channel{
		ID: "research-fixture", Scope: model.ChannelScopeUser, Name: "Research fixture",
		Tags: []string{"chat"}, WorkspaceRoot: t.TempDir(), Mode: model.ChannelModeRoleplay,
	}, "Fixture world", "Observer")
	if err != nil {
		t.Fatal(err)
	}
	store, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := store.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil || !found {
		t.Fatalf("world=%+v found=%t error=%v", world, found, err)
	}
	character := string(channel.RoleplayViewpointCharacterID)
	if _, err := store.WritePersona(ctx, roleplay.PersonaWriteRequest{CharacterID: character,
		Sheet: roleplay.PersonaSheet{Summary: "An attentive observer.", Voice: "Calm, concise sentences.", Traits: []string{}, Goals: []string{}},
	}); err != nil {
		t.Fatal(err)
	}
	scene, err := roleplay.NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateCurrentScene(ctx, roleplay.SceneSetup{
		ID: scene, WorldID: world.ID, Title: "Reading room", Description: "A quiet room with a desk.", ParticipantIDs: []string{character},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ConfigureRoleplayCharacterCapability(ctx, world.ID, character, roleplay.CapabilityWebResearch, true); err != nil {
		t.Fatal(err)
	}
	_, job, err := repository.EnqueueRoleplayChannelTurn(ctx, channel.ID, "/research "+strconv.Quote(fixture.question), roleplay.UserTurnRequest{
		PersonaKind: roleplay.UserPersonaNarrator, ContributionKind: roleplay.UserContributionCommand,
	})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "research-workflow-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v error=%v", claim, err)
	}
	authority, err := newTurnAuthority(claim.Job)
	if err != nil {
		t.Fatal(err)
	}
	authority, err = bindObjectiveModelInstruction(authority, assemblyline.ArtifactIdentityProvenance{})
	if err != nil {
		t.Fatal(err)
	}
	return researchWorkflowFixture{pool, repository, claim, authority, &exactEvidenceStationClient{}, newResearchHTTPFixture(t, fixture)}
}

func (fixture researchWorkflowFixture) runtime() *nativeRuntimeV3 {
	return &nativeRuntimeV3{ctx: context.Background(), claim: fixture.claim, svc: &Service{
		repo: fixture.repository, stationClient: fixture.provider, webSearch: fixture.http.service,
		inferenceContextTokens: "32768", runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}}
}
