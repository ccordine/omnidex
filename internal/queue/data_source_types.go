package queue

import (
	"time"

	"github.com/gryph/omnidex/internal/datasource"
)

const DataSourcesWorkspaceKey = "data_sources"

type DataSourceRecord struct {
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

func (r DataSourceRecord) Connection() datasource.Connection {
	return datasource.Connection{
		Driver: r.Driver, Host: r.Host, Port: r.Port, DatabaseName: r.DatabaseName,
		Username: r.Username, Password: r.Password, SSLMode: r.SSLMode,
		UseDSN: r.UseDSN, DSN: r.DSN, ReadOnly: r.ReadOnly,
	}
}

func (r DataSourceRecord) Profile() datasource.Profile {
	return datasource.NormalizeProfile(datasource.Profile{
		Driver: r.Driver, Domain: r.Domain, ContextPrompt: r.ContextPrompt, PrivacyMode: r.PrivacyMode,
	})
}

func BuildPostgresDSN(record DataSourceRecord) (string, error) {
	return datasource.BuildPostgresDSN(record.Connection())
}
