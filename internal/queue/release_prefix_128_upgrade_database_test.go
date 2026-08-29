package queue

import "testing"

func TestReleaseUpgradeFromProductionPrefix128ToSealedBundle(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "128"),
	); err != nil {
		t.Fatalf("install production migration prefix 128: %v", err)
	}

	bundle := loadCheckedMigrationBundle(t)
	if err := repository.EnsureSchema(t.Context(), bundle); err != nil {
		t.Fatalf("upgrade production prefix 128 to sealed bundle: %v", err)
	}
	assertExactMigrationLedger(t, pool, bundle)
	if err := repository.ValidateRuntimeAuthority(t.Context()); err != nil {
		t.Fatalf("validate upgraded runtime authority: %v", err)
	}

	var latest string
	if err := pool.QueryRow(t.Context(), `SELECT MAX(filename) FROM schema_migrations`).Scan(&latest); err != nil {
		t.Fatal(err)
	}
	if want := bundle.entries[len(bundle.entries)-1].name; latest != want {
		t.Fatalf("latest migrated file=%q want %q", latest, want)
	}
}
