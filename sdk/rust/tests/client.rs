use std::collections::{BTreeMap, VecDeque};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use omnidex_integration::{
    create_delegated_authority_id, validate_delegated_authority_id, Client,
    DelegatedDataSourceInput, Error, HttpRequest, HttpResponse, SendMessageInput, Transport,
};
use serde_json::{json, Value};

const TOKEN: &str = "integration-token-0123456789abcdef";

#[test]
fn delegated_registration_carries_no_credentials() {
    let transport = Arc::new(FakeTransport::new(vec![json_response(
        201,
        json!({
            "source": {
                "id": "source-1", "name": "Clinical", "driver": "postgres",
                "execution_mode": "delegated", "authority_url": "https://application.internal",
                "credential_env": "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN", "read_only": true
            }
        }),
    )]));
    let client = client(transport.clone());
    let source = client
        .register_delegated_data_source(DelegatedDataSourceInput {
            name: "Clinical".into(),
            authority_url: "https://application.internal".into(),
            credential_env: "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN".into(),
        })
        .expect("delegated source");
    assert_eq!(source.id, "source-1");

    let request = transport.requests().remove(0);
    assert_eq!(request.method, "POST");
    assert_eq!(
        request.url,
        "https://omnidex.internal/v1/integrations/data-sources"
    );
    assert_eq!(
        request.headers.get("Authorization").unwrap(),
        &format!("Bearer {TOKEN}")
    );
    let body: Value = serde_json::from_slice(request.body.as_deref().unwrap()).unwrap();
    assert_eq!(
        body,
        json!({
            "name": "Clinical", "driver": "postgres", "execution_mode": "delegated",
            "host": "", "port": 0, "database_name": "", "username": "", "password": "",
            "ssl_mode": "", "use_dsn": false, "dsn": "", "authority_url": "https://application.internal",
            "credential_env": "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN"
        })
    );
}

#[test]
fn message_preserves_exact_prompt_and_authority() {
    let authority = format!("dba_{}", "a".repeat(64));
    let prompt = "  Find the knee collection.\nKeep context. ";
    let transport = Arc::new(FakeTransport::new(vec![json_response(
        202,
        json!({
            "channel": {"id": "clinical-chat", "scope": "user", "data_source_id": "source-1", "mode": "assistant"},
            "user_message": {"id": 12, "channel_id": "clinical-chat", "role": "user", "content": prompt,
                "created_at": "2026-08-19T00:00:00Z"},
            "job": {"id": 73, "instruction": prompt, "pipeline": "chat"}
        }),
    )]));
    let result = client(transport.clone())
        .send_message(
            "clinical-chat",
            SendMessageInput {
                prompt: prompt.into(),
                delegated_data_authority_id: Some(authority.clone()),
            },
        )
        .expect("message");
    assert_eq!(result.job.id, 73);
    assert_eq!(result.user_message.content, prompt);
    let request = transport.requests().remove(0);
    assert_eq!(
        request.url,
        "https://omnidex.internal/v1/integrations/channels/clinical-chat/messages"
    );
    let body: Value = serde_json::from_slice(request.body.as_deref().unwrap()).unwrap();
    assert_eq!(
        body,
        json!({"prompt": prompt, "delegated_data_authority_id": authority})
    );
}

#[test]
fn invalid_authority_fails_before_transport() {
    let transport = Arc::new(FakeTransport::new(vec![]));
    let error = client(transport.clone())
        .send_message(
            "clinical-chat",
            SendMessageInput {
                prompt: "question".into(),
                delegated_data_authority_id: Some("invalid".into()),
            },
        )
        .unwrap_err();
    assert!(error.to_string().contains("opaque dba_"));
    assert!(transport.requests().is_empty());
}

#[test]
fn responses_fail_closed() {
    let transport = Arc::new(FakeTransport::new(vec![
        json_response(
            200,
            json!({
                "channel_id": "clinical-chat", "messages": [], "next_before_id": null,
                "has_more": false, "unknown": true
            }),
        ),
        json_response(409, json!({"error": "channel already has an active turn"})),
    ]));
    let client = client(transport);
    let unknown = client.list_messages("clinical-chat", 24, None).unwrap_err();
    assert!(unknown.to_string().contains("unknown field"));
    let api = client
        .send_message(
            "clinical-chat",
            SendMessageInput {
                prompt: "question".into(),
                delegated_data_authority_id: None,
            },
        )
        .unwrap_err();
    assert!(
        matches!(api, Error::Api { status: 409, ref message } if message == "channel already has an active turn")
    );
}

#[test]
fn configuration_and_generated_authority_are_bounded() {
    assert!(Client::new("file:///tmp/omnidex", TOKEN).is_err());
    assert!(Client::new("https://omnidex.internal/", TOKEN).is_err());
    assert!(Client::new("https://omnidex.internal", "short").is_err());
    validate_delegated_authority_id(&create_delegated_authority_id().unwrap()).unwrap();
    let transport = Arc::new(FakeTransport::new(vec![]));
    let error = client(transport.clone())
        .register_delegated_data_source(DelegatedDataSourceInput {
            name: "Clinical".into(),
            authority_url: "https://application.internal".into(),
            credential_env: "OPENAI_API_KEY".into(),
        })
        .unwrap_err();
    assert!(error.to_string().contains("dedicated namespace"));
    assert!(transport.requests().is_empty());
}

fn client(transport: Arc<FakeTransport>) -> Client {
    Client::with_transport(
        "https://omnidex.internal",
        TOKEN,
        transport,
        Duration::from_secs(2),
    )
    .unwrap()
}

fn json_response(status: u16, value: Value) -> HttpResponse {
    HttpResponse {
        status,
        headers: BTreeMap::from([("content-type".into(), vec!["application/json".into()])]),
        body: serde_json::to_vec(&value).unwrap(),
    }
}

struct FakeTransport {
    responses: Mutex<VecDeque<HttpResponse>>,
    requests: Mutex<Vec<HttpRequest>>,
}

impl FakeTransport {
    fn new(responses: Vec<HttpResponse>) -> Self {
        Self {
            responses: Mutex::new(responses.into()),
            requests: Mutex::new(vec![]),
        }
    }

    fn requests(&self) -> Vec<HttpRequest> {
        self.requests.lock().unwrap().clone()
    }
}

impl Transport for FakeTransport {
    fn send(&self, request: HttpRequest) -> Result<HttpResponse, Error> {
        self.requests.lock().unwrap().push(request);
        self.responses
            .lock()
            .unwrap()
            .pop_front()
            .ok_or_else(|| Error::Transport("unexpected transport call".into()))
    }
}
