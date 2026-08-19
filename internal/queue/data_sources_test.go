package queue

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
)

func TestDataSourceRecordSerializationCannotExposePasswordOrDSN(t *testing.T) {
	t.Parallel()
	record := DataSourceRecord{
		ID: "source-1", Password: "credential-password", UseDSN: true,
		DSN: "postgres://reader:credential-password@database.internal/analytics",
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	serialized := strings.ToLower(string(raw))
	for _, forbidden := range []string{"credential-password", "postgres://", `"password"`, `"dsn"`} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("serialized data-source record contains %q: %s", forbidden, raw)
		}
	}
	if !strings.Contains(serialized, `"use_dsn":true`) {
		t.Fatalf("serialized public connection mode is missing: %s", raw)
	}
}

func TestBuildPostgresDSNFromFields(t *testing.T) {
	dsn, err := BuildPostgresDSN(DataSourceRecord{
		ExecutionMode: datasource.ExecutionModeDirect,
		Host:          "db.example.com",
		Port:          5433,
		DatabaseName:  "analytics",
		Username:      "reader",
		Password:      "secret",
		SSLMode:       "require",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "host=db.example.com port=5433 dbname=analytics user=reader sslmode=require password=secret"
	if dsn != want {
		t.Fatalf("dsn = %q, want %q", dsn, want)
	}
}

func TestBuildPostgresDSNRequiresFields(t *testing.T) {
	if _, err := BuildPostgresDSN(DataSourceRecord{ExecutionMode: datasource.ExecutionModeDirect, Host: "localhost"}); err == nil {
		t.Fatal("expected error for missing database and username")
	}
	base := DataSourceRecord{
		ExecutionMode: datasource.ExecutionModeDirect,
		Host:          "localhost", Port: 5432, DatabaseName: "app", Username: "reader", SSLMode: "prefer",
	}
	for name, mutate := range map[string]func(*DataSourceRecord){
		"port":     func(record *DataSourceRecord) { record.Port = 0 },
		"ssl mode": func(record *DataSourceRecord) { record.SSLMode = "" },
	} {
		t.Run(name, func(t *testing.T) {
			record := base
			mutate(&record)
			if _, err := BuildPostgresDSN(record); err == nil {
				t.Fatalf("invalid connection was accepted: %+v", record)
			}
		})
	}
}

func TestBuildPostgresDSNFromConnectionString(t *testing.T) {
	dsn, err := BuildPostgresDSN(DataSourceRecord{
		ExecutionMode: datasource.ExecutionModeDirect,
		UseDSN:        true,
		DSN:           "postgres://reader:secret@localhost:5432/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "postgres://reader:secret@localhost:5432/app" {
		t.Fatalf("unexpected dsn: %q", dsn)
	}
}

func TestDataSourceCanonicalizationDoesNotInventRequiredAuthority(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	valid := DataSourceRecord{
		ID: "source-1", Name: "Exact source", Driver: "postgres",
		ExecutionMode: datasource.ExecutionModeDirect,
		Host:          "localhost", Port: 5432, DatabaseName: "app", Username: "reader",
		SSLMode: "prefer", ReadOnly: true, CreatedAt: now, UpdatedAt: now,
	}
	for name, mutate := range map[string]func(*DataSourceRecord){
		"name":      func(record *DataSourceRecord) { record.Name = " " },
		"driver":    func(record *DataSourceRecord) { record.Driver = "" },
		"mode":      func(record *DataSourceRecord) { record.ExecutionMode = "" },
		"port":      func(record *DataSourceRecord) { record.Port = 0 },
		"ssl mode":  func(record *DataSourceRecord) { record.SSLMode = "" },
		"read only": func(record *DataSourceRecord) { record.ReadOnly = false },
	} {
		t.Run(name, func(t *testing.T) {
			record := valid
			mutate(&record)
			record = canonicalizeDataSourceRecord(record)
			if err := validateDataSourceRecord(record); err == nil {
				t.Fatalf("missing required authority was silently defaulted: %+v", record)
			}
		})
	}
}

func TestDelegatedDataSourceCarriesOnlyHostAuthorityConfiguration(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0).UTC()
	valid := DataSourceRecord{
		ID: "source-1", Name: "Clinical host", Driver: "postgres",
		ExecutionMode: datasource.ExecutionModeDelegated,
		AuthorityURL:  "https://application.internal",
		CredentialEnv: "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN",
		ReadOnly:      true, CreatedAt: now, UpdatedAt: now,
	}
	if err := validateDataSourceRecord(valid); err != nil {
		t.Fatal(err)
	}
	if _, err := valid.DirectConnection(); err == nil {
		t.Fatal("delegated source exposed a direct PostgreSQL connection")
	}
	for name, mutate := range map[string]func(*DataSourceRecord){
		"host":           func(record *DataSourceRecord) { record.Host = "database.internal" },
		"password":       func(record *DataSourceRecord) { record.Password = "forbidden" },
		"missing URL":    func(record *DataSourceRecord) { record.AuthorityURL = "" },
		"credential env": func(record *DataSourceRecord) { record.CredentialEnv = "OPENAI_API_KEY" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := validateDataSourceRecord(candidate); err == nil {
				t.Fatal("invalid delegated data-source authority was accepted")
			}
		})
	}
}

func TestDataSourceUpsertExposesOnlyConsumedMutableFields(t *testing.T) {
	t.Parallel()
	typeOf := reflect.TypeOf(DataSourceUpsert{})
	want := []string{"Name", "Driver", "ExecutionMode", "Host", "Port", "DatabaseName", "Username", "Password", "SSLMode", "UseDSN", "DSN", "AuthorityURL", "CredentialEnv"}
	got := make([]string, typeOf.NumField())
	for index := range got {
		got[index] = typeOf.Field(index).Name
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("data-source mutable fields=%v want %v", got, want)
	}
}

func TestDataSourcePageRequestRejectsUnboundedReads(t *testing.T) {
	for _, request := range []DataSourcePageRequest{
		{},
		{Limit: -1},
		{Limit: MaxDataSourcePageSize + 1},
		{Limit: 1, Offset: -1},
	} {
		if err := request.validate(); err == nil {
			t.Fatalf("request %+v should fail", request)
		}
	}
	if err := (DataSourcePageRequest{Limit: 20, Offset: 40}).validate(); err != nil {
		t.Fatalf("valid page request: %v", err)
	}
}

func TestDataSourceRepositoryHasNoExportedUnboundedList(t *testing.T) {
	for _, path := range []string{"data_sources.go", "data_source_channels.go", "data_source_pagination.go", "data_source_types.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{" ListDataSources(ctx ", " ListDataSourceChannels(ctx "} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("unbounded repository list %q remains in %s", forbidden, path)
			}
		}
	}
}

func TestDataSourceRepositoryHasOneRelationalAuthority(t *testing.T) {
	for _, path := range []string{
		"data_sources.go", "data_source_rows.go", "data_source_pagination.go", "data_source_schema_snapshot.go",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"workspace_settings", "jsonb_array_elements", "DataSourcesWorkspaceKey"} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("retired data-source authority %q remains in %s", forbidden, path)
			}
		}
	}
}
