package com.omnidex.integration;

import java.net.URI;
import java.time.Duration;
import java.util.List;
import java.util.Map;

public interface Transport {
    Response send(String method, URI uri, Map<String, String> headers, String body, Duration timeout);

    record Response(int status, Map<String, List<String>> headers, byte[] body) {
        public Response {
            if (status < 100 || status > 599) throw new IllegalArgumentException("HTTP status is invalid.");
            headers = Map.copyOf(headers);
            body = body.clone();
        }

        public String header(String name) {
            for (Map.Entry<String, List<String>> entry : headers.entrySet()) {
                if (entry.getKey().equalsIgnoreCase(name)) {
                    if (entry.getValue().size() != 1) {
                        throw new IllegalStateException("Omnidex response repeats an HTTP header.");
                    }
                    return entry.getValue().getFirst();
                }
            }
            return "";
        }

        @Override public byte[] body() { return body.clone(); }
    }
}
