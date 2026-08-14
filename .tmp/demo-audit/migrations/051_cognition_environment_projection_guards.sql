CREATE OR REPLACE FUNCTION cognition_environment_projection_exact(TEXT,BIGINT)
RETURNS BOOLEAN AS $$
    SELECT EXISTS (
        SELECT 1
        FROM cognition_environment_journals journals
        JOIN cognition_environment_receipts receipts
          ON receipts.episode_id=journals.episode_id
         AND receipts.commit_sequence=journals.commit_sequence
        JOIN cognition_actions actions ON actions.action_id=receipts.action_id
        JOIN cognition_episodes episodes ON episodes.episode_id=receipts.episode_id
        JOIN jobs ON jobs.id=receipts.actor_job_id
        JOIN job_steps steps
          ON steps.job_id=receipts.actor_job_id
         AND steps.generation=receipts.actor_generation
         AND steps.id=receipts.actor_step_id
        JOIN job_step_attempts attempts
          ON attempts.job_id=receipts.actor_job_id
         AND attempts.generation=receipts.actor_generation
         AND attempts.step_id=receipts.actor_step_id
         AND attempts.attempt=receipts.actor_attempt
         AND attempts.worker_id=receipts.actor_worker_id
        WHERE journals.episode_id=$1 AND journals.commit_sequence=$2
          AND journals.last_receipt_json=receipts.receipt_json
          AND journals.last_receipt_sha256=receipts.receipt_sha256
          AND actions.episode_id=receipts.episode_id
          AND actions.job_id=receipts.actor_job_id
          AND actions.generation=receipts.actor_generation
          AND actions.step_id=receipts.actor_step_id
          AND actions.expected_revision=receipts.expected_revision
          AND actions.expected_revision_sha256=receipts.expected_revision_sha256
          AND actions.status IN ('dispatched','succeeded')
          AND (actions.registered_action_json::jsonb-'actor')=(receipts.action_json::jsonb-'actor')
          AND episodes.job_id=receipts.actor_job_id
          AND episodes.generation=receipts.actor_generation
          AND episodes.step_id=receipts.actor_step_id
          AND jobs.status='running'
          AND jobs.current_generation=receipts.actor_generation
          AND steps.status='running'
          AND steps.superseded_at_generation IS NULL
          AND steps.current_attempt=receipts.actor_attempt
          AND steps.worker_id=receipts.actor_worker_id
          AND attempts.status='active'
          AND attempts.expires_at>clock_timestamp()
          AND (
            (receipts.status='transition'
             AND journals.current_revision=receipts.expected_revision+1
             AND journals.current_revision_sha256=receipts.receipt_json::jsonb#>>'{transition,current,sha256}'
             AND journals.current_receipt_json=receipts.receipt_json
             AND journals.current_receipt_sha256=receipts.receipt_sha256
             AND journals.terminal=(receipts.receipt_json::jsonb#>>'{transition,terminal}')::BOOLEAN)
            OR
            (receipts.status='failure'
             AND journals.current_revision=receipts.expected_revision
             AND journals.current_revision_sha256=receipts.expected_revision_sha256
             AND NOT journals.terminal)
          )
    );
$$ LANGUAGE SQL VOLATILE;

CREATE OR REPLACE FUNCTION guard_cognition_environment_journal_update()
RETURNS TRIGGER AS $$
BEGIN
    IF ROW(NEW.episode_id,NEW.scenario_id,NEW.scenario_sha256,
           NEW.start_json,NEW.start_sha256,NEW.created_at)
       IS DISTINCT FROM ROW(OLD.episode_id,OLD.scenario_id,OLD.scenario_sha256,
           OLD.start_json,OLD.start_sha256,OLD.created_at) OR OLD.terminal OR
       NEW.commit_sequence<>OLD.commit_sequence+1 OR NEW.last_receipt_json IS NULL OR
       NEW.last_receipt_sha256 IS NULL OR NEW.updated_at<=OLD.updated_at OR NOT (
         (NEW.current_revision=OLD.current_revision+1 AND
          NEW.current_receipt_json=NEW.last_receipt_json AND
          NEW.current_receipt_sha256=NEW.last_receipt_sha256) OR
         (NEW.current_revision=OLD.current_revision AND
          ROW(NEW.current_revision_sha256,NEW.current_receipt_json,NEW.current_receipt_sha256,
              NEW.terminal,NEW.terminal_receipt_json,NEW.terminal_receipt_sha256)
          IS NOT DISTINCT FROM ROW(OLD.current_revision_sha256,OLD.current_receipt_json,
              OLD.current_receipt_sha256,OLD.terminal,OLD.terminal_receipt_json,
              OLD.terminal_receipt_sha256))
       ) THEN
        RAISE EXCEPTION 'cognition environment journal transition is invalid';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_cognition_environment_projection()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT cognition_environment_projection_exact(NEW.episode_id,NEW.commit_sequence) THEN
        RAISE EXCEPTION 'cognition environment receipt and journal projection disagree';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cognition_environment_journals_update_guard BEFORE UPDATE
ON cognition_environment_journals FOR EACH ROW
EXECUTE FUNCTION guard_cognition_environment_journal_update();
CREATE CONSTRAINT TRIGGER cognition_environment_journals_exact AFTER UPDATE
ON cognition_environment_journals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION require_cognition_environment_projection();
CREATE CONSTRAINT TRIGGER cognition_environment_receipts_exact AFTER INSERT
ON cognition_environment_receipts DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION require_cognition_environment_projection();

CREATE OR REPLACE FUNCTION require_exact_cognition_cancellation()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_episodes episodes
        JOIN cognition_terminal_seals seals ON seals.episode_id=episodes.episode_id
        WHERE episodes.episode_id=NEW.episode_id AND episodes.job_id=NEW.job_id
          AND episodes.generation=NEW.generation AND episodes.step_id=NEW.step_id
          AND episodes.status='canceled' AND episodes.current_revision=NEW.expected_revision
          AND episodes.current_revision_sha256=NEW.expected_revision_sha256
          AND episodes.terminal_outcome=NEW.source_evidence_json::jsonb->>'public_message'
          AND seals.outcome='canceled' AND seals.final_revision=NEW.expected_revision
          AND seals.final_revision_sha256=NEW.expected_revision_sha256
          AND seals.sealed_attempt=NEW.actor_attempt AND seals.sealed_worker_id=NEW.actor_worker_id
          AND NEW.source_evidence_json::jsonb->>'id'=NEW.source_evidence_id
          AND NEW.source_evidence_json::jsonb->>'sha256'=NEW.source_evidence_sha256
          AND NEW.source_evidence_json::jsonb->>'code'=NEW.cancellation_code
          AND seals.trace_json::jsonb->'records' @> jsonb_build_array(jsonb_build_object(
              'kind','cancellation_evidence','id',NEW.source_evidence_id,
              'sha256',NEW.source_evidence_json_sha256))
    ) THEN RAISE EXCEPTION 'cognition cancellation lacks exact episode, seal, actor, or trace authority';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_cognition_seal_cancellation()
RETURNS TRIGGER AS $$
BEGIN
    IF (NEW.outcome='canceled') <> EXISTS (
        SELECT 1 FROM cognition_episode_cancellations WHERE episode_id=NEW.episode_id
    ) THEN RAISE EXCEPTION 'cognition canceled seal and typed cancellation disagree'; END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER cognition_episode_cancellations_exact AFTER INSERT
ON cognition_episode_cancellations DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION require_exact_cognition_cancellation();
CREATE CONSTRAINT TRIGGER cognition_terminal_seals_require_cancellation AFTER INSERT
ON cognition_terminal_seals DEFERRABLE INITIALLY DEFERRED FOR EACH ROW
EXECUTE FUNCTION require_cognition_seal_cancellation();

CREATE TRIGGER cognition_environment_journals_delete_immutable BEFORE DELETE
ON cognition_environment_journals FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_environment_journals_no_truncate BEFORE TRUNCATE
ON cognition_environment_journals FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_environment_receipts_update_immutable BEFORE UPDATE OR DELETE
ON cognition_environment_receipts FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_environment_receipts_no_truncate BEFORE TRUNCATE
ON cognition_environment_receipts FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_episode_cancellations_immutable BEFORE UPDATE OR DELETE
ON cognition_episode_cancellations FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_episode_cancellations_no_truncate BEFORE TRUNCATE
ON cognition_episode_cancellations FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();
