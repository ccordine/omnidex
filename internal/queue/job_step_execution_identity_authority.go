package queue

import (
	"context"
	"fmt"
)

func (r *Repository) validateJobStepExecutionIdentityAuthority(ctx context.Context) error {
	if r == nil || r.pool == nil || ctx == nil {
		return fmt.Errorf("job step execution identity validation requires PostgreSQL and context")
	}
	var databaseAuthority bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_trigger AS trigger_row
			JOIN pg_proc AS procedure ON procedure.oid=trigger_row.tgfoid
			WHERE trigger_row.tgrelid='job_steps'::regclass
			  AND trigger_row.tgname='job_steps_generation_identity_immutable'
			  AND trigger_row.tgenabled='O'
			  AND trigger_row.tgtype=19
			  AND trigger_row.tgattr::text=''
			  AND trigger_row.tgqual IS NULL
			  AND trigger_row.tgconstraint=0
			  AND trigger_row.tgconstrrelid=0
			  AND trigger_row.tgconstrindid=0
			  AND NOT trigger_row.tgdeferrable
			  AND NOT trigger_row.tginitdeferred
			  AND trigger_row.tgnargs=0
			  AND octet_length(trigger_row.tgargs)=0
			  AND trigger_row.tgoldtable IS NULL
			  AND trigger_row.tgnewtable IS NULL
			  AND NOT trigger_row.tgisinternal
			  AND procedure.proname='prevent_job_step_generation_identity_mutation'
			  AND procedure.prokind='f'
			  AND procedure.pronargs=0
			  AND procedure.pronargdefaults=0
			  AND procedure.prorettype='trigger'::regtype
			  AND NOT procedure.proretset
			  AND procedure.prolang=(SELECT oid FROM pg_language WHERE lanname='plpgsql')
			  AND procedure.provolatile='v'
			  AND procedure.proparallel='u'
			  AND NOT procedure.proisstrict
			  AND NOT procedure.prosecdef
			  AND NOT procedure.proleakproof
			  AND procedure.proconfig IS NULL
			  AND procedure.prosrc=$1
		)
	`, jobStepExecutionIdentityTriggerSource).Scan(&databaseAuthority); err != nil {
		return fmt.Errorf("inspect job step execution identity database authority: %w", err)
	}
	if !databaseAuthority {
		return fmt.Errorf("job step execution identity database authority differs from the registered contract")
	}
	return nil
}

const jobStepExecutionIdentityTriggerSource = `
BEGIN
    IF OLD.job_id IS DISTINCT FROM NEW.job_id OR
       OLD.generation IS DISTINCT FROM NEW.generation THEN
        RAISE EXCEPTION 'job step generation identity is immutable';
    END IF;
    IF OLD.action IS DISTINCT FROM NEW.action THEN
        RAISE EXCEPTION 'job step action identity is immutable';
    END IF;
    IF OLD.status <> 'pending' AND NEW.status = 'pending' THEN
        RAISE EXCEPTION 'job step execution identity cannot return to pending';
    END IF;
    IF OLD.superseded_at_generation IS NOT NULL AND NEW IS DISTINCT FROM OLD THEN
        RAISE EXCEPTION 'superseded job step history is immutable';
    END IF;
    RETURN NEW;
END;
`
