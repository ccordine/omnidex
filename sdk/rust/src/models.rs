use serde::Deserialize;
use serde_json::Value;

#[derive(Debug, Clone)]
pub struct DirectDataSourceInput {
    pub name: String,
    pub host: String,
    pub port: u16,
    pub database_name: String,
    pub username: String,
    pub password: String,
    pub ssl_mode: String,
    pub use_dsn: bool,
    pub dsn: String,
}

#[derive(Debug, Clone)]
pub struct DelegatedDataSourceInput {
    pub name: String,
    pub authority_url: String,
    pub credential_env: String,
}

#[derive(Debug, Clone)]
pub struct CreateChannelInput {
    pub id: String,
    pub name: String,
    pub tags: Vec<String>,
    pub workspace_root: String,
    pub data_source_id: String,
}

#[derive(Debug, Clone)]
pub struct SendMessageInput {
    pub prompt: String,
    pub delegated_data_authority_id: Option<String>,
}

#[derive(Debug, Clone, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct DataSource {
    pub id: String,
    pub name: String,
    pub driver: String,
    pub execution_mode: String,
    #[serde(default)]
    pub host: String,
    #[serde(default)]
    pub port: u16,
    #[serde(default)]
    pub database_name: String,
    #[serde(default)]
    pub username: String,
    #[serde(default)]
    pub ssl_mode: String,
    #[serde(default)]
    pub use_dsn: bool,
    #[serde(default)]
    pub authority_url: String,
    #[serde(default)]
    pub credential_env: String,
    pub read_only: bool,
    #[serde(default)]
    pub password_set: bool,
    #[serde(default)]
    pub password_hint: String,
    #[serde(default)]
    pub last_test_status: String,
    #[serde(default)]
    pub last_test_message: String,
    #[serde(default)]
    pub last_test_at: Option<String>,
    #[serde(default)]
    pub catalog_updated_at: Option<String>,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub updated_at: String,
}

#[derive(Debug, Clone, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Channel {
    pub id: String,
    pub scope: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub tags: Vec<String>,
    #[serde(default)]
    pub project_id: i64,
    #[serde(default)]
    pub workspace_root: String,
    #[serde(default)]
    pub data_source_id: String,
    pub mode: String,
    #[serde(default)]
    pub roleplay_viewpoint_character_id: String,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub updated_at: String,
}

#[derive(Debug, Clone, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct ChannelMessage {
    pub id: i64,
    pub channel_id: String,
    pub role: String,
    pub content: String,
    pub created_at: String,
}

#[derive(Debug, Clone, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Job {
    pub id: i64,
    pub instruction: String,
    pub pipeline: String,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub result: String,
    #[serde(default)]
    pub error: String,
    #[serde(default)]
    pub metadata: Option<Value>,
    #[serde(default)]
    pub current_generation: i64,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub updated_at: String,
    #[serde(default)]
    pub completed_at: Option<String>,
}

#[derive(Debug, Clone, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct JobStep {
    pub id: i64,
    pub job_id: i64,
    pub action: String,
    #[serde(default)]
    pub sort_index: i64,
    pub status: String,
    #[serde(default)]
    pub generation: i64,
    #[serde(default)]
    pub superseded_at_generation: Option<i64>,
    #[serde(default)]
    pub worker_id: String,
    #[serde(default)]
    pub output: String,
    #[serde(default)]
    pub error: String,
    #[serde(default)]
    pub started_at: Option<String>,
    #[serde(default)]
    pub finished_at: Option<String>,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub updated_at: String,
}

#[derive(Debug, Clone, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct JobContext {
    pub id: i64,
    pub step_id: i64,
    pub key: String,
    pub value: String,
    pub created_at: String,
}

#[derive(Debug, Clone, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct SendMessageResult {
    pub channel: Channel,
    pub user_message: ChannelMessage,
    pub job: Job,
}

#[derive(Debug, Clone, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct MessagePage {
    pub channel_id: String,
    pub messages: Vec<ChannelMessage>,
    pub next_before_id: Option<i64>,
    pub has_more: bool,
}

#[derive(Debug, Clone, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct JobDetails {
    pub job: Job,
    pub steps: Vec<JobStep>,
    pub contexts: Vec<JobContext>,
}
