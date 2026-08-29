package com.omnidex.integration;

public record SendMessageResult(Channel channel, ChannelMessage userMessage, Job job) {}
