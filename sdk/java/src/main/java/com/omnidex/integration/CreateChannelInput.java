package com.omnidex.integration;

import java.util.List;

public record CreateChannelInput(
    String id,
    String name,
    List<String> tags,
    String workspaceRoot,
    String dataSourceId
) {
    public CreateChannelInput {
        if (tags != null) tags = List.copyOf(tags);
    }
}
