export function dataSourceResponse(value) {
  const envelope = exactObject(value, ["source"], "data-source response");
  const source = exactObject(envelope.source, [
    "id", "name", "driver", "execution_mode", "host", "port", "database_name", "username", "ssl_mode",
    "use_dsn", "authority_url", "credential_env", "read_only", "password_set", "password_hint",
    "last_test_status", "last_test_message", "last_test_at", "catalog_updated_at", "created_at", "updated_at",
  ], "data source", true);
  if (!nonblank(source.id) || source.driver !== "postgres" || source.read_only !== true || !["direct", "delegated"].includes(source.execution_mode)) {
    throw new TypeError("Omnidex returned an invalid data-source authority.");
  }
  return source;
}

export function channelResponse(value, expectedId, expectedSourceId) {
  const envelope = exactObject(value, ["channel"], "channel response");
  const channel = validateChannel(envelope.channel);
  if (channel.id !== expectedId || (expectedSourceId !== undefined && channel.data_source_id !== expectedSourceId) ||
    channel.mode !== "assistant") {
    throw new TypeError("Omnidex returned a channel outside the requested authority.");
  }
  return channel;
}

export function messageResponse(value, channelId, prompt) {
  const envelope = exactObject(value, ["channel", "user_message", "job"], "message response");
  const channel = validateChannel(envelope.channel);
  const message = validateMessage(envelope.user_message);
  const job = validateJob(envelope.job);
  if (channel.id !== channelId || message.channel_id !== channelId || message.content !== prompt || job.id < 1) {
    throw new TypeError("Omnidex returned a message outside the requested authority.");
  }
  return { channel, user_message: message, job };
}

export function messagePageResponse(value, channelId) {
  const page = exactObject(value, ["channel_id", "messages", "next_before_id", "has_more"], "message page", true);
  if (page.channel_id !== channelId || !Array.isArray(page.messages) || typeof page.has_more !== "boolean") {
    throw new TypeError("Omnidex returned an invalid message-page authority.");
  }
  if (page.has_more !== (Number.isSafeInteger(page.next_before_id) && page.next_before_id > 0)) {
    throw new TypeError("Omnidex returned contradictory message pagination.");
  }
  return { ...page, messages: page.messages.map(validateMessage) };
}

export function jobDetailsResponse(value, jobId) {
  const details = exactObject(value, ["job", "steps", "contexts"], "job details");
  const job = validateJob(details.job);
  if (job.id !== jobId || !Array.isArray(details.steps) || !Array.isArray(details.contexts)) {
    throw new TypeError("Omnidex returned a different job authority.");
  }
  return {
    job,
    steps: details.steps.map((step) => exactObject(step, [
      "id", "job_id", "action", "sort_index", "status", "generation", "superseded_at_generation",
      "worker_id", "output", "error", "started_at", "finished_at", "created_at", "updated_at",
    ], "job step", true)),
    contexts: details.contexts.map((context) => exactObject(context, ["id", "step_id", "key", "value", "created_at"], "job context")),
  };
}

export function apiError(status, value) {
  let message = "invalid error envelope";
  try {
    const error = exactObject(value, ["error"], "error envelope");
    if (nonblank(error.error)) message = error.error;
  } catch {
    // The fixed message deliberately avoids reflecting an unvalidated response.
  }
  return new OmnidexApiError(status, message);
}

export class OmnidexApiError extends Error {
  constructor(status, message) {
    super(`Omnidex integration API failed with HTTP ${status}: ${message}`);
    this.name = "OmnidexApiError";
    this.status = status;
    this.apiMessage = message;
  }
}

function validateChannel(value) {
  return exactObject(value, [
    "id", "scope", "name", "tags", "project_id", "workspace_root", "data_source_id", "mode",
    "roleplay_viewpoint_character_id", "created_at", "updated_at",
  ], "channel", true);
}

function validateMessage(value) {
  const message = exactObject(value, ["id", "channel_id", "role", "content", "created_at"], "channel message");
  if (!Number.isSafeInteger(message.id) || message.id < 1 || !["user", "assistant"].includes(message.role) || !nonblank(message.channel_id)) {
    throw new TypeError("Omnidex returned an invalid channel message.");
  }
  return message;
}

function validateJob(value) {
  const job = exactObject(value, [
    "id", "instruction", "pipeline", "status", "result", "error", "metadata", "current_generation",
    "created_at", "updated_at", "completed_at",
  ], "job", true);
  if (!Number.isSafeInteger(job.id) || job.id < 1) throw new TypeError("Omnidex returned an invalid job identity.");
  return job;
}

function exactObject(value, allowed, label, optional = false) {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError(`${label} must be an object.`);
  const allowedSet = new Set(allowed);
  for (const key of Object.keys(value)) {
    if (!allowedSet.has(key)) throw new TypeError(`${label} contains unknown field ${JSON.stringify(key)}.`);
  }
  if (!optional) {
    for (const key of allowed) if (!(key in value)) throw new TypeError(`${label} is missing field ${JSON.stringify(key)}.`);
  }
  return value;
}

function nonblank(value) {
  return typeof value === "string" && value.trim().length > 0;
}
