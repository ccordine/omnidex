package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/queue"
)

const sealedDatabaseMigrationReceiptSchema = "omnidex.sealed-database-migration-receipt.v1"

type sealedDatabaseMigrationReceipt struct {
	Schema                string `json:"schema"`
	ManifestSHA256        string `json:"manifest_sha256"`
	RuntimeAuthorityValid bool   `json:"runtime_authority_valid"`
}

func runSealedDatabaseMigrationCommand() error {
	return runSealedDatabaseMigrationCommandTo(os.Stdout)
}

func runSealedDatabaseMigrationCommandTo(output io.Writer) error {
	if output == nil {
		return fmt.Errorf("sealed database migration receipt output is required")
	}
	cfg, err := loadCoreDatabaseCommandConfig()
	if err != nil {
		return fmt.Errorf("load command configuration: %w", err)
	}
	bundle, err := loadCoreMigrationBundle()
	if err != nil {
		return fmt.Errorf("load sealed migration authority: %w", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := db.ConnectRuntime(ctx, cfg.databaseURL, cfg.runtimeSchema)
	if err != nil {
		return fmt.Errorf("connect runtime database: %w", err)
	}
	defer pool.Close()
	repository := queue.New(pool)
	if err := repository.EnsureSchema(ctx, bundle); err != nil {
		return fmt.Errorf("apply sealed migration bundle: %w", err)
	}
	if err := repository.ValidateRuntimeAuthority(ctx); err != nil {
		return fmt.Errorf("validate migrated runtime authority: %w", err)
	}
	if err := writeSealedDatabaseMigrationReceipt(output, bundle.ManifestSHA256()); err != nil {
		return err
	}
	return nil
}

func writeSealedDatabaseMigrationReceipt(output io.Writer, manifestSHA256 string) error {
	if output == nil {
		return fmt.Errorf("sealed database migration receipt output is required")
	}
	receipt, err := json.Marshal(sealedDatabaseMigrationReceipt{
		Schema:         sealedDatabaseMigrationReceiptSchema,
		ManifestSHA256: manifestSHA256, RuntimeAuthorityValid: true,
	})
	if err != nil {
		return fmt.Errorf("encode sealed migration receipt: %w", err)
	}
	if _, err := fmt.Fprintln(output, string(receipt)); err != nil {
		return fmt.Errorf("write sealed migration receipt: %w", err)
	}
	return nil
}
