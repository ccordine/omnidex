package queue

import (
	"fmt"
	"testing"
	"time"
)

func TestScrumCardTagCatalogIsProjectScopedFilteredAndBoundedInSQL(t *testing.T) {
	repository, _, ctx := replanTestRepository(t)
	project, err := repository.CreateProject(ctx, fmt.Sprintf("tag-catalog-%d", time.Now().UnixNano()), t.TempDir(), "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := repository.CreateProject(ctx, fmt.Sprintf("tag-catalog-other-%d", time.Now().UnixNano()), t.TempDir(), "", "", nil)
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
	tags, err := repository.ListScrumCardTags(ctx, project.ID, "go", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0] != "go" || tags[1] != "gopher" {
		t.Fatalf("tags=%v", tags)
	}
}
