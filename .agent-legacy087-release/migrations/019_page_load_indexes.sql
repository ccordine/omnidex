-- Indexes for Jobs, Metrics, Operations Metrics, and Admin page load paths.

CREATE INDEX IF NOT EXISTS idx_jobs_id_desc ON jobs(id DESC);
CREATE INDEX IF NOT EXISTS idx_jobs_status_id_desc ON jobs(status, id DESC);
CREATE INDEX IF NOT EXISTS idx_job_steps_job_sort ON job_steps(job_id, sort_index ASC, id ASC);
CREATE INDEX IF NOT EXISTS idx_job_steps_job_action ON job_steps(job_id, action);

CREATE INDEX IF NOT EXISTS idx_memory_candidates_status ON memory_candidates(status);

CREATE INDEX IF NOT EXISTS idx_omni_runs_started ON omni_runs(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_omni_events_created ON omni_run_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_omni_events_type_created ON omni_run_events(event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_omni_model_role_provider_model ON omni_model_calls(role, provider, model);
CREATE INDEX IF NOT EXISTS idx_omni_playbook_usage_summary ON omni_playbook_usage(playbook_id, reused, success);
CREATE INDEX IF NOT EXISTS idx_omni_benchmark_summary ON omni_benchmark_results(benchmark_id, suite_id, status);
CREATE INDEX IF NOT EXISTS idx_llm_context_usage_created ON omni_llm_context_usage(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_llm_context_usage_run_id ON omni_llm_context_usage(run_id);
CREATE INDEX IF NOT EXISTS idx_llm_context_usage_run_sent ON omni_llm_context_usage(run_id, sent_chars DESC);
