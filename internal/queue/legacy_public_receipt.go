package queue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const legacyPublicCutoverReceiptSchema = "omnidex.legacy-public-cutover-receipt.v1"

// LegacyPublicCutoverReceipt is the exact durable result returned by the
// one-time preservation command. The same value is returned on a verified
// retry; ordinary runtime startup never consumes it.
type LegacyPublicCutoverReceipt struct {
	Schema                  string `json:"schema"`
	RuntimeSchema           string `json:"runtime_schema"`
	MigrationManifestSHA256 string `json:"migration_manifest_sha256"`
	LegacyCatalogSHA256     string `json:"legacy_catalog_sha256"`
	RuntimeCatalogSHA256    string `json:"runtime_catalog_sha256"`
	ObjectOIDsSHA256        string `json:"object_oids_sha256"`
	ExtensionsSHA256        string `json:"extensions_sha256"`
	RuntimeSchemaOID        uint32 `json:"runtime_schema_oid"`
	MigrationCount          int    `json:"migration_count"`
}

func (receipt LegacyPublicCutoverReceipt) validate(bundle MigrationBundle, runtimeSchema string) error {
	if receipt.Schema != legacyPublicCutoverReceiptSchema ||
		receipt.RuntimeSchema != runtimeSchema ||
		receipt.MigrationManifestSHA256 != bundle.manifestSHA256 ||
		receipt.MigrationManifestSHA256 != legacyExpectedMigrationManifestSHA256 ||
		receipt.LegacyCatalogSHA256 != legacyExpectedCatalogSHA256 ||
		receipt.RuntimeCatalogSHA256 != legacyExpectedRuntimeCatalogSHA256 ||
		!validMigrationDigest(receipt.ObjectOIDsSHA256) ||
		!validMigrationDigest(receipt.ExtensionsSHA256) ||
		receipt.RuntimeSchemaOID == 0 || receipt.MigrationCount != len(bundle.entries) {
		return fmt.Errorf("legacy public cutover receipt differs from the sealed command authority")
	}
	return nil
}

func (receipt LegacyPublicCutoverReceipt) exactJSON() (string, error) {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return "", fmt.Errorf("encode legacy public cutover receipt: %w", err)
	}
	return string(raw), nil
}

// JSON returns the canonical durable receipt representation printed by the
// explicit administrative command.
func (receipt LegacyPublicCutoverReceipt) JSON() (string, error) {
	return receipt.exactJSON()
}

func decodeLegacyPublicCutoverReceipt(raw string) (LegacyPublicCutoverReceipt, error) {
	if raw == "" || raw != strings.TrimSpace(raw) || len(raw) > 2048 {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf("legacy public cutover receipt is absent or unbounded")
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.DisallowUnknownFields()
	var receipt LegacyPublicCutoverReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf("decode legacy public cutover receipt: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf("legacy public cutover receipt has trailing JSON")
	}
	exact, err := receipt.exactJSON()
	if err != nil || exact != raw {
		return LegacyPublicCutoverReceipt{}, fmt.Errorf("legacy public cutover receipt is not canonical JSON")
	}
	return receipt, nil
}
