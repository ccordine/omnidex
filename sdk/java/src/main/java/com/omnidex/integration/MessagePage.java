package com.omnidex.integration;

import java.util.List;

public record MessagePage(
    String channelId,
    List<ChannelMessage> messages,
    Long nextBeforeId,
    boolean hasMore
) {}
