package queue

import (
	"encoding/json"
	"testing"
	"time"
)

// legacyDataSourceJSON preserves the retired workspace-settings wire shape
// solely for migration fixtures. Current DataSourceRecord serialization must
// never expose password or DSN values.
type legacyDataSourceJSON struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Driver           string     `json:"driver"`
	Domain           string     `json:"domain"`
	ContextPrompt    string     `json:"context_prompt"`
	PrivacyMode      string     `json:"privacy_mode"`
	Host             string     `json:"host"`
	Port             int        `json:"port"`
	DatabaseName     string     `json:"database_name"`
	Username         string     `json:"username"`
	Password         string     `json:"password,omitempty"`
	SSLMode          string     `json:"ssl_mode"`
	UseDSN           bool       `json:"use_dsn"`
	DSN              string     `json:"dsn,omitempty"`
	ReadOnly         bool       `json:"read_only"`
	LastTestStatus   string     `json:"last_test_status"`
	LastTestMessage  string     `json:"last_test_message"`
	LastTestAt       *time.Time `json:"last_test_at,omitempty"`
	CatalogUpdatedAt *time.Time `json:"catalog_updated_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func marshalLegacyDataSourceRecords(t *testing.T, records ...DataSourceRecord) []byte {
	t.Helper()
	legacy := make([]legacyDataSourceJSON, len(records))
	for index, record := range records {
		legacy[index] = legacyDataSourceJSON{
			ID: record.ID, Name: record.Name, Driver: record.Driver,
			Domain: "retired-domain", ContextPrompt: "retired prompt authority", PrivacyMode: "retired-mode",
			Host: record.Host, Port: record.Port, DatabaseName: record.DatabaseName,
			Username: record.Username, Password: record.Password, SSLMode: record.SSLMode,
			UseDSN: record.UseDSN, DSN: record.DSN, ReadOnly: record.ReadOnly,
			LastTestStatus: record.LastTestStatus, LastTestMessage: record.LastTestMessage,
			LastTestAt: record.LastTestAt, CatalogUpdatedAt: record.CatalogUpdatedAt,
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		}
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
