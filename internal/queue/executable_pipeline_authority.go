package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) validateExecutablePipelineState(ctx context.Context) error {
	if r == nil || r.pool == nil || ctx == nil {
		return fmt.Errorf("executable pipeline validation requires PostgreSQL and context")
	}
	var databaseAuthority bool
	if err := r.pool.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1
				FROM pg_constraint AS constraint_row
				WHERE constraint_row.conrelid='jobs'::regclass
				  AND constraint_row.conname='jobs_executable_pipeline_authority'
				  AND constraint_row.contype='c'
				  AND constraint_row.convalidated
				  AND NOT constraint_row.connoinherit
				  AND pg_get_constraintdef(constraint_row.oid,true)=
				      'CHECK ((pipeline = ANY (ARRAY[''chat''::text, ''coding''::text, ''scrum''::text])) OR (status = ANY (ARRAY[''completed''::text, ''failed''::text, ''canceled''::text])))'
			) AND EXISTS (
				SELECT 1
				FROM pg_trigger AS trigger_row
				JOIN pg_proc AS procedure ON procedure.oid=trigger_row.tgfoid
				WHERE trigger_row.tgrelid='jobs'::regclass
				  AND trigger_row.tgname='jobs_executable_pipeline_authority'
				  AND trigger_row.tgenabled='O'
				  AND trigger_row.tgtype=31
				  AND trigger_row.tgattr::text=''
				  AND trigger_row.tgqual IS NULL
				  AND NOT trigger_row.tgdeferrable
				  AND NOT trigger_row.tginitdeferred
				  AND NOT trigger_row.tgisinternal
				  AND procedure.proname='enforce_jobs_executable_pipeline_authority'
				  AND procedure.prokind='f'
				  AND procedure.pronargs=0
				  AND procedure.prorettype='trigger'::regtype
				  AND procedure.prolang=(SELECT oid FROM pg_language WHERE lanname='plpgsql')
				  AND procedure.provolatile='v'
				  AND procedure.proparallel='u'
				  AND NOT procedure.prosecdef
				  AND NOT procedure.proleakproof
				  AND procedure.proconfig IS NULL
				  AND procedure.prosrc=$1
			) AND EXISTS (
				SELECT 1
				FROM pg_trigger AS trigger_row
				JOIN pg_proc AS procedure ON procedure.oid=trigger_row.tgfoid
				WHERE trigger_row.tgrelid='jobs'::regclass
				  AND trigger_row.tgname='jobs_history_truncate_immutable'
				  AND trigger_row.tgenabled='O'
				  AND trigger_row.tgtype=34
				  AND trigger_row.tgattr::text=''
				  AND trigger_row.tgqual IS NULL
				  AND NOT trigger_row.tgdeferrable
				  AND NOT trigger_row.tginitdeferred
				  AND NOT trigger_row.tgisinternal
				  AND procedure.proname='enforce_jobs_executable_pipeline_authority'
				  AND procedure.prosrc=$1
			)
	`, executablePipelineTriggerSource).Scan(&databaseAuthority); err != nil {
		return fmt.Errorf("inspect executable pipeline database authority: %w", err)
	}
	if !databaseAuthority {
		return fmt.Errorf("executable pipeline database authority differs from the registered contract")
	}
	var jobID int64
	var pipeline, status string
	err := r.pool.QueryRow(ctx, `
		SELECT id,pipeline,status
		FROM jobs
		WHERE pipeline NOT IN ($1,$2,$3)
		  AND status NOT IN ($4,$5,$6)
		ORDER BY id
		LIMIT 1
	`, model.PipelineChat, model.PipelineCoding, model.PipelineScrum,
		model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled,
	).Scan(&jobID, &pipeline, &status)
	if err == nil {
		return fmt.Errorf(
			"nonterminal job %d has retired or unregistered executable pipeline %q and status %q",
			jobID,
			pipeline,
			status,
		)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("validate executable pipeline state: %w", err)
	}
	return nil
}

const executablePipelineTriggerSource = `
BEGIN
    IF TG_OP='TRUNCATE' THEN
        RAISE EXCEPTION 'job history is immutable';
    END IF;
    IF TG_OP='DELETE' THEN
        IF OLD.pipeline NOT IN ('chat','coding','scrum') THEN
            RAISE EXCEPTION 'historical retired job is immutable';
        END IF;
        RETURN OLD;
    END IF;
    IF TG_OP='INSERT' AND NEW.pipeline NOT IN ('chat','coding','scrum') THEN
        RAISE EXCEPTION 'new job pipeline % is retired or unregistered', NEW.pipeline;
    END IF;
    IF TG_OP='UPDATE'
       AND OLD.pipeline NOT IN ('chat','coding','scrum')
       AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'historical retired job is immutable';
    END IF;
    IF TG_OP='UPDATE'
       AND OLD.pipeline IN ('chat','coding','scrum')
       AND NEW.pipeline NOT IN ('chat','coding','scrum') THEN
        RAISE EXCEPTION 'current job pipeline cannot become retired or unregistered';
    END IF;
    IF TG_OP='UPDATE'
       AND OLD.status IN ('completed','failed','canceled')
       AND NEW.status NOT IN ('completed','failed','canceled') THEN
        RAISE EXCEPTION 'terminal job cannot become nonterminal';
    END IF;
    IF NEW.pipeline NOT IN ('chat','coding','scrum')
       AND NEW.status NOT IN ('completed','failed','canceled') THEN
        RAISE EXCEPTION 'nonterminal job pipeline % is retired or unregistered', NEW.pipeline;
    END IF;
    RETURN NEW;
END;
`
