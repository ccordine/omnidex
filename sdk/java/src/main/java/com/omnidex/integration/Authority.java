package com.omnidex.integration;

import java.security.SecureRandom;
import java.util.HexFormat;
import java.util.regex.Pattern;

public final class Authority {
    private static final SecureRandom RANDOM = new SecureRandom();
    private static final Pattern DELEGATED_ID = Pattern.compile("dba_[0-9a-f]{64}");

    private Authority() {}

    public static String createDelegatedId() {
        byte[] bytes = new byte[32];
        RANDOM.nextBytes(bytes);
        return "dba_" + HexFormat.of().formatHex(bytes);
    }

    public static void validateDelegatedId(String value) {
        if (value == null || !DELEGATED_ID.matcher(value).matches()) {
            throw new IllegalArgumentException("Delegated authority must be one opaque dba_ identity.");
        }
    }
}
