package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type RepositoryVerificationExecutionCounts struct {
	Baseline      int
	Staged        int
	Authoritative int
}

func (counts RepositoryVerificationExecutionCounts) Total() int {
	return counts.Baseline + counts.Staged + counts.Authoritative
}

// DesiredRepositoryExecutionEvidence is code-owned durable execution truth.
// It reports physical work already selected and performed by Omnidex; it does
// not grant a model authority to select any operation.
type DesiredRepositoryExecutionEvidence struct {
	MutationOperations           int
	FileTransitions              int
	CreatedFiles                 int
	DeletedFiles                 int
	ModifiedFiles                int
	VerificationCommands         RepositoryVerificationExecutionCounts
	PostStateRepositoryReindexes int
	BeforeInventory              int
	AfterInventory               int
	InventoryDelta               int
}

func (proof DesiredRepositoryExecutionEvidence) DeterministicOperations() int {
	return proof.FileTransitions + proof.VerificationCommands.Total() +
		proof.PostStateRepositoryReindexes
}

// DesiredRepositoryExecutionEvidence loads one exact completed desired-state
// execution while the owning step attempt is still active. Missing, duplicate,
// stale, or differently-owned durable evidence is rejected.
func (r *Repository) DesiredRepositoryExecutionEvidence(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	graphID, beforeSnapshotID, afterSnapshotID string,
) (DesiredRepositoryExecutionEvidence, error) {
	var proof DesiredRepositoryExecutionEvidence
	if ctx == nil || r == nil || r.pool == nil {
		return proof, fmt.Errorf("desired repository execution evidence requires PostgreSQL and context")
	}
	if err := validateStepAttemptAuthority(authority); err != nil {
		return proof, err
	}
	if !repositoryMutationOpaqueID(graphID, "desired_graph_") ||
		!repositoryMutationOpaqueID(beforeSnapshotID, "snapshot_") ||
		!repositoryMutationOpaqueID(afterSnapshotID, "snapshot_") ||
		beforeSnapshotID == afterSnapshotID {
		return proof, fmt.Errorf("desired repository execution evidence identities are invalid")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return proof, fmt.Errorf("begin desired repository execution evidence read: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := r.AuthorizeStepAttemptTransaction(ctx, tx, authority); err != nil {
		return proof, err
	}

	var beforeRepository, afterRepository, beforeRoot, afterRoot, afterGitSHA string
	var postMismatches int
	err = tx.QueryRow(ctx, `
		SELECT before.repository_id,after.repository_id,before.root,after.root,
		       after.git_state_sha256,
		       (SELECT COUNT(*) FROM repository_files WHERE snapshot_id=before.id),
		       (SELECT COUNT(*) FROM repository_files WHERE snapshot_id=after.id)
		FROM jobs
		JOIN repository_snapshots AS before
		  ON before.project_id=jobs.project_id AND before.id=$2
		JOIN repository_snapshots AS after
		  ON after.project_id=jobs.project_id AND after.id=$3
		WHERE jobs.id=$1
	`, authority.JobID, beforeSnapshotID, afterSnapshotID).Scan(
		&beforeRepository, &afterRepository, &beforeRoot, &afterRoot, &afterGitSHA,
		&proof.BeforeInventory, &proof.AfterInventory,
	)
	if err != nil {
		return proof, fmt.Errorf("load desired repository execution snapshots: %w", err)
	}
	if beforeRepository != afterRepository || beforeRoot != afterRoot {
		return proof, fmt.Errorf("desired repository execution snapshots have different repository authority")
	}

	var operationID, stageID, patchSHA, sourceSnapshotID string
	var mutationEvidenceID int64
	rows, err := tx.Query(ctx, `
		SELECT id,stage_id,patch_sha256,source_snapshot_id,evidence_id
		FROM repository_mutation_operations
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND step_attempt=$4
		  AND worker_id=$5 AND contract_id=$6 AND status='applied'
		ORDER BY id
	`, authority.JobID, authority.Generation, authority.StepID, authority.Attempt,
		authority.WorkerID, graphID)
	if err != nil {
		return proof, fmt.Errorf("load desired repository mutation operation: %w", err)
	}
	for rows.Next() {
		proof.MutationOperations++
		if proof.MutationOperations == 1 {
			err = rows.Scan(&operationID, &stageID, &patchSHA, &sourceSnapshotID, &mutationEvidenceID)
		} else {
			var discardID, discardStage, discardPatch, discardSource string
			var discardEvidence int64
			err = rows.Scan(&discardID, &discardStage, &discardPatch, &discardSource, &discardEvidence)
		}
		if err != nil {
			rows.Close()
			return proof, fmt.Errorf("scan desired repository mutation operation: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return proof, fmt.Errorf("iterate desired repository mutation operations: %w", err)
	}
	rows.Close()
	if proof.MutationOperations != 1 || sourceSnapshotID != beforeSnapshotID || mutationEvidenceID < 1 {
		return proof, fmt.Errorf(
			"desired repository execution requires one applied mutation owned by its exact attempt and source; found %d",
			proof.MutationOperations,
		)
	}

	err = tx.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE NOT source_present AND expected_present),
		       COUNT(*) FILTER (WHERE source_present AND NOT expected_present),
		       COUNT(*) FILTER (WHERE source_present AND expected_present),
		       COUNT(*) FILTER (WHERE expected_present AND NOT EXISTS (
		         SELECT 1 FROM repository_files AS post
		         WHERE post.snapshot_id=$2 AND post.file_id=file.file_id AND post.path=file.path
		           AND post.entry_kind='regular' AND post.content_sha256=file.expected_sha256
		           AND post.size_bytes=file.expected_size AND post.mode_bits=file.expected_mode
		       ) OR NOT expected_present AND EXISTS (
		         SELECT 1 FROM repository_files AS post
		         WHERE post.snapshot_id=$2 AND (post.file_id=file.file_id OR post.path=file.path)
		       ))
		FROM repository_mutation_files AS file WHERE file.operation_id=$1
	`, operationID, afterSnapshotID).Scan(
		&proof.FileTransitions, &proof.CreatedFiles, &proof.DeletedFiles,
		&proof.ModifiedFiles, &postMismatches,
	)
	if err != nil {
		return proof, fmt.Errorf("load desired repository file transitions: %w", err)
	}
	if proof.FileTransitions < 1 || proof.FileTransitions != proof.CreatedFiles+proof.DeletedFiles+proof.ModifiedFiles ||
		postMismatches != 0 {
		return proof, fmt.Errorf("desired repository mutation files disagree with exact post-state snapshot")
	}
	proof.InventoryDelta = proof.AfterInventory - proof.BeforeInventory
	if proof.InventoryDelta != proof.CreatedFiles-proof.DeletedFiles {
		return proof, fmt.Errorf("desired repository inventory delta disagrees with durable file transitions")
	}
	if err := validateDesiredMutationEvidence(
		ctx, tx, authority, mutationEvidenceID, operationID, stageID, patchSHA,
		graphID, beforeSnapshotID, proof,
	); err != nil {
		return proof, err
	}
	if err := loadDesiredVerificationEvidence(
		ctx, tx, authority, graphID, beforeSnapshotID, afterSnapshotID,
		stageID, patchSHA, &proof,
	); err != nil {
		return proof, err
	}
	if err := loadDesiredPostIndexEvidence(
		ctx, tx, authority, afterSnapshotID, afterGitSHA, proof.AfterInventory, &proof,
	); err != nil {
		return proof, err
	}
	if err := tx.Commit(ctx); err != nil {
		return proof, fmt.Errorf("commit desired repository execution evidence read: %w", err)
	}
	return proof, nil
}

func validateDesiredMutationEvidence(
	ctx context.Context, tx pgx.Tx, authority model.StepAttemptAuthority,
	evidenceID int64, operationID, stageID, patchSHA, graphID, beforeID string,
	proof DesiredRepositoryExecutionEvidence,
) error {
	var kind, sourceType, sourceRef, hash, owner, persistedOperation, sourceID string
	var created, deleted, modified int
	err := tx.QueryRow(ctx, `
		SELECT kind,COALESCE(source_type,''),COALESCE(source_ref,''),
		       COALESCE(payload_json->>'hash',''),
		       COALESCE(payload_json->'metadata'->>'repository_change_contract_id',''),
		       COALESCE(payload_json->'metadata'->>'repository_mutation_operation_id',''),
		       COALESCE(payload_json->'metadata'->>'source_snapshot_id',''),
		       COALESCE((payload_json->'metadata'->>'created_file_count')::int,-1),
		       COALESCE((payload_json->'metadata'->>'deleted_file_count')::int,-1),
		       COALESCE((payload_json->'metadata'->>'modified_file_count')::int,-1)
		FROM evidence WHERE id=$1 AND job_id=$2 AND step_id=$3
	`, evidenceID, authority.JobID, authority.StepID).Scan(
		&kind, &sourceType, &sourceRef, &hash, &owner, &persistedOperation,
		&sourceID, &created, &deleted, &modified,
	)
	if err != nil {
		return fmt.Errorf("load desired repository mutation evidence: %w", err)
	}
	if kind != evidence.KindGeneratedDiff || sourceType != "repository" || sourceRef != stageID ||
		hash != patchSHA || owner != graphID || persistedOperation != operationID || sourceID != beforeID ||
		created != proof.CreatedFiles || deleted != proof.DeletedFiles || modified != proof.ModifiedFiles {
		return fmt.Errorf("desired repository mutation evidence disagrees with its applied operation")
	}
	return nil
}
