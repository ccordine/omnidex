package com.omnidex.integration;

import java.net.URI;
import java.time.Duration;
import java.util.LinkedHashMap;
import java.util.Map;

public final class OmnidexClient {
    private static final int MAX_RESPONSE_BYTES = 16 * 1024 * 1024;

    private final String baseUrl;
    private final String token;
    private final Transport transport;
    private final Duration timeout;

    public OmnidexClient(String baseUrl, String token) {
        this(baseUrl, token, new JdkTransport(), Duration.ofSeconds(30));
    }

    public OmnidexClient(String baseUrl, String token, Transport transport, Duration timeout) {
        Validation.configuration(baseUrl, token);
        if (transport == null) throw new IllegalArgumentException("Omnidex transport is required.");
        if (timeout == null || timeout.isZero() || timeout.isNegative()) {
            throw new IllegalArgumentException("HTTP timeout must be positive.");
        }
        this.baseUrl = baseUrl;
        this.token = token;
        this.transport = transport;
        this.timeout = timeout;
    }

    public DataSource registerDirectDataSource(DirectDataSourceInput input) {
        Validation.directDataSource(input);
        LinkedHashMap<String, Object> body = dataSourceBody(input.name(), "direct");
        body.put("host", empty(input.host()));
        body.put("port", input.port());
        body.put("database_name", empty(input.databaseName()));
        body.put("username", empty(input.username()));
        body.put("password", empty(input.password()));
        body.put("ssl_mode", input.sslMode());
        body.put("use_dsn", input.useDsn());
        body.put("dsn", empty(input.dsn()));
        body.put("authority_url", "");
        body.put("credential_env", "");
        return registerDataSource(body);
    }

    public DataSource registerDelegatedDataSource(DelegatedDataSourceInput input) {
        Validation.delegatedDataSource(input);
        LinkedHashMap<String, Object> body = dataSourceBody(input.name(), "delegated");
        body.put("host", "");
        body.put("port", 0);
        body.put("database_name", "");
        body.put("username", "");
        body.put("password", "");
        body.put("ssl_mode", "");
        body.put("use_dsn", false);
        body.put("dsn", "");
        body.put("authority_url", input.authorityUrl());
        body.put("credential_env", input.credentialEnv());
        return registerDataSource(body);
    }

    public Channel createChannel(CreateChannelInput input) {
        Validation.channel(input);
        Map<String, Object> body = Map.of(
            "id", input.id(), "name", input.name(), "tags", input.tags(),
            "workspace_root", input.workspaceRoot(), "data_source_id", input.dataSourceId(),
            "mode", "assistant"
        );
        return ResponseDecoder.channelEnvelope(
            request("POST", "/v1/integrations/channels", body, 201), input.id(), input.dataSourceId()
        );
    }

    public Channel getChannel(String channelId) {
        Validation.canonicalId("Channel ID", channelId, 96);
        return ResponseDecoder.channelEnvelope(
            request("GET", "/v1/integrations/channels/" + channelId, null, 200), channelId, null
        );
    }

    public SendMessageResult sendMessage(String channelId, SendMessageInput input) {
        Validation.canonicalId("Channel ID", channelId, 96);
        if (input == null) throw new IllegalArgumentException("Message input is required.");
        Validation.prompt(input.prompt());
        LinkedHashMap<String, Object> body = new LinkedHashMap<>();
        body.put("prompt", input.prompt());
        if (input.delegatedDataAuthorityId() != null) {
            Authority.validateDelegatedId(input.delegatedDataAuthorityId());
            body.put("delegated_data_authority_id", input.delegatedDataAuthorityId());
        }
        return ResponseDecoder.messageEnvelope(
            request("POST", "/v1/integrations/channels/" + channelId + "/messages", body, 202),
            channelId,
            input.prompt()
        );
    }

    public MessagePage listMessages(String channelId) {
        return listMessages(channelId, 24, null);
    }

    public MessagePage listMessages(String channelId, int limit, Long beforeId) {
        Validation.canonicalId("Channel ID", channelId, 96);
        if (limit < 1 || limit > 200 || beforeId != null && beforeId < 1) {
            throw new IllegalArgumentException("Message page bounds are invalid.");
        }
        String path = "/v1/integrations/channels/" + channelId + "/messages?limit=" + limit;
        if (beforeId != null) path += "&before_id=" + beforeId;
        return ResponseDecoder.messagePage(request("GET", path, null, 200), channelId);
    }

    public JobDetails getJob(long jobId) {
        if (jobId < 1) throw new IllegalArgumentException("Job ID must be positive.");
        return ResponseDecoder.jobDetails(request("GET", "/v1/integrations/jobs/" + jobId, null, 200), jobId);
    }

    private DataSource registerDataSource(Map<String, Object> body) {
        return ResponseDecoder.dataSource(request("POST", "/v1/integrations/data-sources", body, 201));
    }

    private Map<String, Object> request(String method, String path, Map<String, Object> body, int expectedStatus) {
        String encoded = body == null ? null : Json.encode(body);
        LinkedHashMap<String, String> headers = new LinkedHashMap<>();
        headers.put("Authorization", "Bearer " + token);
        headers.put("Accept", "application/json");
        if (body != null) headers.put("Content-Type", "application/json");
        Transport.Response response = transport.send(method, URI.create(baseUrl + path), Map.copyOf(headers), encoded, timeout);
        byte[] raw = response.body();
        if (raw.length > MAX_RESPONSE_BYTES) {
            throw new IllegalStateException("Omnidex integration response exceeds 16777216 bytes.");
        }
        if (response.status() != expectedStatus) throw ResponseDecoder.error(response.status(), raw);
        String contentType = response.header("content-type").split(";", 2)[0].trim().toLowerCase();
        if (!contentType.equals("application/json")) {
            throw new IllegalStateException("Omnidex returned a non-JSON response.");
        }
        return Json.parseObject(raw);
    }

    private static LinkedHashMap<String, Object> dataSourceBody(String name, String executionMode) {
        LinkedHashMap<String, Object> body = new LinkedHashMap<>();
        body.put("name", name);
        body.put("driver", "postgres");
        body.put("execution_mode", executionMode);
        return body;
    }

    private static String empty(String value) {
        return value == null ? "" : value;
    }
}
