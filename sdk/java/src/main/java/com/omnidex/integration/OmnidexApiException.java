package com.omnidex.integration;

public final class OmnidexApiException extends RuntimeException {
    private final int status;
    private final String apiMessage;

    public OmnidexApiException(int status, String apiMessage) {
        super("Omnidex integration API failed with HTTP " + status + ": " + apiMessage);
        this.status = status;
        this.apiMessage = apiMessage;
    }

    public int status() { return status; }
    public String apiMessage() { return apiMessage; }
}
