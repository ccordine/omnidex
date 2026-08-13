package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
)

func verifyLegacyPublicMigrationLedger(ctx context.Context, tx pgx.Tx, bundle MigrationBundle) error {
	legacy, err := legacyMigrationEntries(bundle)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT attributes.attname,format_type(attributes.atttypid,attributes.atttypmod),
		       attributes.attnotnull,COALESCE(pg_get_expr(defaults.adbin,defaults.adrelid,true),'')
		FROM pg_attribute attributes
		JOIN pg_class relations ON relations.oid=attributes.attrelid
		JOIN pg_namespace namespaces ON namespaces.oid=relations.relnamespace
		LEFT JOIN pg_attrdef defaults ON defaults.adrelid=relations.oid AND
		                                  defaults.adnum=attributes.attnum
		WHERE namespaces.nspname='public' AND relations.relname='schema_migrations' AND
		      attributes.attnum>0 AND NOT attributes.attisdropped
		ORDER BY attributes.attnum
	`)
	if err != nil {
		return fmt.Errorf("inspect legacy migration ledger columns: %w", err)
	}
	type column struct {
		name, dataType string
		notNull        bool
		defaultValue   string
	}
	columns := make([]column, 0, 2)
	for rows.Next() {
		var value column
		if err := rows.Scan(&value.name, &value.dataType, &value.notNull, &value.defaultValue); err != nil {
			rows.Close()
			return err
		}
		columns = append(columns, value)
	}
	rows.Close()
	if len(columns) != 2 || columns[0] != (column{"filename", "text", true, ""}) ||
		columns[1].name != "applied_at" || columns[1].dataType != "timestamp with time zone" ||
		!columns[1].notNull || columns[1].defaultValue != "now()" {
		return fmt.Errorf("legacy migration ledger is not the exact filename/applied_at contract")
	}
	appliedRows, err := tx.Query(ctx, `SELECT filename FROM public.schema_migrations ORDER BY filename`)
	if err != nil {
		return fmt.Errorf("load legacy migration ledger: %w", err)
	}
	applied := make([]string, 0, len(legacy))
	for appliedRows.Next() {
		var name string
		if err := appliedRows.Scan(&name); err != nil {
			appliedRows.Close()
			return err
		}
		applied = append(applied, name)
	}
	appliedRows.Close()
	if len(applied) != len(legacy) {
		return fmt.Errorf("legacy migration ledger has %d entries; expected %d", len(applied), len(legacy))
	}
	for index := range legacy {
		if applied[index] != legacy[index].name {
			return fmt.Errorf("legacy migration ledger differs at entry %d", index+1)
		}
	}
	return nil
}

func legacyMigrationEntries(bundle MigrationBundle) ([]migrationBundleEntry, error) {
	if err := bundle.validate(); err != nil {
		return nil, err
	}
	entries := make([]migrationBundleEntry, 0, legacyMigrationMaximumPrefix)
	seen := make(map[int]bool, legacyMigrationMaximumPrefix)
	for _, entry := range bundle.entries {
		prefix, err := migrationNumericPrefix(entry.name)
		if err != nil {
			return nil, err
		}
		if prefix > legacyMigrationMaximumPrefix {
			break
		}
		if seen[prefix] {
			return nil, fmt.Errorf("legacy migration prefix %03d is not singular", prefix)
		}
		seen[prefix] = true
		entries = append(entries, entry)
	}
	if len(entries) != legacyMigrationMaximumPrefix {
		return nil, fmt.Errorf("sealed bundle lacks the exact legacy 001..024 prefix")
	}
	for prefix := 1; prefix <= legacyMigrationMaximumPrefix; prefix++ {
		if !seen[prefix] {
			return nil, fmt.Errorf("sealed bundle lacks legacy migration prefix %03d", prefix)
		}
	}
	return entries, nil
}

func migrationNumericPrefix(name string) (int, error) {
	if len(name) < 3 {
		return 0, fmt.Errorf("migration name is too short")
	}
	return int(name[0]-'0')*100 + int(name[1]-'0')*10 + int(name[2]-'0'), nil
}

func loadLegacyExtensions(ctx context.Context, tx pgx.Tx) ([]legacyExtension, error) {
	rows, err := tx.Query(ctx, `
		SELECT extensions.oid,extensions.extname,extensions.extversion,
		       pg_get_userbyid(extensions.extowner),extensions.extrelocatable
		FROM pg_extension extensions
		JOIN pg_namespace namespaces ON namespaces.oid=extensions.extnamespace
		WHERE namespaces.nspname='public'
		ORDER BY extensions.extname
	`)
	if err != nil {
		return nil, fmt.Errorf("inspect legacy public extensions: %w", err)
	}
	defer rows.Close()
	values := make([]legacyExtension, 0, 3)
	for rows.Next() {
		var value legacyExtension
		if err := rows.Scan(&value.OID, &value.Name, &value.Version, &value.Owner, &value.Relocatable); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	expectedNames := []string{"pg_trgm", "pgcrypto", "vector"}
	expectedVersions := []string{"1.6", "1.3", "0.8.2"}
	if len(values) != len(expectedNames) {
		return nil, fmt.Errorf("legacy public extension inventory is not exact")
	}
	var currentUser string
	if err := tx.QueryRow(ctx, `SELECT current_user`).Scan(&currentUser); err != nil {
		return nil, err
	}
	for index, value := range values {
		if value.Name != expectedNames[index] || value.Version != expectedVersions[index] ||
			!value.Relocatable || value.Owner == "" {
			return nil, fmt.Errorf("legacy public extension %d violates its exact authority", index+1)
		}
		if value.Owner != currentUser {
			return nil, fmt.Errorf("legacy public extension %q is not owned by the database user", value.Name)
		}
	}
	return values, nil
}

func legacyExtensionSHA256(values []legacyExtension) string {
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	hash := sha256.New()
	for _, value := range values {
		writeLegacyCatalogHashField(hash, fmt.Sprintf("%d", value.OID))
		writeLegacyCatalogHashField(hash, value.Name)
		writeLegacyCatalogHashField(hash, value.Version)
		writeLegacyCatalogHashField(hash, value.Owner)
		writeLegacyCatalogHashField(hash, fmt.Sprintf("%t", value.Relocatable))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func legacyExtensionDescriptorSHA256(values []legacyExtension) string {
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	hash := sha256.New()
	for _, value := range values {
		writeLegacyCatalogHashField(hash, value.Name)
		writeLegacyCatalogHashField(hash, value.Version)
		writeLegacyCatalogHashField(hash, "owner=current_user")
		writeLegacyCatalogHashField(hash, fmt.Sprintf("%t", value.Relocatable))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
