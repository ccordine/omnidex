package com.omnidex.integration;

import java.util.ArrayList;
import java.util.Collections;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;

final class ResponseDecoder {
    private static final Set<String> DATA_SOURCE_KEYS = Set.of(
        "id", "name", "driver", "execution_mode", "host", "port", "database_name", "username",
        "ssl_mode", "use_dsn", "authority_url", "credential_env", "read_only", "password_set",
        "password_hint", "last_test_status", "last_test_message", "last_test_at", "catalog_updated_at",
        "created_at", "updated_at"
    );
    private static final Set<String> CHANNEL_KEYS = Set.of(
        "id", "scope", "name", "tags", "project_id", "workspace_root", "data_source_id", "mode",
        "roleplay_viewpoint_character_id", "created_at", "updated_at"
    );
    private static final Set<String> JOB_KEYS = Set.of(
        "id", "instruction", "pipeline", "status", "result", "error", "metadata", "current_generation",
        "created_at", "updated_at", "completed_at"
    );
    private static final Set<String> STEP_KEYS = Set.of(
        "id", "job_id", "action", "sort_index", "status", "generation", "superseded_at_generation",
        "worker_id", "output", "error", "started_at", "finished_at", "created_at", "updated_at"
    );

    private ResponseDecoder() {}

    static DataSource dataSource(Map<String, Object> envelope) {
        exact(envelope, Set.of("source"), Set.of("source"), "data-source response");
        Map<String, Object> source = object(envelope.get("source"), "data source");
        exact(source, DATA_SOURCE_KEYS, Set.of("id", "name", "driver", "execution_mode", "read_only"), "data source");
        String driver = string(source, "driver", true);
        String executionMode = string(source, "execution_mode", true);
        if (!driver.equals("postgres") || !Set.of("direct", "delegated").contains(executionMode) ||
            !Boolean.TRUE.equals(source.get("read_only"))) {
            throw new IllegalStateException("Omnidex returned an invalid data-source authority.");
        }
        return new DataSource(
            string(source, "id", true), string(source, "name", true), driver, executionMode, true, immutable(source)
        );
    }

    static Channel channelEnvelope(Map<String, Object> envelope, String expectedId, String expectedSourceId) {
        exact(envelope, Set.of("channel"), Set.of("channel"), "channel response");
        Channel channel = channel(object(envelope.get("channel"), "channel"));
        if (!channel.id().equals(expectedId) || expectedSourceId != null && !channel.dataSourceId().equals(expectedSourceId) ||
            !channel.mode().equals("assistant")) {
            throw new IllegalStateException("Omnidex returned a channel outside the requested authority.");
        }
        return channel;
    }

    static SendMessageResult messageEnvelope(Map<String, Object> envelope, String channelId, String prompt) {
        exact(envelope, Set.of("channel", "user_message", "job"), Set.of("channel", "user_message", "job"), "message response");
        Channel channel = channel(object(envelope.get("channel"), "channel"));
        ChannelMessage message = message(object(envelope.get("user_message"), "user message"));
        Job job = job(object(envelope.get("job"), "job"));
        if (!channel.id().equals(channelId) || !message.channelId().equals(channelId) ||
            !message.content().equals(prompt) || job.id() < 1) {
            throw new IllegalStateException("Omnidex returned a message outside the requested authority.");
        }
        return new SendMessageResult(channel, message, job);
    }

    static MessagePage messagePage(Map<String, Object> value, String channelId) {
        exact(value, Set.of("channel_id", "messages", "next_before_id", "has_more"),
            Set.of("channel_id", "messages", "has_more"), "message page");
        if (!channelId.equals(value.get("channel_id")) || !(value.get("messages") instanceof List<?> values) ||
            !(value.get("has_more") instanceof Boolean hasMore)) {
            throw new IllegalStateException("Omnidex returned invalid message-page authority.");
        }
        Long next = nullablePositiveInteger(value.get("next_before_id"), "next_before_id");
        if (hasMore != (next != null)) {
            throw new IllegalStateException("Omnidex returned contradictory message pagination.");
        }
        ArrayList<ChannelMessage> messages = new ArrayList<>();
        for (Object item : values) messages.add(message(object(item, "channel message")));
        return new MessagePage(channelId, List.copyOf(messages), next, hasMore);
    }

    static JobDetails jobDetails(Map<String, Object> value, long jobId) {
        exact(value, Set.of("job", "steps", "contexts"), Set.of("job", "steps", "contexts"), "job details");
        Job job = job(object(value.get("job"), "job"));
        if (job.id() != jobId || !(value.get("steps") instanceof List<?> steps) ||
            !(value.get("contexts") instanceof List<?> contexts)) {
            throw new IllegalStateException("Omnidex returned a different job authority.");
        }
        List<Map<String, Object>> decodedSteps = objects(
            steps, STEP_KEYS, Set.of("id", "job_id", "action", "status"), "job step"
        );
        List<Map<String, Object>> decodedContexts = objects(
            contexts, Set.of("id", "step_id", "key", "value", "created_at"),
            Set.of("id", "step_id", "key", "value", "created_at"), "job context"
        );
        return new JobDetails(job, decodedSteps, decodedContexts);
    }

    static OmnidexApiException error(int status, byte[] raw) {
        String message = "invalid error envelope";
        try {
            Map<String, Object> value = Json.parseObject(raw);
            exact(value, Set.of("error"), Set.of("error"), "error envelope");
            message = string(value, "error", true);
        } catch (RuntimeException ignored) {
            // The fixed text deliberately avoids reflecting an unvalidated response.
        }
        return new OmnidexApiException(status, message);
    }

    private static Channel channel(Map<String, Object> value) {
        exact(value, CHANNEL_KEYS, Set.of("id", "scope", "mode"), "channel");
        List<String> tags = strings(value.get("tags"), "channel tags");
        return new Channel(
            string(value, "id", true), string(value, "scope", true), optionalString(value, "name"), tags,
            optionalInteger(value, "project_id"), optionalString(value, "workspace_root"),
            optionalString(value, "data_source_id"), string(value, "mode", true)
        );
    }

    private static ChannelMessage message(Map<String, Object> value) {
        exact(value, Set.of("id", "channel_id", "role", "content", "created_at"),
            Set.of("id", "channel_id", "role", "content", "created_at"), "channel message");
        String role = string(value, "role", true);
        if (!Set.of("user", "assistant").contains(role)) {
            throw new IllegalStateException("Omnidex returned an invalid channel message.");
        }
        return new ChannelMessage(
            positiveInteger(value.get("id"), "message ID"), string(value, "channel_id", true), role,
            string(value, "content", false), string(value, "created_at", true)
        );
    }

    private static Job job(Map<String, Object> value) {
        exact(value, JOB_KEYS, Set.of("id", "instruction", "pipeline"), "job");
        return new Job(
            positiveInteger(value.get("id"), "job ID"), string(value, "instruction", false),
            string(value, "pipeline", true), optionalString(value, "status"), optionalString(value, "result"),
            optionalString(value, "error"), optionalInteger(value, "current_generation"), immutable(value)
        );
    }

    private static List<Map<String, Object>> objects(
        List<?> values, Set<String> allowed, Set<String> required, String label
    ) {
        ArrayList<Map<String, Object>> result = new ArrayList<>();
        for (Object item : values) {
            Map<String, Object> object = object(item, label);
            exact(object, allowed, required, label);
            result.add(immutable(object));
        }
        return List.copyOf(result);
    }

    private static List<String> strings(Object value, String label) {
        if (value == null) return List.of();
        if (!(value instanceof List<?> values)) throw new IllegalStateException(label + " must be an array.");
        ArrayList<String> result = new ArrayList<>();
        for (Object item : values) {
            if (!(item instanceof String text)) throw new IllegalStateException(label + " must contain strings.");
            result.add(text);
        }
        return List.copyOf(result);
    }

    private static void exact(Map<String, Object> value, Set<String> allowed, Set<String> required, String label) {
        for (String key : value.keySet()) {
            if (!allowed.contains(key)) throw new IllegalStateException(label + " contains unknown field \"" + key + "\".");
        }
        for (String key : required) {
            if (!value.containsKey(key)) throw new IllegalStateException(label + " is missing field \"" + key + "\".");
        }
    }

    private static Map<String, Object> object(Object value, String label) {
        if (!(value instanceof Map<?, ?> map)) throw new IllegalStateException(label + " must be an object.");
        @SuppressWarnings("unchecked") Map<String, Object> result = (Map<String, Object>) map;
        return result;
    }

    private static String string(Map<String, Object> value, String key, boolean nonblank) {
        Object resolved = value.get(key);
        if (!(resolved instanceof String text) || nonblank && text.trim().isEmpty()) {
            throw new IllegalStateException(key + " must be a string.");
        }
        return text;
    }

    private static String optionalString(Map<String, Object> value, String key) {
        Object resolved = value.get(key);
        if (resolved == null) return "";
        if (!(resolved instanceof String text)) throw new IllegalStateException(key + " must be a string.");
        return text;
    }

    private static long optionalInteger(Map<String, Object> value, String key) {
        Object resolved = value.get(key);
        if (resolved == null) return 0;
        if (!(resolved instanceof Long number) || number < 0) throw new IllegalStateException(key + " must be an integer.");
        return number;
    }

    private static long positiveInteger(Object value, String label) {
        if (!(value instanceof Long number) || number < 1) throw new IllegalStateException(label + " must be positive.");
        return number;
    }

    private static Long nullablePositiveInteger(Object value, String label) {
        if (value == null) return null;
        return positiveInteger(value, label);
    }

    private static Map<String, Object> immutable(Map<String, Object> value) {
        return Collections.unmodifiableMap(new LinkedHashMap<>(value));
    }
}
