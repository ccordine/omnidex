package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/queue"
)

type legacyPublicPreservationConfig struct {
	databaseURL   string
	runtimeSchema string
}

func loadLegacyPublicPreservationConfig() (legacyPublicPreservationConfig, error) {
	databaseURL, configured := os.LookupEnv("DATABASE_URL")
	if !configured || strings.TrimSpace(databaseURL) == "" {
		return legacyPublicPreservationConfig{}, fmt.Errorf("DATABASE_URL is required")
	}
	if databaseURL != strings.TrimSpace(databaseURL) {
		return legacyPublicPreservationConfig{}, fmt.Errorf("DATABASE_URL must not contain surrounding whitespace")
	}
	runtimeSchema, configured := os.LookupEnv("DATABASE_SCHEMA")
	if !configured {
		runtimeSchema = db.DefaultRuntimeSchema
	} else if runtimeSchema == "" {
		return legacyPublicPreservationConfig{}, fmt.Errorf("DATABASE_SCHEMA is explicitly empty")
	}
	if err := db.ValidateRuntimeSchemaName(runtimeSchema); err != nil {
		return legacyPublicPreservationConfig{}, fmt.Errorf("DATABASE_SCHEMA: %w", err)
	}
	if raw, configured := os.LookupEnv("WRAPPER_ONLY"); configured {
		wrapperOnly, err := strconv.ParseBool(raw)
		if err != nil {
			return legacyPublicPreservationConfig{}, fmt.Errorf("WRAPPER_ONLY must be one boolean: %w", err)
		}
		if wrapperOnly {
			return legacyPublicPreservationConfig{}, fmt.Errorf("legacy public preservation requires the database-backed core")
		}
	}
	return legacyPublicPreservationConfig{databaseURL: databaseURL, runtimeSchema: runtimeSchema}, nil
}

func runLegacyPublicPreservationCommand() error {
	cfg, err := loadLegacyPublicPreservationConfig()
	if err != nil {
		return fmt.Errorf("load command configuration: %w", err)
	}
	bundle, err := loadCoreMigrationBundle()
	if err != nil {
		return fmt.Errorf("load sealed migration authority: %w", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	pool, err := db.ConnectLegacyPublicCutover(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	receipt, err := queue.PreserveLegacyPublic(ctx, pool, cfg.runtimeSchema, bundle)
	if err != nil {
		return err
	}
	raw, err := receipt.JSON()
	if err != nil {
		return err
	}
	fmt.Println(raw)
	return nil
}
