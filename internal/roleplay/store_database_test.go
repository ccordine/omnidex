package roleplay

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRoleplayCanonPersistsAndCharacterProjectionCannotSeeUnknownFacts(t *testing.T) {
	pool, reopen := openRoleplayTestPool(t)
	ctx := context.Background()
	store, err := NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}
	world, bob := bootstrapRoleplayChannel(
		t, pool, "story-one", "Harbor Kingdom", "Bob", 101, 102, 103,
	)
	_, outsider := bootstrapRoleplayChannel(
		t, pool, "story-two", "Mountain Kingdom", "Outsider", 201,
	)

	alice, err := store.CreateCharacter(ctx, world.ID, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := store.AppendCanonEvent(ctx, world.ID, 101, "The crown is hidden beneath the west gate.")
	if err != nil {
		t.Fatal(err)
	}
	public, err := store.AppendCanonEvent(ctx, world.ID, 102, "Rain began over the harbor.")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendCanonEvent(
		ctx, world.ID, 103, "The crown is hidden beneath the west gate.",
	); err == nil {
		t.Fatal("duplicate exact canon content was accepted in one world")
	}
	if _, created, err := store.GrantKnowledge(ctx, alice.ID, secret.ID); err != nil || !created {
		t.Fatalf("grant Alice secret: created=%t err=%v", created, err)
	}
	if _, created, err := store.GrantKnowledge(ctx, alice.ID, public.ID); err != nil || !created {
		t.Fatalf("grant Alice public fact: created=%t err=%v", created, err)
	}
	firstGrant, created, err := store.GrantKnowledge(ctx, bob.ID, public.ID)
	if err != nil || !created {
		t.Fatalf("grant Bob public fact: created=%t err=%v", created, err)
	}
	repeatedGrant, created, err := store.GrantKnowledge(ctx, bob.ID, public.ID)
	if err != nil || created || repeatedGrant.ID != firstGrant.ID {
		t.Fatalf("repeat grant: first=%q repeated=%q created=%t err=%v", firstGrant.ID, repeatedGrant.ID, created, err)
	}

	aliceContext, err := store.ProjectCharacterContext(ctx, alice.ID, MaxProjectionEvents)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectionContents(aliceContext); got != "The crown is hidden beneath the west gate.|Rain began over the harbor." {
		t.Fatalf("Alice context=%q", got)
	}
	bobContext, err := store.ProjectCharacterContext(ctx, bob.ID, MaxProjectionEvents)
	if err != nil {
		t.Fatal(err)
	}
	if got := projectionContents(bobContext); got != "Rain began over the harbor." {
		t.Fatalf("Bob context=%q", got)
	}
	if strings.Contains(projectionContents(bobContext), "crown") {
		t.Fatal("Bob received an unknown canon fact")
	}
	if _, err := store.ProjectChannelCharacterContext(
		ctx, "story-two", bob.ID, MaxProjectionEvents,
	); err == nil {
		t.Fatal("a character was projected through a different channel authority")
	}
	canon, err := store.ProjectCanonContext(ctx, world.ID, MaxProjectionEvents)
	if err != nil {
		t.Fatal(err)
	}
	if len(canon.Facts) != 2 || canon.Authority != AuthorityFictionalCanon {
		t.Fatalf("canon=%#v", canon)
	}

	if _, err := store.AppendCanonEvent(
		ctx, world.ID, 201, "This source belongs to another story.",
	); err == nil || !strings.Contains(err.Error(), "assistant message in the world channel") {
		t.Fatalf("cross-channel canon source error=%v", err)
	}
	if _, _, err := store.GrantKnowledge(ctx, outsider.ID, secret.ID); err == nil || !strings.Contains(err.Error(), "different fictional worlds") {
		t.Fatalf("cross-world knowledge error=%v", err)
	}

	if _, err := pool.Exec(ctx, `UPDATE roleplay_canon_events SET content='changed' WHERE id=$1`, secret.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("canon update error=%v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM roleplay_canon_events WHERE id=$1`, secret.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("canon delete error=%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE roleplay_character_knowledge SET character_id=$2 WHERE id=$1`, firstGrant.ID, alice.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("knowledge update error=%v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE roleplay_canon_events CASCADE`); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("canon truncate error=%v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE roleplay_character_knowledge`); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("knowledge truncate error=%v", err)
	}

	pool.Close()
	reopenedPool := reopen(t)
	t.Cleanup(reopenedPool.Close)
	reopenedStore, err := NewStore(reopenedPool)
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := reopenedStore.ProjectCharacterContext(ctx, bob.ID, MaxProjectionEvents)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Fingerprint != bobContext.Fingerprint || projectionContents(persisted) != "Rain began over the harbor." {
		t.Fatalf("persisted Bob context=%#v", persisted)
	}
}

func openRoleplayTestPool(t *testing.T) (*pgxpool.Pool, func(*testing.T) *pgxpool.Pool) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL roleplay tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "roleplay_" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	var authoritySchema string
	if err := admin.QueryRow(ctx, `SELECT 'omnidex_host_authority_' || md5($1)`, schema).Scan(
		&authoritySchema,
	); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	newPool := func(t *testing.T) *pgxpool.Pool {
		config, err := pgxpool.ParseConfig(databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
		pool, err := pgxpool.NewWithConfig(context.Background(), config)
		if err != nil {
			t.Fatal(err)
		}
		return pool
	}
	pool := newPool(t)
	setupPath, err := filepath.Abs(filepath.Join("..", "..", "database", "setup.sql"))
	if err != nil {
		pool.Close()
		admin.Close()
		t.Fatal(err)
	}
	setup, err := os.ReadFile(setupPath)
	if err != nil {
		pool.Close()
		admin.Close()
		t.Fatal(err)
	}
	setup = []byte(strings.ReplaceAll(
		string(setup), "__OMNIDEX_RUNTIME_SCHEMA__", schema,
	))
	if _, err := pool.Exec(ctx, string(setup)); err != nil {
		pool.Close()
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := admin.Exec(cleanupCtx,
			"DROP SCHEMA IF EXISTS "+pgx.Identifier{authoritySchema}.Sanitize()+" CASCADE",
		); err != nil {
			t.Errorf("drop roleplay authority schema: %v", err)
		}
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("drop roleplay test schema: %v", err)
		}
		admin.Close()
	})
	return pool, newPool
}

func bootstrapRoleplayChannel(
	t *testing.T,
	pool *pgxpool.Pool,
	channelID, worldName, viewpointName string,
	messageIDs ...int64,
) (World, Character) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	worldID, err := NewWorldIdentity()
	if err != nil {
		t.Fatal(err)
	}
	viewpointID, err := NewCharacterIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ai_channels (id,mode,roleplay_viewpoint_character_id)
		VALUES ($1,'roleplay',$2)
	`, channelID, viewpointID); err != nil {
		t.Fatal(err)
	}
	world, viewpoint, err := BootstrapWorldTx(
		ctx, tx, channelID, worldID, worldName, viewpointID, viewpointName,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range messageIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ai_channel_messages (id,channel_id,role,content)
			VALUES ($1,$2,'assistant','turn')
		`, id, channelID); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return world, viewpoint
}

func projectionContents(projection CharacterProjection) string {
	values := make([]string, 0, len(projection.Facts))
	for _, fact := range projection.Facts {
		values = append(values, fact.Content)
	}
	return strings.Join(values, "|")
}
