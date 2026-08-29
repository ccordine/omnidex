package com.omnidex.integration;

import java.util.Map;

public record DataSource(
    String id,
    String name,
    String driver,
    String executionMode,
    boolean readOnly,
    Map<String, Object> fields
) {}
