# Go SDK

Import `github.com/gryph/omnidex/sdk/go` (package name `omnidex`). Every method accepts a caller-owned context.

```go
client, err := omnidex.NewClient(omnidexURL, integrationToken)
if err != nil {
    return err
}

source, err := client.RegisterDelegatedDataSource(ctx, omnidex.DelegatedDataSourceInput{
    Name: "Clinical", AuthorityURL: "https://application.internal",
    CredentialEnv: "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN",
})
if err != nil {
    return err
}

_, err = client.CreateChannel(ctx, omnidex.CreateChannelInput{
    ID: "clinical-chat", Name: "Clinical", Tags: []string{"clinical"},
    WorkspaceRoot: "/workspace", DataSourceID: source.ID,
})
if err != nil {
    return err
}

authorityID, err := omnidex.NewDelegatedAuthorityID()
if err != nil {
    return err
}
turn, err := client.SendMessage(ctx, "clinical-chat", omnidex.SendMessageInput{
    Prompt: "Find the knee collection.", DelegatedDataAuthorityID: authorityID,
})
```

Use `GetJob`, `GetChannel`, and cursor-based `ListMessages` to reconcile server-authoritative state. `NewClientWithHTTPClient` accepts an injected bounded client for custom TLS roots or tests and always replaces redirect handling with rejection.
