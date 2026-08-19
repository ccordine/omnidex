# Rust SDK

The crate provides a blocking Rustls transport and an injectable `Transport` trait.

```rust
use omnidex_integration::{
    create_delegated_authority_id, Client, CreateChannelInput,
    DelegatedDataSourceInput, SendMessageInput,
};

let client = Client::new(&omnidex_url, &integration_token)?;
let source = client.register_delegated_data_source(DelegatedDataSourceInput {
    name: "Clinical".into(),
    authority_url: "https://application.internal".into(),
    credential_env: "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN".into(),
})?;
client.create_channel(CreateChannelInput {
    id: "clinical-chat".into(), name: "Clinical".into(), tags: vec!["clinical".into()],
    workspace_root: "/workspace".into(), data_source_id: source.id,
})?;
let turn = client.send_message("clinical-chat", SendMessageInput {
    prompt: "Find the knee collection.".into(),
    delegated_data_authority_id: Some(create_delegated_authority_id()?),
})?;
```

`get_job`, `get_channel`, and cursor-based `list_messages` return typed server state. `Client::with_transport` accepts an application transport for custom policy or tests.
