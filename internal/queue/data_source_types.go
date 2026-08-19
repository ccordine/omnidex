package queue

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
)

type DataSourceRecord struct {
	ID               string                   `json:"id"`
	Name             string                   `json:"name"`
	Driver           string                   `json:"driver"`
	ExecutionMode    datasource.ExecutionMode `json:"execution_mode"`
	Host             string                   `json:"host"`
	Port             int                      `json:"port"`
	DatabaseName     string                   `json:"database_name"`
	Username         string                   `json:"username"`
	Password         string                   `json:"-"`
	SSLMode          string                   `json:"ssl_mode"`
	UseDSN           bool                     `json:"use_dsn"`
	DSN              string                   `json:"-"`
	AuthorityURL     string                   `json:"authority_url,omitempty"`
	CredentialEnv    string                   `json:"credential_env,omitempty"`
	ReadOnly         bool                     `json:"read_only"`
	LastTestStatus   string                   `json:"last_test_status"`
	LastTestMessage  string                   `json:"last_test_message"`
	LastTestAt       *time.Time               `json:"last_test_at,omitempty"`
	CatalogUpdatedAt *time.Time               `json:"catalog_updated_at,omitempty"`
	CreatedAt        time.Time                `json:"created_at"`
	UpdatedAt        time.Time                `json:"updated_at"`
}

type DataSourceUpsert struct {
	Name          string
	Driver        string
	ExecutionMode datasource.ExecutionMode
	Host          string
	Port          int
	DatabaseName  string
	Username      string
	Password      string
	SSLMode       string
	UseDSN        bool
	DSN           string
	AuthorityURL  string
	CredentialEnv string
}

func (r DataSourceRecord) DirectConnection() (datasource.Connection, error) {
	if r.ExecutionMode != datasource.ExecutionModeDirect {
		return datasource.Connection{}, fmt.Errorf("data source %q is not a direct PostgreSQL authority", r.ID)
	}
	return datasource.Connection{
		Driver: r.Driver, Host: r.Host, Port: r.Port, DatabaseName: r.DatabaseName,
		Username: r.Username, Password: r.Password, SSLMode: r.SSLMode,
		UseDSN: r.UseDSN, DSN: r.DSN, ReadOnly: r.ReadOnly,
	}, nil
}

func BuildPostgresDSN(record DataSourceRecord) (string, error) {
	connection, err := record.DirectConnection()
	if err != nil {
		return "", err
	}
	return datasource.BuildPostgresDSN(connection)
}
