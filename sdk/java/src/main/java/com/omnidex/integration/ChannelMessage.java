package com.omnidex.integration;

public record ChannelMessage(long id, String channelId, String role, String content, String createdAt) {}
