package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/jackc/pgx/v5"
)

const CodingAgentSettingsSchema = "omnidex.coding-agent-settings.v1"

type codingAgentSettingsQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (repository *Repository) LoadCodingAgentConfiguration(ctx context.Context) (codingagent.Configuration, string, error) {
	return repository.loadCodingAgentConfiguration(ctx, repository.pool)
}

func (repository *Repository) loadCodingAgentConfiguration(ctx context.Context, querier codingAgentSettingsQuerier) (codingagent.Configuration, string, error) {
	var schema string
	var raw []byte
	err := querier.QueryRow(ctx, `SELECT schema,configuration FROM coding_agent_settings WHERE singleton=TRUE FOR SHARE`).Scan(&schema, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		configuration, err := repository.codingAgents.ForMode(codingagent.Agents)
		return configuration, "startup", err
	}
	if err != nil {
		return codingagent.Configuration{}, "", err
	}
	if schema != CodingAgentSettingsSchema {
		return codingagent.Configuration{}, "", fmt.Errorf("stored coding-agent settings have an unsupported schema")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var configuration codingagent.Configuration
	if err := decoder.Decode(&configuration); err != nil {
		return configuration, "", err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return configuration, "", fmt.Errorf("stored coding-agent settings contain trailing content")
	}
	if configuration.LedgerParser == (codingagent.Route{}) {
		configuration.LedgerParser = repository.codingAgents.LedgerParser
	}
	if configuration.Mode != codingagent.Agents {
		return configuration, "", fmt.Errorf("stored coding-agent settings require Agents mode")
	}
	if err := configuration.Validate(); err != nil {
		return configuration, "", err
	}
	return configuration, "saved", nil
}

func (repository *Repository) codingConfigurationForMode(ctx context.Context, querier codingAgentSettingsQuerier, mode codingagent.Mode) (codingagent.Configuration, error) {
	if mode == "" {
		return codingagent.Configuration{}, nil
	}
	if mode != codingagent.Agents {
		return repository.codingAgents.ForMode(mode)
	}
	configuration, _, err := repository.loadCodingAgentConfiguration(ctx, querier)
	return configuration, err
}

func (repository *Repository) StoreCodingAgentConfiguration(ctx context.Context, configuration codingagent.Configuration) error {
	if configuration.Mode != codingagent.Agents {
		return fmt.Errorf("coding-agent settings require explicit Agents mode")
	}
	if err := configuration.Validate(); err != nil {
		return err
	}
	encoded, err := json.Marshal(configuration)
	if err != nil {
		return err
	}
	_, err = repository.pool.Exec(ctx, `INSERT INTO coding_agent_settings(singleton,schema,configuration) VALUES(TRUE,$1,$2::jsonb) ON CONFLICT(singleton) DO UPDATE SET schema=EXCLUDED.schema,configuration=EXCLUDED.configuration,updated_at=clock_timestamp()`, CodingAgentSettingsSchema, string(encoded))
	return err
}

func (repository *Repository) CodingAgentsBusy(ctx context.Context) (bool, error) {
	var busy bool
	err := repository.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE status IN ('pending','running','waiting_input') AND metadata->'coding_execution'->>'mode'='agents')`).Scan(&busy)
	return busy, err
}
