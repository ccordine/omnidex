package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

var errRoleplayChannelStoreUnavailable = errors.New("channel store is unavailable")

func roleplayComponentQuery(r *http.Request) (roleplaySimulationPageState, model.ChannelID, error) {
	if err := validateExactQuery(
		r, "channel_id", "characters_offset", "personas_offset", "turn_order_offset",
		"meters_offset", "inventory_offset", "interactions_offset", "item_templates_offset",
	); err != nil {
		return roleplaySimulationPageState{}, "", err
	}
	channelID := model.ChannelID(r.URL.Query().Get("channel_id"))
	if err := channelID.Validate(); err != nil {
		return roleplaySimulationPageState{}, "", err
	}
	page := roleplaySimulationPageState{}
	fields := []struct {
		name  string
		value *int
	}{
		{"characters_offset", &page.Characters},
		{"personas_offset", &page.Personas},
		{"turn_order_offset", &page.TurnOrder},
		{"meters_offset", &page.Meters},
		{"inventory_offset", &page.Inventory},
		{"interactions_offset", &page.Interactions},
		{"item_templates_offset", &page.ItemTemplates},
	}
	for _, field := range fields {
		value, err := exactChannelQueryInteger(r, field.name, 0, 0, 1<<30)
		if err != nil {
			return roleplaySimulationPageState{}, "", err
		}
		if value%roleplaySimulationPageSize != 0 {
			return roleplaySimulationPageState{}, "", fmt.Errorf(
				"%s must use the server page size %d", field.name, roleplaySimulationPageSize,
			)
		}
		*field.value = value
	}
	return page, channelID, nil
}

func writeRoleplaySimulationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errRoleplaySimulationUnavailable), errors.Is(err, errRoleplayChannelStoreUnavailable):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "roleplay authority was not found")
	case errors.Is(err, roleplay.ErrSimulationNotConfigured),
		errors.Is(err, roleplay.ErrSimulationStaleRevision),
		errors.Is(err, roleplay.ErrSimulationConflict),
		errors.Is(err, roleplay.ErrSimulationIllegal):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, roleplay.ErrSimulationUnknown), errors.Is(err, roleplay.ErrSimulationAmbiguous):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
