package com.omnidex.integration;

public record DirectDataSourceInput(
    String name,
    String host,
    int port,
    String databaseName,
    String username,
    String password,
    String sslMode,
    boolean useDsn,
    String dsn
) {}
