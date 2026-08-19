package queue

import (
	"time"

	"github.com/gryph/omnidex/internal/datasource"
)

type DataSourceRecord struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Driver           string     `json:"driver"`
	Host             string     `json:"host"`
	Port             int        `json:"port"`
	DatabaseName     string     `json:"database_name"`
	Username         string     `json:"username"`
	Password         string     `json:"-"`
	SSLMode          string     `json:"ssl_mode"`
	UseDSN           bool       `json:"use_dsn"`
	DSN              string     `json:"-"`
	ReadOnly         bool       `json:"read_only"`
	LastTestStatus   string     `json:"last_test_status"`
	LastTestMessage  string     `json:"last_test_message"`
	LastTestAt       *time.Time `json:"last_test_at,omitempty"`
	CatalogUpdatedAt *time.Time `json:"catalog_updated_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type DataSourceUpsert struct {
	Name         string
	Driver       string
	Host         string
	Port         int
	DatabaseName string
	Username     string
	Password     string
	SSLMode      string
	UseDSN       bool
	DSN          string
}

func (r DataSourceRecord) Connection() datasource.Connection {
	return datasource.Connection{
		Driver: r.Driver, Host: r.Host, Port: r.Port, DatabaseName: r.DatabaseName,
		Username: r.Username, Password: r.Password, SSLMode: r.SSLMode,
		UseDSN: r.UseDSN, DSN: r.DSN, ReadOnly: r.ReadOnly,
	}
}

func BuildPostgresDSN(record DataSourceRecord) (string, error) {
	return datasource.BuildPostgresDSN(record.Connection())
}
