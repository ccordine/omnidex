package queue

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestScrumCardTagCatalogIsProjectScopedFilteredAndBoundedInSQL(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	project, err := repository.CreateProject(ctx, fmt.Sprintf("tag-catalog-%d", time.Now().UnixNano()), t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := repository.CreateProject(ctx, fmt.Sprintf("tag-catalog-other-%d", time.Now().UnixNano()), t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	for id, tags := range map[string]string{
		"one": `["Go","frontend"]`, "two": `["go","gopher"]`, "three": `["ignored"]`,
	} {
		if _, err := repository.pool.Exec(ctx, `
			INSERT INTO scrum_cards(id,project_id,title,column_name,tags) VALUES($1,$2,$1,'assigned',$3::jsonb)
		`, id, project.ID, tags); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO scrum_cards(id,project_id,title,column_name,tags) VALUES('foreign',$1,'foreign','assigned','["go-foreign"]'::jsonb)
	`, other.ID); err != nil {
		t.Fatal(err)
	}
	tags, err := repository.ListScrumCardTags(ctx, project.ID, "GO", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0] != "go" || tags[1] != "gopher" {
		t.Fatalf("tags=%v", tags)
	}
	tags, err = repository.ListScrumCardTags(ctx, project.ID, " go ", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Fatalf("space-bearing exact query was silently trimmed: %v", tags)
	}
	for _, query := range []string{"bad\x00tag", strings.Repeat("x", 257)} {
		if _, err := repository.ListScrumCardTags(ctx, project.ID, query, 2); err == nil {
			t.Fatalf("invalid query %q was accepted", query)
		}
	}
	for _, limit := range []int{0, -1, MaxScrumTagPageSize + 1} {
		if _, err := repository.ListScrumCardTags(ctx, project.ID, "go", limit); err == nil {
			t.Fatalf("invalid limit %d was accepted", limit)
		}
	}
}
