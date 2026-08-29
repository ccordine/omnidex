package roleplay

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/jackc/pgx/v5"
)

const (
	SimulationSlashCommandProjectionSchemaV1 = "omnidex.roleplay-simulation-slash-commands.v1"
	MaxSimulationSlashCommands               = MaxInteractionCommands + MaxWorldItemTemplates + 1
)

type SimulationSlashCommandKind string

const (
	SimulationSlashCommandInteraction SimulationSlashCommandKind = "interaction"
	SimulationSlashCommandGive        SimulationSlashCommandKind = "give"
	SimulationSlashCommandTake        SimulationSlashCommandKind = "take"
	SimulationSlashCommandResearch    SimulationSlashCommandKind = "research"
)

type SimulationSlashCommand struct {
	ID           string
	Kind         SimulationSlashCommandKind
	Key          string
	Insertion    string
	Display      string
	Label        string
	Description  string
	CursorUTF16  int
	DisplayOrder int
}

type SimulationSlashCommandProjection struct {
	Schema              string
	WorldID             string
	SceneID             string
	SceneRevision       int64
	ActiveCharacterID   string
	ActiveCharacterName string
	Commands            []SimulationSlashCommand
}

// ProjectSimulationSlashCommands returns the complete bounded command surface
// for the active character in one repeatable-read snapshot. It is presentation
// authority only: the ordinary turn boundary reparses and validates any text a
// user later submits.
func (s *Store) ProjectSimulationSlashCommands(
	ctx context.Context,
	worldID string,
) (SimulationSlashCommandProjection, error) {
	if err := s.validateContext(ctx); err != nil {
		return SimulationSlashCommandProjection{}, err
	}
	if err := validateIdentity(worldID, worldIdentity); err != nil {
		return SimulationSlashCommandProjection{}, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return SimulationSlashCommandProjection{}, err
	}
	defer tx.Rollback(context.Background())
	projection, err := projectSimulationSlashCommandsTx(ctx, tx, worldID)
	if err != nil {
		return SimulationSlashCommandProjection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SimulationSlashCommandProjection{}, err
	}
	return projection, nil
}

func projectSimulationSlashCommandsTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID string,
) (SimulationSlashCommandProjection, error) {
	scene, err := projectCurrentSceneTx(ctx, tx, worldID)
	if err != nil {
		return SimulationSlashCommandProjection{}, err
	}
	participants, err := loadSceneParticipantsTx(ctx, tx, scene.ID)
	if err != nil {
		return SimulationSlashCommandProjection{}, err
	}
	activeName := ""
	for _, participant := range participants {
		if participant.CharacterID == scene.ActiveCharacterID {
			activeName = participant.Name
			break
		}
	}
	if activeName == "" {
		return SimulationSlashCommandProjection{}, fmt.Errorf(
			"%w: active character is not a scene participant", ErrSimulationNotConfigured,
		)
	}

	interactions, err := loadInteractionDefinitionsTx(ctx, tx, worldID)
	if err != nil {
		return SimulationSlashCommandProjection{}, err
	}
	templates, err := loadItemDefinitionsTx(ctx, tx, worldID)
	if err != nil {
		return SimulationSlashCommandProjection{}, err
	}
	inventory, err := loadInventoryProjectionsTx(
		ctx, tx, worldID, scene.ActiveCharacterID, MaxInventoryItems, 0,
	)
	if err != nil {
		return SimulationSlashCommandProjection{}, err
	}
	capability, err := projectCharacterCapabilityTx(ctx, tx, worldID, scene.ActiveCharacterID)
	if err != nil {
		return SimulationSlashCommandProjection{}, err
	}

	commands := make([]SimulationSlashCommand, 0, MaxSimulationSlashCommands)
	for _, interaction := range interactions {
		exact := "/" + interaction.Key
		display := exact
		cursor := slashCommandUTF16Length(exact)
		if interaction.ArgumentMode == CommandArgumentRequired {
			exact += ` ""`
			display += ` "…"`
			cursor = slashCommandUTF16Length(strings.TrimSuffix(exact, `"`))
		}
		commands = append(commands, newSimulationSlashCommand(
			SimulationSlashCommandInteraction, interaction.Key, exact, display,
			interaction.Name, interaction.Description, cursor,
		))
	}

	heldTemplateIDs := make(map[string]struct{}, len(inventory))
	for _, item := range inventory {
		if _, duplicate := heldTemplateIDs[item.TemplateID]; duplicate {
			return SimulationSlashCommandProjection{}, fmt.Errorf(
				"%w: active inventory repeats an item template", ErrSimulationNotConfigured,
			)
		}
		heldTemplateIDs[item.TemplateID] = struct{}{}
	}
	if len(inventory) < MaxInventoryItems {
		for _, template := range templates {
			if _, held := heldTemplateIDs[template.ID]; held {
				continue
			}
			exact, exactErr := CanonicalItemAction(SimulationActionGive, template.Name)
			if exactErr != nil {
				return SimulationSlashCommandProjection{}, fmt.Errorf("project /give command: %w", exactErr)
			}
			commands = append(commands, newSimulationSlashCommand(
				SimulationSlashCommandGive, string(SimulationActionGive), exact, exact,
				"Give "+template.Name, "Give "+template.Name+" to "+activeName+".",
				slashCommandUTF16Length(exact),
			))
		}
	}
	for _, item := range inventory {
		exact, exactErr := CanonicalItemAction(SimulationActionTake, item.Name)
		if exactErr != nil {
			return SimulationSlashCommandProjection{}, fmt.Errorf("project /take command: %w", exactErr)
		}
		commands = append(commands, newSimulationSlashCommand(
			SimulationSlashCommandTake, string(SimulationActionTake), exact, exact,
			"Take "+item.Name, "Take "+item.Name+" from "+activeName+".",
			slashCommandUTF16Length(exact),
		))
	}
	if capability.WebResearch {
		const exact = `/research ""`
		commands = append(commands, newSimulationSlashCommand(
			SimulationSlashCommandResearch, "research", exact, `/research "…"`,
			"Research the web", "Research a real-world question as "+activeName+".",
			slashCommandUTF16Length(strings.TrimSuffix(exact, `"`)),
		))
	}

	sort.Slice(commands, func(left, right int) bool {
		leftRank := simulationSlashCommandKindRank(commands[left].Kind)
		rightRank := simulationSlashCommandKindRank(commands[right].Kind)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if commands[left].Key != commands[right].Key {
			return commands[left].Key < commands[right].Key
		}
		return commands[left].Insertion < commands[right].Insertion
	})
	seenIDs := make(map[string]struct{}, len(commands))
	seenExact := make(map[string]struct{}, len(commands))
	for index := range commands {
		commands[index].DisplayOrder = index
		if _, duplicate := seenIDs[commands[index].ID]; duplicate {
			return SimulationSlashCommandProjection{}, fmt.Errorf("projected slash command identity is duplicated")
		}
		if _, duplicate := seenExact[commands[index].Insertion]; duplicate {
			return SimulationSlashCommandProjection{}, fmt.Errorf("projected slash command syntax is duplicated")
		}
		seenIDs[commands[index].ID] = struct{}{}
		seenExact[commands[index].Insertion] = struct{}{}
	}
	if len(commands) > MaxSimulationSlashCommands {
		return SimulationSlashCommandProjection{}, fmt.Errorf(
			"%w: slash commands exceed their %d-option bound", ErrSimulationNotConfigured, MaxSimulationSlashCommands,
		)
	}
	return SimulationSlashCommandProjection{
		Schema:  SimulationSlashCommandProjectionSchemaV1,
		WorldID: worldID, SceneID: scene.ID, SceneRevision: scene.Revision,
		ActiveCharacterID: scene.ActiveCharacterID, ActiveCharacterName: activeName,
		Commands: commands,
	}, nil
}

func newSimulationSlashCommand(
	kind SimulationSlashCommandKind,
	key, exact, display, label, description string,
	cursorUTF16 int,
) SimulationSlashCommand {
	fingerprint := simulationSHA([]byte("slash-command.v1\x00" + string(kind) + "\x00" + key + "\x00" + exact))
	return SimulationSlashCommand{
		ID: "slash-command-option-" + fingerprint[:16], Kind: kind, Key: key,
		Insertion: exact, Display: display, Label: label, Description: description,
		CursorUTF16: cursorUTF16,
	}
}

func simulationSlashCommandKindRank(kind SimulationSlashCommandKind) int {
	switch kind {
	case SimulationSlashCommandInteraction:
		return 0
	case SimulationSlashCommandGive:
		return 1
	case SimulationSlashCommandTake:
		return 2
	case SimulationSlashCommandResearch:
		return 3
	default:
		return 4
	}
}

func slashCommandUTF16Length(value string) int {
	return len(utf16.Encode([]rune(value)))
}
