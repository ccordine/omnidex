package com.omnidex.integration;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.Map;

public final class JdkTransport implements Transport {
    private static final int MAX_RESPONSE_BYTES = 16 * 1024 * 1024;
    private final HttpClient client;

    public JdkTransport() {
        this(HttpClient.newBuilder().followRedirects(HttpClient.Redirect.NEVER).build());
    }

    public JdkTransport(HttpClient client) {
        if (client == null || client.followRedirects() != HttpClient.Redirect.NEVER) {
            throw new IllegalArgumentException("Omnidex HTTP client must reject redirects.");
        }
        this.client = client;
    }

    @Override
    public Response send(String method, URI uri, Map<String, String> headers, String body, Duration timeout) {
        HttpRequest.Builder builder = HttpRequest.newBuilder(uri).timeout(timeout);
        headers.forEach(builder::header);
        builder.method(method, body == null ? HttpRequest.BodyPublishers.noBody() : HttpRequest.BodyPublishers.ofString(body));
        try {
            HttpResponse<InputStream> response = client.send(builder.build(), HttpResponse.BodyHandlers.ofInputStream());
            try (InputStream stream = response.body()) {
                return new Response(response.statusCode(), response.headers().map(), readBounded(stream));
            }
        } catch (InterruptedException error) {
            Thread.currentThread().interrupt();
            throw new TransportException("Execute Omnidex integration request was interrupted.", error);
        } catch (IOException error) {
            throw new TransportException("Execute Omnidex integration request failed.", error);
        }
    }

    private static byte[] readBounded(InputStream stream) throws IOException {
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        byte[] buffer = new byte[8192];
        int total = 0;
        for (int count; (count = stream.read(buffer)) >= 0;) {
            total += count;
            if (total > MAX_RESPONSE_BYTES) {
                throw new IOException("Omnidex integration response exceeds 16777216 bytes.");
            }
            output.write(buffer, 0, count);
        }
        return output.toByteArray();
    }
}
