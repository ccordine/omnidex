CREATE TABLE cognition_obligation_graphs (
    episode_id TEXT NOT NULL,
    graph_version BIGINT NOT NULL CHECK (graph_version>0),
    job_id BIGINT NOT NULL,
    generation BIGINT NOT NULL,
    step_id BIGINT NOT NULL,
    command_id TEXT NOT NULL UNIQUE CHECK (
        command_id~'^cognition_graph_command_[0-9a-f]{64}$'
    ),
    command_sha256 TEXT NOT NULL CHECK (command_sha256~'^[0-9a-f]{64}$'),
    command_kind TEXT NOT NULL CHECK (command_kind IN (
        'initial','add','activate','fail','add_evidence','add_dependency','satisfy'
    )),
    graph_json TEXT NOT NULL CHECK (
        jsonb_typeof(graph_json::jsonb)='object' AND octet_length(graph_json)<=2097152
    ),
    graph_sha256 TEXT NOT NULL CHECK (graph_sha256~'^[0-9a-f]{64}$'),
    graph_json_sha256 TEXT NOT NULL CHECK (
        graph_json_sha256~'^[0-9a-f]{64}$' AND
        graph_json_sha256=encode(digest(graph_json,'sha256'),'hex')
    ),
    actor_attempt BIGINT NOT NULL CHECK (actor_attempt>0),
    actor_worker_id TEXT NOT NULL CHECK (task_ledger_text_is_exact(actor_worker_id)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (episode_id,graph_version),
    FOREIGN KEY (episode_id,job_id,generation)
        REFERENCES cognition_episodes(episode_id,job_id,generation) ON DELETE RESTRICT,
    FOREIGN KEY (job_id,generation,step_id,actor_attempt,actor_worker_id)
        REFERENCES job_step_attempts(job_id,generation,step_id,attempt,worker_id) ON DELETE RESTRICT
);

CREATE OR REPLACE FUNCTION validate_cognition_obligation_graph_append()
RETURNS TRIGGER AS $$
DECLARE
    episode_status TEXT;
    previous_version BIGINT;
BEGIN
    SELECT status INTO episode_status
    FROM cognition_episodes
    WHERE episode_id=NEW.episode_id AND job_id=NEW.job_id AND generation=NEW.generation
      AND step_id=NEW.step_id
    FOR UPDATE;
    IF NOT FOUND OR episode_status<>'active' THEN
        RAISE EXCEPTION 'cognition obligation graph requires its exact active episode';
    END IF;
    SELECT MAX(graph_version) INTO previous_version
    FROM cognition_obligation_graphs WHERE episode_id=NEW.episode_id;
    IF previous_version IS NULL THEN
        IF NEW.graph_version<>1 OR NEW.command_kind<>'initial' THEN
            RAISE EXCEPTION 'initial cognition obligation graph must be version one';
        END IF;
    ELSIF NEW.command_kind='initial' OR NEW.graph_version<>previous_version+1 THEN
        RAISE EXCEPTION 'cognition obligation graph versions must append exactly once';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER cognition_obligation_graph_append
BEFORE INSERT ON cognition_obligation_graphs
FOR EACH ROW EXECUTE FUNCTION validate_cognition_obligation_graph_append();

CREATE TRIGGER cognition_obligation_graphs_immutable
BEFORE UPDATE OR DELETE ON cognition_obligation_graphs
FOR EACH ROW EXECUTE FUNCTION prevent_cognition_immutable_mutation();
CREATE TRIGGER cognition_obligation_graphs_no_truncate
BEFORE TRUNCATE ON cognition_obligation_graphs
FOR EACH STATEMENT EXECUTE FUNCTION prevent_cognition_immutable_mutation();

CREATE OR REPLACE FUNCTION require_cognition_episode_graph()
RETURNS TRIGGER AS $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM cognition_obligation_graphs graphs
        WHERE graphs.episode_id=NEW.episode_id AND graphs.graph_version=1
          AND graphs.job_id=NEW.job_id AND graphs.generation=NEW.generation
          AND graphs.step_id=NEW.step_id AND graphs.command_kind='initial'
    ) THEN
        RAISE EXCEPTION 'cognition episode requires an initial obligation graph';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
CREATE CONSTRAINT TRIGGER cognition_episode_requires_graph
AFTER INSERT ON cognition_episodes DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_cognition_episode_graph();
