package queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) GetDataSource(ctx context.Context, id string) (DataSourceRecord, error) {
	if err := validateDataSourceID(id); err != nil {
		return DataSourceRecord{}, err
	}
	return scanDataSource(r.pool.QueryRow(ctx, `
		SELECT `+dataSourceSelectColumns+`
		FROM data_sources
		WHERE id=$1
	`, id))
}

func (r *Repository) CreateDataSource(
	ctx context.Context,
	input DataSourceUpsert,
) (DataSourceRecord, error) {
	id, err := newDataSourceID()
	if err != nil {
		return DataSourceRecord{}, err
	}
	now := time.Now().UTC()
	record := canonicalizeDataSourceRecord(DataSourceRecord{
		ID:           string(id),
		Name:         input.Name,
		Driver:       input.Driver,
		Host:         input.Host,
		Port:         input.Port,
		DatabaseName: input.DatabaseName,
		Username:     input.Username,
		Password:     input.Password,
		SSLMode:      input.SSLMode,
		UseDSN:       input.UseDSN,
		DSN:          input.DSN,
		ReadOnly:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err := validateDataSourceRecord(record); err != nil {
		return DataSourceRecord{}, err
	}
	return scanDataSource(r.pool.QueryRow(ctx, `
		INSERT INTO data_sources (
			id,name,driver,host,port,
			database_name,username,password,ssl_mode,use_dsn,dsn,read_only,
			last_test_status,last_test_message,last_test_at,catalog_updated_at,
			created_at,updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,
			$13,$14,$15,$16,$17,$18
		)
		RETURNING `+dataSourceSelectColumns,
		record.ID, record.Name, record.Driver, record.Host, record.Port, record.DatabaseName,
		record.Username, record.Password, record.SSLMode, record.UseDSN, record.DSN,
		record.ReadOnly, record.LastTestStatus, record.LastTestMessage,
		record.LastTestAt, record.CatalogUpdatedAt, record.CreatedAt, record.UpdatedAt,
	))
}

func (r *Repository) UpdateDataSource(
	ctx context.Context,
	id string,
	input DataSourceUpsert,
) (DataSourceRecord, error) {
	if err := validateDataSourceID(id); err != nil {
		return DataSourceRecord{}, err
	}
	current, err := r.GetDataSource(ctx, id)
	if err != nil {
		return DataSourceRecord{}, err
	}
	password := input.Password
	if strings.TrimSpace(password) == "" {
		password = current.Password
	}
	dsnInput := strings.TrimSpace(input.DSN)
	dsn := dsnInput
	if dsn == "" {
		dsn = current.DSN
	}
	next := canonicalizeDataSourceRecord(DataSourceRecord{
		ID:           id,
		Name:         input.Name,
		Driver:       input.Driver,
		Host:         input.Host,
		Port:         input.Port,
		DatabaseName: input.DatabaseName,
		Username:     input.Username,
		Password:     password,
		SSLMode:      input.SSLMode,
		UseDSN:       input.UseDSN,
		DSN:          dsn,
		ReadOnly:     true,
		CreatedAt:    current.CreatedAt,
		UpdatedAt:    time.Now().UTC(),
	})
	if err := validateDataSourceRecord(next); err != nil {
		return DataSourceRecord{}, err
	}
	return scanDataSource(r.pool.QueryRow(ctx, `
		UPDATE data_sources
		SET name=$2, driver=$3, host=$4, port=$5, database_name=$6, username=$7,
		    password=CASE WHEN btrim($8)='' THEN password ELSE $8 END,
		    ssl_mode=$9, use_dsn=$10,
		    dsn=CASE WHEN btrim($11)='' THEN dsn ELSE $11 END,
		    updated_at=NOW()
		WHERE id=$1
		RETURNING `+dataSourceSelectColumns,
		id, next.Name, next.Driver, next.Host, next.Port, next.DatabaseName,
		next.Username, input.Password, next.SSLMode, next.UseDSN, dsnInput,
	))
}

func (r *Repository) DeleteDataSource(ctx context.Context, id string) error {
	if err := validateDataSourceID(id); err != nil {
		return err
	}
	command, err := r.pool.Exec(ctx, `DELETE FROM data_sources WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *Repository) UpdateDataSourceTestResult(
	ctx context.Context,
	id, status, message string,
) (DataSourceRecord, error) {
	if err := validateDataSourceID(id); err != nil {
		return DataSourceRecord{}, err
	}
	return scanDataSource(r.pool.QueryRow(ctx, `
		UPDATE data_sources
		SET last_test_status=$2, last_test_message=$3, last_test_at=NOW(), updated_at=NOW()
		WHERE id=$1
		RETURNING `+dataSourceSelectColumns,
		id, strings.TrimSpace(status), strings.TrimSpace(message),
	))
}

func newDataSourceID() (model.DataSourceID, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate data source identity: %w", err)
	}
	id := model.DataSourceID(hex.EncodeToString(buf))
	if err := id.Validate(); err != nil {
		return "", err
	}
	return id, nil
}

func canonicalizeDataSourceRecord(record DataSourceRecord) DataSourceRecord {
	record.Name = strings.TrimSpace(record.Name)
	record.Driver = strings.ToLower(strings.TrimSpace(record.Driver))
	record.Host = strings.TrimSpace(record.Host)
	record.DatabaseName = strings.TrimSpace(record.DatabaseName)
	record.Username = strings.TrimSpace(record.Username)
	record.SSLMode = strings.ToLower(strings.TrimSpace(record.SSLMode))
	record.DSN = strings.TrimSpace(record.DSN)
	return record
}
