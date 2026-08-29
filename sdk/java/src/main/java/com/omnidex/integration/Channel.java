package com.omnidex.integration;

import java.util.List;

public record Channel(
    String id,
    String scope,
    String name,
    List<String> tags,
    long projectId,
    String workspaceRoot,
    String dataSourceId,
    String mode
) {}
