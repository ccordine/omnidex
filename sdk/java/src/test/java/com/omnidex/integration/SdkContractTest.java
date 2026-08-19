package com.omnidex.integration;

import java.net.URI;
import java.time.Duration;
import java.util.ArrayDeque;
import java.util.List;
import java.util.Map;
import java.util.Queue;

public final class SdkContractTest {
    private static final String TOKEN = "integration-token-0123456789abcdef";

    public static void main(String[] args) {
        delegatedRegistrationCarriesNoCredentials();
        messagePreservesPromptAndAuthority();
        invalidAuthorityFailsBeforeTransport();
        responsesFailClosed();
        configurationAndAuthorityAreBounded();
        System.out.println("Java SDK contract tests passed.");
    }

    private static void delegatedRegistrationCarriesNoCredentials() {
        FakeTransport transport = new FakeTransport();
        transport.add(request -> {
            equal("POST", request.method(), "method");
            equal(URI.create("https://omnidex.internal/v1/integrations/data-sources"), request.uri(), "URI");
            equal("Bearer " + TOKEN, request.headers().get("Authorization"), "authorization");
            equal(Map.ofEntries(
                Map.entry("name", "Clinical"), Map.entry("driver", "postgres"),
                Map.entry("execution_mode", "delegated"), Map.entry("host", ""), Map.entry("port", 0L),
                Map.entry("database_name", ""), Map.entry("username", ""), Map.entry("password", ""),
                Map.entry("ssl_mode", ""), Map.entry("use_dsn", false), Map.entry("dsn", ""),
                Map.entry("authority_url", "https://application.internal"),
                Map.entry("credential_env", "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN")
            ), Json.parseObject(request.body()), "delegated body");
            return jsonResponse(201, Map.of("source", Map.of(
                "id", "source-1", "name", "Clinical", "driver", "postgres",
                "execution_mode", "delegated", "authority_url", "https://application.internal",
                "credential_env", "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN", "read_only", true
            )));
        });
        OmnidexClient client = client(transport);
        DataSource source = client.registerDelegatedDataSource(new DelegatedDataSourceInput(
            "Clinical", "https://application.internal", "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN"
        ));
        equal("source-1", source.id(), "source ID");
        equal(0, transport.callsRemaining(), "remaining transport calls");
    }

    private static void messagePreservesPromptAndAuthority() {
        String authority = "dba_" + "a".repeat(64);
        String prompt = "  Find the knee collection.\nKeep context. ";
        FakeTransport transport = new FakeTransport();
        transport.add(request -> {
            equal("https://omnidex.internal/v1/integrations/channels/clinical-chat/messages",
                request.uri().toString(), "message URI");
            equal(Map.of("prompt", prompt, "delegated_data_authority_id", authority),
                Json.parseObject(request.body()), "message body");
            return jsonResponse(202, Map.of(
                "channel", Map.of("id", "clinical-chat", "scope", "user", "data_source_id", "source-1", "mode", "assistant"),
                "user_message", Map.of("id", 12, "channel_id", "clinical-chat", "role", "user",
                    "content", prompt, "created_at", "2026-08-19T00:00:00Z"),
                "job", Map.of("id", 73, "instruction", prompt, "pipeline", "chat")
            ));
        });
        SendMessageResult result = client(transport).sendMessage(
            "clinical-chat", new SendMessageInput(prompt, authority)
        );
        equal(73L, result.job().id(), "job ID");
        equal(prompt, result.userMessage().content(), "prompt");
    }

    private static void invalidAuthorityFailsBeforeTransport() {
        FakeTransport transport = new FakeTransport();
        expect(IllegalArgumentException.class, "opaque dba_", () -> client(transport).sendMessage(
            "clinical-chat", new SendMessageInput("question", "invalid")
        ));
        equal(0, transport.callCount(), "transport calls");
    }

    private static void responsesFailClosed() {
        FakeTransport transport = new FakeTransport();
        transport.add(request -> jsonResponse(200, Map.of(
            "channel_id", "clinical-chat", "messages", List.of(), "next_before_id", 5,
            "has_more", false, "unknown", true
        )));
        transport.add(request -> jsonResponse(409, Map.of("error", "channel already has an active turn")));
        OmnidexClient client = client(transport);
        expect(IllegalStateException.class, "unknown", () -> client.listMessages("clinical-chat", 24, null));
        Throwable error = expect(OmnidexApiException.class, "active turn", () ->
            client.sendMessage("clinical-chat", new SendMessageInput("question", null))
        );
        equal(409, ((OmnidexApiException) error).status(), "API status");
    }

    private static void configurationAndAuthorityAreBounded() {
        expect(IllegalArgumentException.class, "HTTP", () -> new OmnidexClient("file:///tmp/omnidex", TOKEN));
        expect(IllegalArgumentException.class, "trailing slash", () -> new OmnidexClient("https://omnidex.internal/", TOKEN));
        expect(IllegalArgumentException.class, "32", () -> new OmnidexClient("https://omnidex.internal", "short"));
        Authority.validateDelegatedId(Authority.createDelegatedId());
        FakeTransport transport = new FakeTransport();
        expect(IllegalArgumentException.class, "dedicated namespace", () -> client(transport)
            .registerDelegatedDataSource(new DelegatedDataSourceInput(
                "Clinical", "https://application.internal", "OPENAI_API_KEY"
            )));
        equal(0, transport.callCount(), "credential validation transport calls");
    }

    private static OmnidexClient client(FakeTransport transport) {
        return new OmnidexClient("https://omnidex.internal", TOKEN, transport, Duration.ofSeconds(2));
    }

    private static Transport.Response jsonResponse(int status, Map<String, Object> body) {
        return new Transport.Response(status, Map.of("content-type", List.of("application/json")), Json.encode(body).getBytes(java.nio.charset.StandardCharsets.UTF_8));
    }

    private static void equal(Object expected, Object actual, String label) {
        if (!expected.equals(actual)) {
            throw new AssertionError(label + ": expected=" + expected + " actual=" + actual);
        }
    }

    private static Throwable expect(Class<? extends Throwable> type, String message, Runnable callback) {
        try {
            callback.run();
        } catch (Throwable error) {
            if (!type.isInstance(error) || !error.getMessage().contains(message)) {
                throw new AssertionError("Unexpected exception: " + error, error);
            }
            return error;
        }
        throw new AssertionError("Expected " + type.getSimpleName());
    }

    private record Request(String method, URI uri, Map<String, String> headers, String body, Duration timeout) {}
    private interface Handler { Transport.Response handle(Request request); }

    private static final class FakeTransport implements Transport {
        private final Queue<Handler> handlers = new ArrayDeque<>();
        private int calls;

        void add(Handler handler) { handlers.add(handler); }
        int callCount() { return calls; }
        int callsRemaining() { return handlers.size(); }

        @Override
        public Response send(String method, URI uri, Map<String, String> headers, String body, Duration timeout) {
            calls++;
            Handler handler = handlers.poll();
            if (handler == null) throw new AssertionError("Unexpected transport call");
            return handler.handle(new Request(method, uri, headers, body, timeout));
        }
    }
}
