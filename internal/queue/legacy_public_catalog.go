package queue

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type legacyCatalogSnapshot struct {
	SHA256           string
	ObjectOIDsSHA256 string
	Tables           int
	Sequences        int
	Indexes          int
	Functions        int
	Triggers         int
}

type legacyCatalogEntry struct {
	kind, identity, detail string
	oid                    uint32
}

func loadLegacyCatalogSnapshot(
	ctx context.Context,
	tx pgx.Tx,
	schema string,
) (legacyCatalogSnapshot, error) {
	queries := []string{
		legacyRelationsCatalogSQL,
		legacyColumnsCatalogSQL,
		legacyConstraintsCatalogSQL,
		legacyIndexesCatalogSQL,
		legacySequencesCatalogSQL,
		legacyFunctionsCatalogSQL,
		legacyTriggersCatalogSQL,
	}
	entries := make([]legacyCatalogEntry, 0, 512)
	for _, query := range queries {
		rows, err := tx.Query(ctx, query, schema)
		if err != nil {
			return legacyCatalogSnapshot{}, fmt.Errorf("read legacy catalog: %w", err)
		}
		for rows.Next() {
			var entry legacyCatalogEntry
			if err := rows.Scan(&entry.kind, &entry.identity, &entry.detail, &entry.oid); err != nil {
				rows.Close()
				return legacyCatalogSnapshot{}, fmt.Errorf("scan legacy catalog: %w", err)
			}
			entry.detail = normalizeLegacyCatalogDefinition(entry.detail, schema)
			entries = append(entries, entry)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return legacyCatalogSnapshot{}, fmt.Errorf("iterate legacy catalog: %w", err)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		left := entries[i].kind + "\x00" + entries[i].identity + "\x00" + entries[i].detail
		right := entries[j].kind + "\x00" + entries[j].identity + "\x00" + entries[j].detail
		return left < right
	})
	snapshot := legacyCatalogSnapshot{}
	catalogHash := sha256.New()
	oidHash := sha256.New()
	for _, entry := range entries {
		writeLegacyCatalogHashField(catalogHash, entry.kind)
		writeLegacyCatalogHashField(catalogHash, entry.identity)
		writeLegacyCatalogHashField(catalogHash, entry.detail)
		if entry.oid != 0 && (entry.kind == "relation" || entry.kind == "function") {
			writeLegacyCatalogHashField(oidHash, entry.kind)
			writeLegacyCatalogHashField(oidHash, entry.identity)
			var raw [4]byte
			binary.BigEndian.PutUint32(raw[:], entry.oid)
			_, _ = oidHash.Write(raw[:])
		}
		switch entry.kind {
		case "relation":
			if strings.Contains(entry.detail, `"relkind": "r"`) {
				snapshot.Tables++
			} else if strings.Contains(entry.detail, `"relkind": "S"`) {
				snapshot.Sequences++
			} else if strings.Contains(entry.detail, `"relkind": "i"`) {
				snapshot.Indexes++
			}
		case "function":
			snapshot.Functions++
		case "trigger":
			snapshot.Triggers++
		}
	}
	snapshot.SHA256 = hex.EncodeToString(catalogHash.Sum(nil))
	snapshot.ObjectOIDsSHA256 = hex.EncodeToString(oidHash.Sum(nil))
	return snapshot, nil
}

func normalizeLegacyCatalogDefinition(value, schema string) string {
	value = strings.ReplaceAll(value, `\\`+schema+`.`, `\\@schema@.`)
	value = strings.ReplaceAll(value, `"`+schema+`".`, `"@schema@".`)
	value = strings.ReplaceAll(value, schema+`.`, `@schema@.`)
	value = strings.ReplaceAll(value, `"`+schema+`"`, `"@schema@"`)
	value = strings.ReplaceAll(value, `SET search_path TO pg_catalog, `+schema,
		`SET search_path TO pg_catalog, @schema@`)
	value = strings.ReplaceAll(value, `pg_catalog.gen_random_uuid()`, `gen_random_uuid()`)
	if schema == legacyPublicSchema {
		value = strings.ReplaceAll(value, `"default": "@schema@.gen_random_uuid()"`,
			`"default": "gen_random_uuid()"`)
	}
	return value
}

type legacyCatalogHasher interface{ Write([]byte) (int, error) }

func writeLegacyCatalogHashField(hash legacyCatalogHasher, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = hash.Write(size[:])
	_, _ = hash.Write([]byte(value))
}

const legacyExtensionExclusionSQL = `NOT EXISTS (
 SELECT 1 FROM pg_depend dependency
 WHERE dependency.classid='pg_class'::regclass AND dependency.objid=objects.oid AND
       dependency.deptype='e'
)`

const legacyRelationsCatalogSQL = `
SELECT 'relation',objects.relname,
       jsonb_build_object(
         'relkind',objects.relkind,'persistence',objects.relpersistence,
         'access_method',COALESCE(methods.amname,''),'row_security',objects.relrowsecurity,
         'force_row_security',objects.relforcerowsecurity,'replica_identity',objects.relreplident,
         'options',COALESCE(to_jsonb(objects.reloptions),'[]'::jsonb),
         'partition_bound',COALESCE(pg_get_expr(objects.relpartbound,objects.oid,true),'')
       )::text,objects.oid
FROM pg_class objects
JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
LEFT JOIN pg_am methods ON methods.oid=objects.relam
WHERE namespaces.nspname=$1 AND ` + legacyExtensionExclusionSQL + `
ORDER BY objects.relname,objects.relkind`

const legacyColumnsCatalogSQL = `
SELECT 'column',objects.relname||':'||attributes.attnum::text,
       jsonb_build_object(
         'name',attributes.attname,'dropped',attributes.attisdropped,
         'type',format_type(attributes.atttypid,attributes.atttypmod),
         'not_null',attributes.attnotnull,'identity',attributes.attidentity,
         'generated',attributes.attgenerated,'dimensions',attributes.attndims,
         'collation',COALESCE(collations.collname,''),'storage',attributes.attstorage,
         'compression',attributes.attcompression,'statistics',attributes.attstattarget,
         'default',COALESCE(pg_get_expr(defaults.adbin,defaults.adrelid,true),'')
       )::text,0::oid
FROM pg_class objects
JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
JOIN pg_attribute attributes ON attributes.attrelid=objects.oid AND attributes.attnum>0
LEFT JOIN pg_attrdef defaults ON defaults.adrelid=objects.oid AND defaults.adnum=attributes.attnum
LEFT JOIN pg_collation collations ON collations.oid=attributes.attcollation
WHERE namespaces.nspname=$1 AND ` + legacyExtensionExclusionSQL + `
ORDER BY objects.relname,attributes.attnum`

const legacyConstraintsCatalogSQL = `
SELECT 'constraint',objects.relname||':'||constraints.conname,
       jsonb_build_object(
         'type',constraints.contype,'definition',pg_get_constraintdef(constraints.oid,true),
         'deferrable',constraints.condeferrable,'deferred',constraints.condeferred,
         'validated',constraints.convalidated,'no_inherit',constraints.connoinherit
       )::text,0::oid
FROM pg_constraint constraints
JOIN pg_class objects ON objects.oid=constraints.conrelid
JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
WHERE namespaces.nspname=$1
ORDER BY objects.relname,constraints.conname`

const legacyIndexesCatalogSQL = `
SELECT 'index',indexes.relname,
       jsonb_build_object(
         'table',tables.relname,'definition',pg_get_indexdef(indexes.oid),
         'unique',metadata.indisunique,'primary',metadata.indisprimary,
         'valid',metadata.indisvalid,'ready',metadata.indisready,
         'live',metadata.indislive,'replica_identity',metadata.indisreplident
       )::text,0::oid
FROM pg_index metadata
JOIN pg_class indexes ON indexes.oid=metadata.indexrelid
JOIN pg_class tables ON tables.oid=metadata.indrelid
JOIN pg_namespace namespaces ON namespaces.oid=tables.relnamespace
WHERE namespaces.nspname=$1
ORDER BY indexes.relname`

const legacySequencesCatalogSQL = `
SELECT 'sequence',objects.relname,
		       jsonb_build_object(
		         'type',format_type(sequences.seqtypid,NULL),'start',sequences.seqstart,
		         'increment',sequences.seqincrement,'maximum',sequences.seqmax,
		         'minimum',sequences.seqmin,'cache',sequences.seqcache,'cycle',sequences.seqcycle
		       )::text,0::oid
FROM pg_sequence sequences
JOIN pg_class objects ON objects.oid=sequences.seqrelid
JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
WHERE namespaces.nspname=$1
ORDER BY objects.relname`

const legacyFunctionsCatalogSQL = `
SELECT 'function',procedures.proname||'('||pg_get_function_identity_arguments(procedures.oid)||')',
       jsonb_build_object(
         'definition',pg_get_functiondef(procedures.oid),'kind',procedures.prokind,
         'volatile',procedures.provolatile,'parallel',procedures.proparallel,
         'strict',procedures.proisstrict,'security_definer',procedures.prosecdef,
         'leakproof',procedures.proleakproof,'config',COALESCE(to_jsonb(procedures.proconfig),'[]'::jsonb)
       )::text,procedures.oid
FROM pg_proc procedures
JOIN pg_namespace namespaces ON namespaces.oid=procedures.pronamespace
WHERE namespaces.nspname=$1 AND NOT EXISTS (
 SELECT 1 FROM pg_depend dependency
 WHERE dependency.classid='pg_proc'::regclass AND dependency.objid=procedures.oid AND
       dependency.deptype='e'
)
ORDER BY procedures.proname,pg_get_function_identity_arguments(procedures.oid)`

const legacyTriggersCatalogSQL = `
SELECT 'trigger',objects.relname||':'||triggers.tgname,
       jsonb_build_object(
         'definition',pg_get_triggerdef(triggers.oid,true),'enabled',triggers.tgenabled,
         'type',triggers.tgtype
       )::text,triggers.oid
FROM pg_trigger triggers
JOIN pg_class objects ON objects.oid=triggers.tgrelid
JOIN pg_namespace namespaces ON namespaces.oid=objects.relnamespace
WHERE namespaces.nspname=$1 AND NOT triggers.tgisinternal
ORDER BY objects.relname,triggers.tgname`
