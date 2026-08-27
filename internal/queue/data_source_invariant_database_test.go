package queue

import (
	"strings"
	"testing"
)

func TestRelationalDataSourceRepositoryRejectsMissingAuthorityWithoutDefaults(t *testing.T) {
	_, repository := relationalDataSourceTestRepository(t)
	valid := DataSourceUpsert{
		Name: "Exact source", Driver: "postgres", ExecutionMode: "direct", Host: "localhost", Port: 5432,
		DatabaseName: "fixture", Username: "reader", SSLMode: "prefer",
	}
	for name, test := range map[string]struct {
		mutate  func(*DataSourceUpsert)
		failure string
	}{
		"name": {
			func(input *DataSourceUpsert) { input.Name = " " },
			"name must be exact nonblank text",
		},
		"driver": {
			func(input *DataSourceUpsert) { input.Driver = "" },
			"driver \"\" is unsupported",
		},
		"port": {
			func(input *DataSourceUpsert) { input.Port = 0 },
			"port must be between 1 and 65535",
		},
		"ssl mode": {
			func(input *DataSourceUpsert) { input.SSLMode = "" },
			"ssl_mode \"\" is unsupported",
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := valid
			test.mutate(&input)
			if _, err := repository.CreateDataSource(t.Context(), input); err == nil ||
				!strings.Contains(err.Error(), test.failure) {
				t.Fatalf("missing authority error=%v want %q", err, test.failure)
			}
		})
	}
}

func TestRelationalDataSourceDatabaseRejectsMissingAndMutableAuthority(t *testing.T) {
	ctx, repository := relationalDataSourceTestRepository(t)
	source, err := repository.CreateDataSource(ctx, DataSourceUpsert{
		Name: "Exact source", Driver: "postgres", ExecutionMode: "direct", Host: "localhost", Port: 5432,
		DatabaseName: "fixture", Username: "reader", SSLMode: "prefer",
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		statement  string
		constraint string
	}{
		"name": {
			`UPDATE data_sources SET name='' WHERE id=$1`,
			"data_sources_name_check",
		},
		"driver": {
			`UPDATE data_sources SET driver='' WHERE id=$1`,
			"data_sources_driver_check",
		},
		"port": {
			`UPDATE data_sources SET port=0 WHERE id=$1`,
			"data_sources_execution_authority_shape_check",
		},
		"ssl mode": {
			`UPDATE data_sources SET ssl_mode='' WHERE id=$1`,
			"data_sources_execution_authority_shape_check",
		},
		"read only": {
			`UPDATE data_sources SET read_only=FALSE WHERE id=$1`,
			"data_sources_read_only_check",
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := repository.pool.Exec(ctx, test.statement, source.ID)
			if err == nil || !strings.Contains(err.Error(), test.constraint) {
				t.Fatalf("missing/mutable authority error=%v want constraint %s", err, test.constraint)
			}
		})
	}
}
