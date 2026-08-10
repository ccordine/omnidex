package queue

import (
	"strings"
	"testing"
)

func TestPostgresRepositoryMutationFileAuthoritySealsAfterPreparation(t *testing.T) {
	fixture := newRepositoryMutationDatabaseFixture(t, "sealed-files")
	identity, err := repositoryMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.prepareRepositoryMutation(
		fixture.ctx, fixture.authority, fixture.command, identity,
	); err != nil {
		t.Fatal(err)
	}

	file := fixture.command.ChangedFiles[0]
	started := make(chan struct{})
	results := make(chan error, 2)
	for ordinal := 1; ordinal <= 2; ordinal++ {
		go func(ordinal int) {
			<-started
			_, insertErr := fixture.pool.Exec(fixture.ctx, `
				INSERT INTO repository_mutation_files (
					operation_id, ordinal, file_id, path, source_sha256, source_size,
					expected_sha256, expected_size
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			`, identity.ID, ordinal, file.FileID, file.Path, file.SourceSHA256,
				file.SourceSize, file.ExpectedSHA256, file.ExpectedSize)
			results <- insertErr
		}(ordinal)
	}
	close(started)
	for attempt := 0; attempt < 2; attempt++ {
		err = <-results
		if err == nil || !strings.Contains(err.Error(), "file authority is sealed") {
			t.Fatalf("concurrent post-commit mutation file insert error=%v", err)
		}
	}

	loaded, err := fixture.repository.UnresolvedRepositoryMutation(
		fixture.ctx, fixture.job.ID, fixture.command.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || len(loaded.ChangedFiles) != 1 ||
		loaded.ChangedFiles[0] != fixture.command.ChangedFiles[0] {
		t.Fatalf("sealed mutation replay authority=%+v", loaded)
	}
}
