package com.omnidex.integration;

public record SendMessageInput(String prompt, String delegatedDataAuthorityId) {
    public SendMessageInput(String prompt) {
        this(prompt, null);
    }
}
