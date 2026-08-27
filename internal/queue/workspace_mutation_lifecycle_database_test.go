package queue

import (
	"errors"
	"testing"
)

func TestPostgresWorkspaceMutationBlocksReplanWhileNonterminal(t *testing.T) {
	fixture := newWorkspaceMutationDatabaseFixture(t, "lifecycle")
	identity, err := workspaceMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.prepareWorkspaceMutation(
		fixture.ctx, fixture.authority, fixture.command, identity,
	); err != nil {
		t.Fatal(err)
	}

	_, err = fixture.repository.ReplanJob(fixture.ctx, testReplanCommand(
		t, fixture.command.JobID, "workspace-mutation-prepared",
		"Do not supersede a prepared workspace mutation.",
	))
	if !errors.Is(err, ErrWorkspaceMutationUnresolved) {
		t.Fatalf("replan with prepared workspace mutation error=%v", err)
	}

	var currentGeneration, generationCount int64
	var status string
	if err := fixture.pool.QueryRow(fixture.ctx, `
		SELECT job.current_generation,operation.status,
		       (SELECT COUNT(*) FROM job_generations WHERE job_id=job.id)
		FROM jobs AS job
		JOIN workspace_mutation_operations AS operation ON operation.job_id=job.id
		WHERE job.id=$1 AND operation.id=$2
	`, fixture.command.JobID, identity.ID).Scan(
		&currentGeneration, &status, &generationCount,
	); err != nil {
		t.Fatal(err)
	}
	if currentGeneration != 1 || generationCount != 1 || status != workspaceMutationPrepared {
		t.Fatalf(
			"rejected replan changed generation/journal=%d/%d/%q",
			currentGeneration, generationCount, status,
		)
	}
}
