package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/jackc/pgx/v5"
)

func (r DataSourceRecord) Connection() datasource.Connection {
	return datasource.Connection{
		Driver:       r.Driver,
		Host:         r.Host,
		Port:         r.Port,
		DatabaseName: r.DatabaseName,
		Username:     r.Username,
		Password:     r.Password,
		SSLMode:      r.SSLMode,
		UseDSN:       r.UseDSN,
		DSN:          r.DSN,
		ReadOnly:     r.ReadOnly,
	}
}

func (r DataSourceRecord) Profile() datasource.Profile {
	return datasource.NormalizeProfile(datasource.Profile{
		Driver:        r.Driver,
		Domain:        r.Domain,
		ContextPrompt: r.ContextPrompt,
		PrivacyMode:   r.PrivacyMode,
	})
}

func BuildPostgresDSN(record DataSourceRecord) (string, error) {
	return datasource.BuildPostgresDSN(record.Connection())
}

const DataSourcesWorkspaceKey = "data_sources"

type DataSourceRecord struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Driver          string     `json:"driver"`
	Domain          string     `json:"domain"`
	ContextPrompt   string     `json:"context_prompt"`
	PrivacyMode     string     `json:"privacy_mode"`
	Host            string     `json:"host"`
	Port            int        `json:"port"`
	DatabaseName    string     `json:"database_name"`
	Username        string     `json:"username"`
	Password        string     `json:"password,omitempty"`
	SSLMode         string     `json:"ssl_mode"`
	UseDSN          bool       `json:"use_dsn"`
	DSN             string     `json:"dsn,omitempty"`
	ReadOnly        bool       `json:"read_only"`
	LastTestStatus  string     `json:"last_test_status"`
	LastTestMessage string     `json:"last_test_message"`
	LastTestAt      *time.Time `json:"last_test_at,omitempty"`
	CatalogUpdatedAt *time.Time `json:"catalog_updated_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type DataSourceUpsert struct {
	Name          string
	Driver        string
	Domain        string
	ContextPrompt string
	PrivacyMode   string
	Host          string
	Port          int
	DatabaseName  string
	Username      string
	Password      string
	SSLMode       string
	UseDSN        bool
	DSN           string
	ReadOnly      bool
}

func (r *Repository) ListDataSources(ctx context.Context) ([]DataSourceRecord, error) {
	raw, err := r.getWorkspaceJSON(ctx, DataSourcesWorkspaceKey)
	if err != nil {
		return nil, err
	}
	var items []DataSourceRecord
	if len(raw) == 0 {
		return []DataSourceRecord{}, nil
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) GetDataSource(ctx context.Context, id string) (DataSourceRecord, error) {
	id = strings.TrimSpace(id)
	items, err := r.ListDataSources(ctx)
	if err != nil {
		return DataSourceRecord{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return DataSourceRecord{}, pgx.ErrNoRows
}

func (r *Repository) CreateDataSource(ctx context.Context, input DataSourceUpsert) (DataSourceRecord, error) {
	items, err := r.ListDataSources(ctx)
	if err != nil {
		return DataSourceRecord{}, err
	}
	now := time.Now().UTC()
	record := normalizeDataSourceRecord(DataSourceRecord{
		ID:            newDataSourceID(),
		Name:          input.Name,
		Driver:        input.Driver,
		Domain:        input.Domain,
		ContextPrompt: input.ContextPrompt,
		PrivacyMode:   input.PrivacyMode,
		Host:          input.Host,
		Port:          input.Port,
		DatabaseName:  input.DatabaseName,
		Username:      input.Username,
		Password:      input.Password,
		SSLMode:       input.SSLMode,
		UseDSN:        input.UseDSN,
		DSN:           input.DSN,
		ReadOnly:      input.ReadOnly,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	items = append(items, record)
	if err := r.saveDataSources(ctx, items); err != nil {
		return DataSourceRecord{}, err
	}
	return record, nil
}

func (r *Repository) UpdateDataSource(ctx context.Context, id string, input DataSourceUpsert) (DataSourceRecord, error) {
	id = strings.TrimSpace(id)
	items, err := r.ListDataSources(ctx)
	if err != nil {
		return DataSourceRecord{}, err
	}
	found := false
	var updated DataSourceRecord
	for i, item := range items {
		if item.ID != id {
			continue
		}
		found = true
		next := item
		next.Name = input.Name
		next.Driver = input.Driver
		next.Domain = input.Domain
		next.ContextPrompt = input.ContextPrompt
		next.PrivacyMode = input.PrivacyMode
		next.Host = input.Host
		next.Port = input.Port
		next.DatabaseName = input.DatabaseName
		next.Username = input.Username
		if strings.TrimSpace(input.Password) != "" {
			next.Password = input.Password
		}
		next.SSLMode = input.SSLMode
		next.UseDSN = input.UseDSN
		if strings.TrimSpace(input.DSN) != "" {
			next.DSN = input.DSN
		}
		next.ReadOnly = input.ReadOnly
		next.UpdatedAt = time.Now().UTC()
		next = normalizeDataSourceRecord(next)
		items[i] = next
		updated = next
		break
	}
	if !found {
		return DataSourceRecord{}, pgx.ErrNoRows
	}
	if err := r.saveDataSources(ctx, items); err != nil {
		return DataSourceRecord{}, err
	}
	return updated, nil
}

func (r *Repository) DeleteDataSource(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	items, err := r.ListDataSources(ctx)
	if err != nil {
		return err
	}
	next := make([]DataSourceRecord, 0, len(items))
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
			continue
		}
		next = append(next, item)
	}
	if !found {
		return pgx.ErrNoRows
	}
	if err := r.saveDataSources(ctx, next); err != nil {
		return err
	}
	_ = r.DeleteDataSourceCatalog(ctx, id)
	return nil
}

func (r *Repository) UpdateDataSourceTestResult(ctx context.Context, id, status, message string) (DataSourceRecord, error) {
	id = strings.TrimSpace(id)
	items, err := r.ListDataSources(ctx)
	if err != nil {
		return DataSourceRecord{}, err
	}
	found := false
	var updated DataSourceRecord
	now := time.Now().UTC()
	for i, item := range items {
		if item.ID != id {
			continue
		}
		found = true
		item.LastTestStatus = strings.TrimSpace(status)
		item.LastTestMessage = strings.TrimSpace(message)
		item.LastTestAt = &now
		item.UpdatedAt = now
		items[i] = item
		updated = item
		break
	}
	if !found {
		return DataSourceRecord{}, pgx.ErrNoRows
	}
	if err := r.saveDataSources(ctx, items); err != nil {
		return DataSourceRecord{}, err
	}
	return updated, nil
}

func (r *Repository) saveDataSources(ctx context.Context, items []DataSourceRecord) error {
	payload, err := json.Marshal(items)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO workspace_settings (key, value, updated_at)
		VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = NOW()
	`, DataSourcesWorkspaceKey, string(payload))
	return err
}

func (r *Repository) getWorkspaceJSON(ctx context.Context, key string) (json.RawMessage, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT value FROM workspace_settings WHERE key = $1`, key).Scan(&raw)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func newDataSourceID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("ds-%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func (r *Repository) UpdateDataSourceCatalogTimestamp(ctx context.Context, id string, at time.Time) error {
	id = strings.TrimSpace(id)
	items, err := r.ListDataSources(ctx)
	if err != nil {
		return err
	}
	found := false
	for i, item := range items {
		if item.ID != id {
			continue
		}
		found = true
		ts := at.UTC()
		items[i].CatalogUpdatedAt = &ts
		items[i].UpdatedAt = time.Now().UTC()
		break
	}
	if !found {
		return pgx.ErrNoRows
	}
	return r.saveDataSources(ctx, items)
}

func normalizeDataSourceRecord(record DataSourceRecord) DataSourceRecord {
	record.Name = strings.TrimSpace(record.Name)
	record.Driver = strings.ToLower(strings.TrimSpace(record.Driver))
	if record.Driver == "" {
		record.Driver = "postgres"
	}
	profile := datasource.NormalizeProfile(datasource.Profile{
		Driver:        record.Driver,
		Domain:        record.Domain,
		ContextPrompt: record.ContextPrompt,
		PrivacyMode:   record.PrivacyMode,
	})
	record.Domain = profile.Domain
	record.ContextPrompt = profile.ContextPrompt
	record.PrivacyMode = profile.PrivacyMode
	record.Host = strings.TrimSpace(record.Host)
	if record.Port <= 0 {
		record.Port = 5432
	}
	record.DatabaseName = strings.TrimSpace(record.DatabaseName)
	record.Username = strings.TrimSpace(record.Username)
	record.SSLMode = strings.TrimSpace(record.SSLMode)
	if record.SSLMode == "" {
		record.SSLMode = "prefer"
	}
	record.DSN = strings.TrimSpace(record.DSN)
	if !record.ReadOnly {
		record.ReadOnly = true
	}
	if record.Name == "" {
		record.Name = "Untitled source"
	}
	return record
}
