package queue

import (
	"os"
	"strings"
	"testing"
)

func TestBuildPostgresDSNFromFields(t *testing.T) {
	dsn, err := BuildPostgresDSN(DataSourceRecord{
		Host:         "db.example.com",
		Port:         5433,
		DatabaseName: "analytics",
		Username:     "reader",
		Password:     "secret",
		SSLMode:      "require",
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
	if _, err := BuildPostgresDSN(DataSourceRecord{Host: "localhost"}); err == nil {
		t.Fatal("expected error for missing database and username")
	}
}

func TestBuildPostgresDSNFromConnectionString(t *testing.T) {
	dsn, err := BuildPostgresDSN(DataSourceRecord{
		UseDSN: true,
		DSN:    "postgres://reader:secret@localhost:5432/app",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dsn != "postgres://reader:secret@localhost:5432/app" {
		t.Fatalf("unexpected dsn: %q", dsn)
	}
}

func TestNormalizeDataSourceRecordDefaults(t *testing.T) {
	record := normalizeDataSourceRecord(DataSourceRecord{})
	if record.Driver != "postgres" {
		t.Fatalf("driver = %q", record.Driver)
	}
	if record.Port != 5432 {
		t.Fatalf("port = %d", record.Port)
	}
	if record.SSLMode != "prefer" {
		t.Fatalf("ssl_mode = %q", record.SSLMode)
	}
	if !record.ReadOnly {
		t.Fatal("expected read-only default")
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
