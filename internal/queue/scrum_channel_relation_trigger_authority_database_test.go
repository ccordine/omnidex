package queue

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPostgresScrumMessageRelationOwnsRowsCountersAndDeletion(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "089")); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(t.Context(), "message-authority", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		t.Context(), project.ID, "message-card", "Message card", "", "assigned", nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO scrum_cards(
		 id,project_id,title,column_name,channel_message_count,channel_content_bytes
		) VALUES('forged-counter-card',$1,'Forged counters','assigned',1,10)
	`, project.ID); err == nil || !strings.Contains(err.Error(), "must start with empty relation-owned channel counters") {
		t.Fatalf("forged new-card counters error=%v", err)
	}

	var ordinal, count, contentBytes int64
	var createdAt, insertedAt time.Time
	var sourceCreatedAt, origin string
	if err := pool.QueryRow(t.Context(), `
		INSERT INTO scrum_card_messages(project_id,card_id,message_id,role,content,status)
		VALUES($1,$2,'message-1','assistant','exact bytes','')
		RETURNING ordinal,created_at,inserted_at,source_created_at,timestamp_origin
	`, project.ID, card.ID).Scan(
		&ordinal, &createdAt, &insertedAt, &sourceCreatedAt, &origin,
	); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT channel_message_count,channel_content_bytes
		FROM scrum_cards WHERE project_id=$1 AND id=$2
	`, project.ID, card.ID).Scan(&count, &contentBytes); err != nil {
		t.Fatal(err)
	}
	if ordinal != 1 || count != 1 || contentBytes != int64(len("exact bytes")) ||
		!createdAt.Equal(insertedAt) || sourceCreatedAt != createdAt.UTC().Format(time.RFC3339Nano) || origin != "runtime" {
		t.Fatalf(
			"ordinal/count/bytes=%d/%d/%d timestamps=%s/%s source=%q origin=%q",
			ordinal, count, contentBytes, createdAt, insertedAt, sourceCreatedAt, origin,
		)
	}

	for name, statement := range map[string]string{
		"counter": `UPDATE scrum_cards SET channel_message_count=2 WHERE project_id=$1 AND id=$2`,
		"update":  `UPDATE scrum_card_messages SET content='changed' WHERE project_id=$1 AND card_id=$2`,
		"delete":  `DELETE FROM scrum_card_messages WHERE project_id=$1 AND card_id=$2`,
	} {
		if _, err := pool.Exec(t.Context(), statement, project.ID, card.ID); err == nil {
			t.Fatalf("%s mutation unexpectedly erased or forged message authority", name)
		}
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM scrum_cards WHERE project_id=$1 AND id=$2`, project.ID, card.ID); err != nil {
		t.Fatalf("delete card with cascade-owned channel cleanup: %v", err)
	}
	var remaining int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM scrum_card_messages WHERE project_id=$1 AND card_id=$2
	`, project.ID, card.ID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("deleted card retained %d channel messages", remaining)
	}
}

func TestPostgresScrumMessageFunctionsIgnorePGTempAndNestedTriggers(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "089")); err != nil {
		t.Fatal(err)
	}
	project, err := repository.CreateProject(t.Context(), "message-shadow", t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(
		t.Context(), project.ID, "shadow-card", "Shadow card", "", "assigned", nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	var runtimeSchema string
	if err := conn.QueryRow(t.Context(), `SELECT current_schema()`).Scan(&runtimeSchema); err != nil {
		t.Fatal(err)
	}
	runtimeMessages := pgx.Identifier{runtimeSchema, "scrum_card_messages"}.Sanitize()
	if _, err := conn.Exec(t.Context(), `
		CREATE TEMP TABLE scrum_cards(project_id bigint,id text,channel_message_count bigint);
		INSERT INTO scrum_cards VALUES(1,'shadow-card',9007199254740991);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), `INSERT INTO `+runtimeMessages+`(
		project_id,card_id,message_id,role,content,status
	) VALUES($1,$2,'shadow-safe','system','runtime relation','')`, project.ID, card.ID); err != nil {
		t.Fatalf("pinned message trigger resolved pg_temp shadow: %v", err)
	}

	attackFunction := `CREATE FUNCTION pg_temp.attack_scrum_message() RETURNS trigger LANGUAGE plpgsql AS $attack$
	BEGIN
	  DELETE FROM ` + runtimeMessages + ` WHERE project_id=` + runtimeMessages + `.project_id;
	  RETURN NEW;
	END $attack$`
	if _, err := conn.Exec(t.Context(), `
		CREATE TEMP TABLE attack_gate(value integer);
		`+attackFunction+`;
		CREATE TRIGGER attack BEFORE INSERT ON attack_gate
		FOR EACH ROW EXECUTE FUNCTION pg_temp.attack_scrum_message();
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(t.Context(), `INSERT INTO attack_gate VALUES(1)`); err == nil ||
		!strings.Contains(err.Error(), "append-only") {
		t.Fatalf("nested trigger delete error=%v", err)
	}
	var count int64
	if err := pool.QueryRow(t.Context(), `SELECT channel_message_count FROM scrum_cards WHERE project_id=$1 AND id=$2`, project.ID, card.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("nested or shadow attack changed authoritative count=%d", count)
	}
}
