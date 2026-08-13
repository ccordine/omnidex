export const JOB_STATUSES = ["pending", "running", "waiting_input", "completed", "failed", "canceled"] as const;
export type JobStatus = (typeof JOB_STATUSES)[number];

export interface JobRecord {
  id: number;
  status: JobStatus;
  current_generation: number;
  result?: unknown;
  error?: unknown;
}

export interface JobStepRecord {
  id: number;
  action: string;
  status: JobStatus;
  generation: number;
}

export interface JobProgressRecord {
  latest_context_id: number;
  count: number;
}

export interface JobDetails {
  job: JobRecord;
  steps: JobStepRecord[];
  progress: JobProgressRecord;
}

export function positiveJobID(value: unknown, source: string): number {
  const jobID = typeof value === "number" || typeof value === "string" ? Number(value) : Number.NaN;
  if (!Number.isSafeInteger(jobID) || jobID <= 0) {
    throw new Error(`${source} did not include a valid positive integer id.`);
  }
  return jobID;
}

export function requireJobRecord(value: unknown, source: string): JobRecord {
  if (!value || typeof value !== "object") throw new Error(`${source} did not include a job record.`);
  const raw = value as Record<string, unknown>;
  const id = positiveJobID(raw.id, source);
  if (!isJobStatus(raw.status)) {
    throw new Error(`${source} job #${id} has invalid status ${JSON.stringify(raw.status)}.`);
  }
  const currentGeneration = positiveInteger(raw.current_generation, `${source} current generation`);
  return {
    id, status: raw.status, current_generation: currentGeneration,
    result: raw.result, error: raw.error,
  };
}

export function requireJobDetails(value: Record<string, any>, requestedJobID: number): JobDetails {
  const job = requireJobRecord(value.job, `Job #${requestedJobID} response`);
  if (job.id !== requestedJobID) {
    throw new Error(`Job refresh requested #${requestedJobID} but the server returned #${job.id}.`);
  }
  if (!Array.isArray(value.steps)) throw new Error(`Job #${requestedJobID} response did not include a steps array.`);
  if (!value.progress || typeof value.progress !== "object") {
    throw new Error(`Job #${requestedJobID} response did not include bounded progress authority.`);
  }
  const steps = value.steps.map((candidate: unknown, index: number) => requireJobStep(
    candidate, job, `Job #${requestedJobID} step ${index}`,
  ));
  const progress = value.progress as Record<string, unknown>;
  const latestContextID = nonnegativeInteger(
    progress.latest_context_id, `Job #${requestedJobID} latest progress context`,
  );
  const count = nonnegativeInteger(progress.count, `Job #${requestedJobID} progress count`);
  if (count > 24 || (count === 0) !== (latestContextID === 0)) {
    throw new Error(`Job #${requestedJobID} response contains inconsistent bounded progress authority.`);
  }
  return { job, steps, progress: { latest_context_id: latestContextID, count } };
}

export function requiredMessage(value: unknown, error: string): string {
  if (typeof value !== "string" || !value.trim()) throw new Error(error);
  return value;
}

function isJobStatus(value: unknown): value is JobStatus {
  return typeof value === "string" && JOB_STATUSES.includes(value as JobStatus);
}

function requireJobStep(value: unknown, job: JobRecord, source: string): JobStepRecord {
  if (!value || typeof value !== "object") throw new Error(`${source} is not an object.`);
  const raw = value as Record<string, unknown>;
  const id = positiveInteger(raw.id, `${source} id`);
  const generation = positiveInteger(raw.generation, `${source} generation`);
  if (generation !== job.current_generation) {
    throw new Error(`${source} escaped current generation ${job.current_generation}.`);
  }
  if (typeof raw.action !== "string" || !raw.action.trim() || raw.action !== raw.action.trim()) {
    throw new Error(`${source} has a noncanonical action.`);
  }
  if (!isJobStatus(raw.status)) throw new Error(`${source} has invalid status ${JSON.stringify(raw.status)}.`);
  return { id, action: raw.action, status: raw.status, generation };
}

function positiveInteger(value: unknown, source: string): number {
  const parsed = nonnegativeInteger(value, source);
  if (parsed === 0) throw new Error(`${source} must be positive.`);
  return parsed;
}

function nonnegativeInteger(value: unknown, source: string): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 0) {
    throw new Error(`${source} must be a nonnegative safe integer.`);
  }
  return value;
}
