package api

import (
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/roleplay"
)

const maxRoleplaySimulationBodyBytes int64 = 32 * 1024

type roleplayCharacterRequest struct {
	Name string `json:"name"`
}

type roleplayPersonaRequest struct {
	ExpectedRevision *int64   `json:"expected_revision"`
	Summary          string   `json:"summary"`
	Voice            string   `json:"voice"`
	Traits           []string `json:"traits"`
	Goals            []string `json:"goals"`
}

func (request roleplayPersonaRequest) sheet() roleplay.PersonaSheet {
	return roleplay.PersonaSheet{
		Summary: request.Summary,
		Voice:   request.Voice,
		Traits:  append([]string(nil), request.Traits...),
		Goals:   append([]string(nil), request.Goals...),
	}
}

type roleplaySceneRequest struct {
	ExpectedRevision      *int64   `json:"expected_revision,omitempty"`
	ExpectedDraftRevision *int64   `json:"expected_draft_revision,omitempty"`
	Title                 string   `json:"title"`
	Description           string   `json:"description"`
	ParticipantIDs        []string `json:"participant_ids"`
}

type roleplayMeterRequest struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Minimum      *int   `json:"minimum"`
	Maximum      *int   `json:"maximum"`
	InitialValue *int   `json:"initial_value"`
}

func (request roleplayMeterRequest) definition(worldID string) (roleplay.MeterDefinition, error) {
	if request.Minimum == nil || request.Maximum == nil || request.InitialValue == nil {
		return roleplay.MeterDefinition{}, fmt.Errorf("meter minimum, maximum, and initial_value are required")
	}
	return roleplay.MeterDefinition{
		WorldID: worldID, Key: request.Key, Name: request.Name,
		Minimum: *request.Minimum, Maximum: *request.Maximum, InitialValue: *request.InitialValue,
	}, nil
}

type roleplayMeterDeltaRequest struct {
	MeterKey string `json:"meter_key"`
	Delta    *int   `json:"delta"`
}

func roleplayMeterDeltas(requests []roleplayMeterDeltaRequest) ([]roleplay.MeterDelta, error) {
	effects := make([]roleplay.MeterDelta, len(requests))
	for index, request := range requests {
		if request.Delta == nil {
			return nil, fmt.Errorf("meter effect %d delta is required", index)
		}
		effects[index] = roleplay.MeterDelta{MeterKey: request.MeterKey, Delta: *request.Delta}
	}
	return effects, nil
}

type roleplayInteractionRequest struct {
	Key          string                       `json:"key"`
	Name         string                       `json:"name"`
	Description  string                       `json:"description"`
	ArgumentMode roleplay.CommandArgumentMode `json:"argument_mode"`
	Effects      []roleplayMeterDeltaRequest  `json:"effects"`
}

func (request roleplayInteractionRequest) definition(
	id, worldID string,
) (roleplay.InteractionCommandDefinition, error) {
	effects, err := roleplayMeterDeltas(request.Effects)
	if err != nil {
		return roleplay.InteractionCommandDefinition{}, err
	}
	return roleplay.InteractionCommandDefinition{
		ID: id, WorldID: worldID, Key: request.Key, Name: request.Name,
		Description: request.Description, ArgumentMode: request.ArgumentMode, Effects: effects,
	}, nil
}

type roleplayItemTriggerRequest struct {
	MeterKey  string                      `json:"meter_key"`
	Direction roleplay.ThresholdDirection `json:"direction"`
	Threshold *int                        `json:"threshold"`
}

type roleplayItemRequest struct {
	Name        string                      `json:"name"`
	Description string                      `json:"description"`
	UsePolicy   roleplay.ItemUsePolicy      `json:"use_policy"`
	InitialUses *int                        `json:"initial_uses"`
	Trigger     *roleplayItemTriggerRequest `json:"trigger"`
	Priority    *int                        `json:"priority"`
	Effects     []roleplayMeterDeltaRequest `json:"effects"`
}

func (request roleplayItemRequest) definition(id, worldID string) (roleplay.ItemTemplateDefinition, error) {
	if request.Priority == nil || request.InitialUses == nil {
		return roleplay.ItemTemplateDefinition{}, fmt.Errorf("item priority and initial_uses are required")
	}
	effects, err := roleplayMeterDeltas(request.Effects)
	if err != nil {
		return roleplay.ItemTemplateDefinition{}, err
	}
	var trigger *roleplay.ItemTrigger
	if request.Trigger != nil {
		if request.Trigger.Threshold == nil {
			return roleplay.ItemTemplateDefinition{}, fmt.Errorf("item trigger threshold is required")
		}
		trigger = &roleplay.ItemTrigger{
			MeterKey:  request.Trigger.MeterKey,
			Direction: request.Trigger.Direction,
			Threshold: *request.Trigger.Threshold,
		}
	}
	return roleplay.ItemTemplateDefinition{
		ID: id, WorldID: worldID, Name: request.Name, Description: request.Description,
		UsePolicy: request.UsePolicy, InitialUses: *request.InitialUses,
		Trigger: trigger, Priority: *request.Priority, Effects: effects,
	}, nil
}

type roleplayMeterValueRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
	Value            *int   `json:"value"`
}

type roleplayResearchCapabilityRequest struct {
	Enabled          *bool `json:"enabled"`
	CharactersOffset *int  `json:"characters_offset"`
}

func decodeExactRoleplayJSON(w http.ResponseWriter, r *http.Request, name string, target any) error {
	return decodeExactChannelJSON(w, r, name, maxRoleplaySimulationBodyBytes, target)
}
