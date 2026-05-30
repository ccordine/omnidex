package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProjectLocationFromMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata string
		want     string
	}{
		{
			name:     "uses client cwd",
			metadata: `{"client_cwd":"/home/gryph/Projects/ai/omnidex"}`,
			want:     "/home/gryph/Projects/ai/omnidex",
		},
		{
			name:     "falls back to host cwd",
			metadata: `{"host_env_cwd":"/tmp/work"}`,
			want:     "/tmp/work",
		},
		{
			name:     "ignores non-string values",
			metadata: `{"client_cwd":123,"host_env_cwd":false}`,
			want:     "",
		},
		{
			name:     "invalid json",
			metadata: `{`,
			want:     "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := projectLocationFromMetadata([]byte(tc.metadata))
			if got != tc.want {
				t.Fatalf("projectLocationFromMetadata(%q)=%q want %q", tc.metadata, got, tc.want)
			}
		})
	}
}

func TestProjectNameFromLocation(t *testing.T) {
	tests := []struct {
		location string
		want     string
	}{
		{location: "/home/gryph/Projects/ai/omnidex", want: "omnidex"},
		{location: "/tmp/workspace/", want: "workspace"},
		{location: ".", want: "workspace"},
		{location: "", want: "workspace"},
	}

	for _, tc := range tests {
		got := projectNameFromLocation(tc.location)
		if got != tc.want {
			t.Fatalf("projectNameFromLocation(%q)=%q want %q", tc.location, got, tc.want)
		}
	}
}

func TestEnqueueJobPreservesCustomProjectName(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run Postgres project name regression test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("Postgres unavailable: %v", err)
	}

	repo := New(pool)
	if err := repo.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	location := filepath.Join(t.TempDir(), "omni-nxt")
	customName := "Omnidex"
	metadata := fmt.Sprintf(`{"client_cwd":%q}`, location)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM projects WHERE location = $1`, location)
	})

	project, err := repo.CreateProject(ctx, customName, location, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != customName {
		t.Fatalf("CreateProject name=%q want %q", project.Name, customName)
	}

	if _, err := repo.EnqueueJob(ctx, "test instruction", "scrum", []byte(metadata)); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetProject(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != customName {
		t.Fatalf("after EnqueueJob name=%q want %q", got.Name, customName)
	}
}
