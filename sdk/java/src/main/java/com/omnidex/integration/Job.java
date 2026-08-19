package com.omnidex.integration;

import java.util.Map;

public record Job(
    long id,
    String instruction,
    String pipeline,
    String status,
    String result,
    String error,
    long currentGeneration,
    Map<String, Object> fields
) {}
