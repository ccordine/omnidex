package queue

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestLegacyPublicCutoverDerivesFrozenAuthorityFromCurrentProductionBundle(t *testing.T) {
	current := loadCheckedMigrationBundle(t)
	frozen, err := deriveLegacyCutoverBundle(current)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.ManifestSHA256() != legacyExpectedMigrationManifestSHA256 {
		t.Fatalf(
			"derived manifest=%q, want frozen %q",
			frozen.ManifestSHA256(), legacyExpectedMigrationManifestSHA256,
		)
	}
	if len(frozen.entries) >= len(current.entries) {
		t.Fatalf(
			"derived entries=%d, want a strict frozen prefix of current entries=%d",
			len(frozen.entries), len(current.entries),
		)
	}
	lastPrefix, err := migrationNumericPrefix(frozen.entries[len(frozen.entries)-1].name)
	if err != nil {
		t.Fatal(err)
	}
	if lastPrefix != legacyCutoverFinalMigrationPrefix {
		t.Fatalf("derived final prefix=%03d, want %03d", lastPrefix, legacyCutoverFinalMigrationPrefix)
	}
	again, err := deriveLegacyCutoverBundle(current)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again, frozen) {
		t.Fatal("current production bundle did not produce one deterministic frozen projection")
	}
}

func TestLegacyPublicCutoverRejectsChangedFrozenPrefixAuthority(t *testing.T) {
	changed := rejectedFrozenPrefixMigrationBundle(t, loadCheckedMigrationBundle(t))
	_, err := deriveLegacyCutoverBundle(changed)
	if err == nil || !strings.Contains(err.Error(), "release migration manifest differs") {
		t.Fatalf("changed frozen-prefix projection error=%v", err)
	}
}

func TestLegacyPublicCutoverRejectsFrozenPrefixOnlySourceBundle(t *testing.T) {
	current := loadCheckedMigrationBundle(t)
	frozen, err := deriveLegacyCutoverBundle(current)
	if err != nil {
		t.Fatal(err)
	}
	_, err = deriveLegacyCutoverBundle(frozen)
	if err == nil || !strings.Contains(
		err.Error(), "must contain verified migrations after frozen prefix 158",
	) {
		t.Fatalf("frozen-prefix-only source error=%v", err)
	}
}

func TestLegacyPublicTailProofRejectsFrozenPrefixOnlySourceBundle(t *testing.T) {
	current := loadCheckedMigrationBundle(t)
	frozen, err := deriveLegacyCutoverBundle(current)
	if err != nil {
		t.Fatal(err)
	}
	err = proveLegacyCutoverTailApplicability(context.Background(), nil, "", frozen)
	if err == nil || !strings.Contains(
		err.Error(), "current-tail applicability requires verified migrations after frozen prefix 158",
	) {
		t.Fatalf("frozen-prefix-only tail proof error=%v", err)
	}
}
