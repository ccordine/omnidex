package roleplay

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestSimulationSlashCommandsProjectOnlyCurrentLegalActiveCharacterSurface(t *testing.T) {
	pool, _ := openRoleplayTestPool(t)
	installSimulationTestSchema(t, pool)
	installResearchTestSchema(t, pool)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, viewpoint := bootstrapRoleplayChannel(t, pool, "slash-projection", "Archive", "Ari")
	active, err := store.CreateCharacter(ctx, world.ID, "Bex")
	if err != nil {
		t.Fatal(err)
	}
	writeTestPersona(t, store, viewpoint.ID, "A careful archivist.")
	writeTestPersona(t, store, active.ID, "A patient cook.")
	if err := store.RegisterMeter(ctx, MeterDefinition{
		WorldID: world.ID, Key: "affinity", Name: "Affinity", Minimum: 0, Maximum: 10, InitialValue: 5,
	}); err != nil {
		t.Fatal(err)
	}
	commandID, err := NewInteractionCommandIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterInteractionCommand(ctx, InteractionCommandDefinition{
		ID: commandID, WorldID: world.ID, Key: "feed", Name: "Feed",
		Description: "Offer the active character some food.", ArgumentMode: CommandArgumentRequired,
		Effects: []MeterDelta{{MeterKey: "affinity", Delta: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	waveID, err := NewInteractionCommandIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterInteractionCommand(ctx, InteractionCommandDefinition{
		ID: waveID, WorldID: world.ID, Key: "wave", Name: "Wave",
		Description: "Wave to the active character.", ArgumentMode: CommandArgumentNone,
		Effects: []MeterDelta{{MeterKey: "affinity", Delta: 1}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Alpha", "Beta"} {
		itemID, idErr := NewItemTemplateIdentity()
		if idErr != nil {
			t.Fatal(idErr)
		}
		if err := store.RegisterItemTemplate(ctx, ItemTemplateDefinition{
			ID: itemID, WorldID: world.ID, Name: name, Description: "A bounded fixture item.",
			UsePolicy: ItemUseInfinite, Effects: []MeterDelta{{MeterKey: "affinity", Delta: 1}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	sceneID, err := NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	scene, err := store.CreateCurrentScene(ctx, SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Reading room",
		Description: "An exact task-neutral simulation scene.", ParticipantIDs: []string{active.ID, viewpoint.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	given := applyTestAction(t, store, world.ID, scene.ID, scene.Revision, `/give "Alpha"`)
	if _, err := store.ConfigureCharacterCapability(ctx, world.ID, active.ID, CapabilityWebResearch, true); err != nil {
		t.Fatal(err)
	}

	projection, err := store.ProjectSimulationSlashCommands(ctx, world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Schema != SimulationSlashCommandProjectionSchemaV1 ||
		projection.WorldID != world.ID || projection.SceneID != scene.ID ||
		projection.SceneRevision != given.AfterRevision || projection.ActiveCharacterID != active.ID ||
		projection.ActiveCharacterName != "Bex" || projection.ActiveCharacterID == string(viewpoint.ID) {
		t.Fatalf("projection authority=%+v", projection)
	}
	want := []struct {
		kind      SimulationSlashCommandKind
		insertion string
		display   string
		cursor    int
	}{
		{SimulationSlashCommandInteraction, `/feed ""`, `/feed "…"`, 7},
		{SimulationSlashCommandInteraction, `/wave`, `/wave`, 5},
		{SimulationSlashCommandGive, `/give "Beta"`, `/give "Beta"`, 12},
		{SimulationSlashCommandTake, `/take "Alpha"`, `/take "Alpha"`, 13},
		{SimulationSlashCommandResearch, `/research ""`, `/research "…"`, 11},
	}
	if len(projection.Commands) != len(want) {
		t.Fatalf("commands=%+v", projection.Commands)
	}
	for index, expected := range want {
		actual := projection.Commands[index]
		if actual.Kind != expected.kind || actual.Insertion != expected.insertion ||
			actual.Display != expected.display || actual.CursorUTF16 != expected.cursor ||
			actual.DisplayOrder != index || actual.ID == "" {
			t.Errorf("command[%d]=%+v want=%+v", index, actual, expected)
		}
	}
	replayed, err := store.ProjectSimulationSlashCommands(ctx, world.ID)
	if err != nil || !reflect.DeepEqual(replayed, projection) {
		t.Fatalf("replayed=%+v error=%v want=%+v", replayed, err, projection)
	}

	if _, err := store.ConfigureCharacterCapability(ctx, world.ID, active.ID, CapabilityWebResearch, false); err != nil {
		t.Fatal(err)
	}
	withoutResearch, err := store.ProjectSimulationSlashCommands(ctx, world.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range withoutResearch.Commands {
		if command.Kind == SimulationSlashCommandResearch {
			t.Fatalf("revoked research command remains: %+v", withoutResearch.Commands)
		}
	}

	revision := given.AfterRevision
	for index := 0; index < MaxWorldItemTemplates-2; index++ {
		name := fmt.Sprintf("Capacity %02d", index)
		itemID, idErr := NewItemTemplateIdentity()
		if idErr != nil {
			t.Fatal(idErr)
		}
		if err := store.RegisterItemTemplate(ctx, ItemTemplateDefinition{
			ID: itemID, WorldID: world.ID, Name: name, Description: "A capacity fixture.",
			UsePolicy: ItemUseInfinite, Effects: []MeterDelta{{MeterKey: "affinity", Delta: 1}},
		}); err != nil {
			t.Fatal(err)
		}
		transition := applyTestAction(t, store, world.ID, scene.ID, revision, `/give "`+name+`"`)
		revision = transition.AfterRevision
	}
	transition := applyTestAction(t, store, world.ID, scene.ID, revision, `/give "Beta"`)
	revision = transition.AfterRevision
	full, err := store.ProjectSimulationSlashCommands(ctx, world.ID)
	if err != nil {
		t.Fatal(err)
	}
	if full.SceneRevision != revision || len(full.Commands) != 2+MaxInventoryItems {
		t.Fatalf("full inventory projection=%+v", full)
	}
	for _, command := range full.Commands {
		if command.Kind == SimulationSlashCommandGive {
			t.Fatalf("full inventory exposed illegal give command: %+v", command)
		}
	}

	overflowID, err := NewItemTemplateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO roleplay_item_templates (
			id,world_id,name,description,use_policy,priority
		) VALUES ($1,$2,'Overflow','A corrupt over-bound fixture.','infinite',0)
	`, overflowID, world.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO roleplay_item_effects (template_id,world_id,meter_key,delta)
		VALUES ($1,$2,'affinity',1)
	`, overflowID, world.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ProjectSimulationSlashCommands(ctx, world.ID); !errors.Is(err, ErrSimulationNotConfigured) {
		t.Fatalf("over-bound projection error=%v", err)
	}
}
