package com.omnidex.integration;

import java.net.URI;
import java.net.URISyntaxException;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.StandardCharsets;
import java.util.HashSet;
import java.util.Set;
import java.util.regex.Pattern;

final class Validation {
    private static final Pattern CANONICAL_ID = Pattern.compile("[a-z0-9][a-z0-9_.:-]*");
    private static final Pattern ENVIRONMENT = Pattern.compile(
        "OMNIDEX_DELEGATED_AUTHORITY_[A-Z][A-Z0-9_]{0,93}_TOKEN");
    private static final Set<String> SSL_MODES = Set.of(
        "disable", "allow", "prefer", "require", "verify-ca", "verify-full"
    );

    private Validation() {}

    static void configuration(String baseUrl, String token) {
        baseUrl(baseUrl, "Omnidex base URL");
        if (token == null || token.length() < 32 || token.length() > 4096) {
            throw new IllegalArgumentException("Omnidex integration token must contain 32..4096 exact visible ASCII bytes.");
        }
        for (int index = 0; index < token.length(); index++) {
            char value = token.charAt(index);
            if (value < 0x21 || value > 0x7e) {
                throw new IllegalArgumentException("Omnidex integration token must contain 32..4096 exact visible ASCII bytes.");
            }
        }
    }

    static void directDataSource(DirectDataSourceInput input) {
        if (input == null) throw new IllegalArgumentException("Direct data-source input is required.");
        exactText(input.name(), "Data-source name");
        if (input.port() < 1 || input.port() > 65535) {
            throw new IllegalArgumentException("PostgreSQL port must be between 1 and 65535.");
        }
        if (!SSL_MODES.contains(input.sslMode())) {
            throw new IllegalArgumentException("PostgreSQL SSL mode is unsupported.");
        }
        if (input.useDsn()) {
            exactText(input.dsn(), "PostgreSQL DSN");
        } else {
            exactText(input.host(), "PostgreSQL host");
            exactText(input.databaseName(), "PostgreSQL database");
            exactText(input.username(), "PostgreSQL username");
        }
    }

    static void delegatedDataSource(DelegatedDataSourceInput input) {
        if (input == null) throw new IllegalArgumentException("Delegated data-source input is required.");
        exactText(input.name(), "Data-source name");
        baseUrl(input.authorityUrl(), "Delegated authority URL");
        if (input.credentialEnv() == null || !ENVIRONMENT.matcher(input.credentialEnv()).matches()) {
            throw new IllegalArgumentException(
                "Credential environment variable is outside the dedicated namespace OMNIDEX_DELEGATED_AUTHORITY_*.");
        }
    }

    static void channel(CreateChannelInput input) {
        if (input == null) throw new IllegalArgumentException("Channel input is required.");
        canonicalId("Channel ID", input.id(), 96);
        canonicalId("Data-source ID", input.dataSourceId(), 128);
        exactText(input.name(), "Channel name");
        exactText(input.workspaceRoot(), "Channel workspace root");
        if (input.tags() == null || input.tags().size() > 32) {
            throw new IllegalArgumentException("Channel tags must contain at most 32 entries.");
        }
        Set<String> seen = new HashSet<>();
        for (String tag : input.tags()) {
            if (tag == null || tag.isEmpty() || !tag.equals(tag.trim()) || !tag.equals(tag.toLowerCase()) ||
                tag.length() > 64 || !seen.add(tag)) {
                throw new IllegalArgumentException("Channel tags must be exact, lowercase, bounded, and unique.");
            }
        }
    }

    static void canonicalId(String label, String value, int maximum) {
        if (value == null || value.isEmpty() || value.length() > maximum || !CANONICAL_ID.matcher(value).matches()) {
            throw new IllegalArgumentException(label + " is not canonical.");
        }
    }

    static void prompt(String value) {
        if (value == null || value.trim().isEmpty() || value.indexOf('\0') >= 0 || utf8Length(value) > 4096) {
            throw new IllegalArgumentException("Prompt must contain 1..4096 exact nonblank UTF-8 bytes without NUL.");
        }
    }

    private static void baseUrl(String value, String label) {
        if (value == null || value.isEmpty() || !value.equals(value.trim()) || value.endsWith("/")) {
            throw new IllegalArgumentException(label + " must be canonical without a trailing slash.");
        }
        try {
            URI uri = new URI(value);
            if (!("http".equals(uri.getScheme()) || "https".equals(uri.getScheme())) || uri.getHost() == null ||
                uri.getHost().isEmpty() || uri.getRawUserInfo() != null || uri.getRawQuery() != null ||
                uri.getRawFragment() != null) {
                throw new IllegalArgumentException(label + " must be one HTTP(S) origin or canonical base path.");
            }
        } catch (URISyntaxException error) {
            throw new IllegalArgumentException(label + " must be one HTTP(S) origin or canonical base path.", error);
        }
    }

    private static void exactText(String value, String label) {
        if (value == null || value.isEmpty() || !value.equals(value.trim()) || value.indexOf('\0') >= 0) {
            throw new IllegalArgumentException(label + " must be exact nonblank text.");
        }
    }

    private static int utf8Length(String value) {
        try {
            return StandardCharsets.UTF_8.newEncoder().encode(java.nio.CharBuffer.wrap(value)).remaining();
        } catch (CharacterCodingException error) {
            throw new IllegalArgumentException("Text must be valid Unicode.", error);
        }
    }
}
