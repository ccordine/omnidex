package queue

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
)

const dataSourceSelectColumns = `
	id, name, driver, execution_mode, host, port,
	database_name, username, password, ssl_mode, use_dsn, dsn, read_only,
	authority_url, credential_env,
	last_test_status, last_test_message, last_test_at, catalog_updated_at,
	created_at, updated_at
`

type dataSourceRowScanner interface {
	Scan(dest ...any) error
}

func scanDataSource(row dataSourceRowScanner) (DataSourceRecord, error) {
	var record DataSourceRecord
	err := row.Scan(
		&record.ID, &record.Name, &record.Driver, &record.ExecutionMode, &record.Host, &record.Port,
		&record.DatabaseName, &record.Username, &record.Password, &record.SSLMode,
		&record.UseDSN, &record.DSN, &record.ReadOnly, &record.AuthorityURL, &record.CredentialEnv,
		&record.LastTestStatus,
		&record.LastTestMessage, &record.LastTestAt, &record.CatalogUpdatedAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return DataSourceRecord{}, err
	}
	if err := validateDataSourceRecord(record); err != nil {
		return DataSourceRecord{}, fmt.Errorf("invalid stored data source %q: %w", record.ID, err)
	}
	return record, nil
}

func validateDataSourceID(id string) error {
	return model.DataSourceID(id).Validate()
}

func validateDataSourceRecord(record DataSourceRecord) error {
	if err := validateDataSourceID(record.ID); err != nil {
		return err
	}
	if record.Name == "" || record.Name != strings.TrimSpace(record.Name) {
		return fmt.Errorf("data source name must be exact nonblank text")
	}
	if record.Driver != "postgres" {
		return fmt.Errorf("data source driver %q is unsupported", record.Driver)
	}
	if err := record.ExecutionMode.Validate(); err != nil {
		return err
	}
	if !record.ReadOnly {
		return fmt.Errorf("data source must be read-only")
	}
	switch record.ExecutionMode {
	case datasource.ExecutionModeDirect:
		if record.AuthorityURL != "" || record.CredentialEnv != "" {
			return fmt.Errorf("direct data source cannot carry delegated authority configuration")
		}
		if record.Port < 1 || record.Port > 65535 {
			return fmt.Errorf("data source port must be between 1 and 65535")
		}
		if !validDataSourceSSLMode(record.SSLMode) {
			return fmt.Errorf("data source ssl_mode %q is unsupported", record.SSLMode)
		}
		if record.UseDSN {
			if strings.TrimSpace(record.DSN) == "" {
				return fmt.Errorf("data source dsn is required when use_dsn is true")
			}
		} else if record.Host == "" || record.DatabaseName == "" || record.Username == "" {
			return fmt.Errorf("data source host, database_name, and username are required")
		}
	case datasource.ExecutionModeDelegated:
		if record.Host != "" || record.Port != 0 || record.DatabaseName != "" || record.Username != "" ||
			record.Password != "" || record.SSLMode != "" || record.UseDSN || record.DSN != "" {
			return fmt.Errorf("delegated data source cannot carry direct PostgreSQL credentials")
		}
		if err := datasource.ValidateDelegatedBaseURL(record.AuthorityURL); err != nil {
			return err
		}
		if err := datasource.ValidateDelegatedCredentialEnvironmentName(record.CredentialEnv); err != nil {
			return err
		}
	}
	if record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		return fmt.Errorf("data source requires created and updated timestamps")
	}
	return nil
}

func validDataSourceSSLMode(value string) bool {
	switch value {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}
