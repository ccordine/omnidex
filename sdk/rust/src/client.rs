use std::collections::BTreeMap;
use std::sync::Arc;
use std::time::Duration;

use serde::Serialize;

use crate::responses;
use crate::validation;
use crate::{
    validate_delegated_authority_id, Channel, CreateChannelInput, DataSource,
    DelegatedDataSourceInput, DirectDataSourceInput, Error, HttpRequest, JobDetails, MessagePage,
    ReqwestTransport, SendMessageInput, SendMessageResult, Transport,
};

const MAX_RESPONSE_BYTES: usize = 16 * 1024 * 1024;

pub struct Client {
    base_url: String,
    token: String,
    transport: Arc<dyn Transport>,
    timeout: Duration,
}

impl Client {
    pub fn new(base_url: &str, token: &str) -> Result<Self, Error> {
        validation::configuration(base_url, token)?;
        let transport = Arc::new(ReqwestTransport::new()?);
        Self::with_transport(base_url, token, transport, Duration::from_secs(30))
    }

    pub fn with_transport(
        base_url: &str,
        token: &str,
        transport: Arc<dyn Transport>,
        timeout: Duration,
    ) -> Result<Self, Error> {
        validation::configuration(base_url, token)?;
        if timeout.is_zero() {
            return Err(Error::InvalidArgument(
                "HTTP timeout must be positive.".into(),
            ));
        }
        Ok(Self {
            base_url: base_url.into(),
            token: token.into(),
            transport,
            timeout,
        })
    }

    pub fn register_direct_data_source(
        &self,
        input: DirectDataSourceInput,
    ) -> Result<DataSource, Error> {
        validation::direct_data_source(&input)?;
        let body = DataSourceRequest {
            name: &input.name,
            driver: "postgres",
            execution_mode: "direct",
            host: &input.host,
            port: input.port,
            database_name: &input.database_name,
            username: &input.username,
            password: &input.password,
            ssl_mode: &input.ssl_mode,
            use_dsn: input.use_dsn,
            dsn: &input.dsn,
            authority_url: "",
            credential_env: "",
        };
        let response = self.request(
            "POST",
            "/v1/integrations/data-sources",
            Some(encode(&body)?),
            201,
        )?;
        responses::data_source(&response.body)
    }

    pub fn register_delegated_data_source(
        &self,
        input: DelegatedDataSourceInput,
    ) -> Result<DataSource, Error> {
        validation::delegated_data_source(&input)?;
        let body = DataSourceRequest {
            name: &input.name,
            driver: "postgres",
            execution_mode: "delegated",
            host: "",
            port: 0,
            database_name: "",
            username: "",
            password: "",
            ssl_mode: "",
            use_dsn: false,
            dsn: "",
            authority_url: &input.authority_url,
            credential_env: &input.credential_env,
        };
        let response = self.request(
            "POST",
            "/v1/integrations/data-sources",
            Some(encode(&body)?),
            201,
        )?;
        responses::data_source(&response.body)
    }

    pub fn create_channel(&self, input: CreateChannelInput) -> Result<Channel, Error> {
        validation::channel(&input)?;
        let body = ChannelRequest {
            id: &input.id,
            name: &input.name,
            tags: &input.tags,
            workspace_root: &input.workspace_root,
            data_source_id: &input.data_source_id,
            mode: "assistant",
        };
        let response = self.request(
            "POST",
            "/v1/integrations/channels",
            Some(encode(&body)?),
            201,
        )?;
        responses::channel(&response.body, &input.id, Some(&input.data_source_id))
    }

    pub fn get_channel(&self, channel_id: &str) -> Result<Channel, Error> {
        validation::canonical_id("Channel ID", channel_id, 96)?;
        let path = format!("/v1/integrations/channels/{channel_id}");
        let response = self.request("GET", &path, None, 200)?;
        responses::channel(&response.body, channel_id, None)
    }

    pub fn send_message(
        &self,
        channel_id: &str,
        input: SendMessageInput,
    ) -> Result<SendMessageResult, Error> {
        validation::canonical_id("Channel ID", channel_id, 96)?;
        validation::prompt(&input.prompt)?;
        if let Some(authority) = &input.delegated_data_authority_id {
            validate_delegated_authority_id(authority)?;
        }
        let body = MessageRequest {
            prompt: &input.prompt,
            delegated_data_authority_id: input.delegated_data_authority_id.as_deref(),
        };
        let path = format!("/v1/integrations/channels/{channel_id}/messages");
        let response = self.request("POST", &path, Some(encode(&body)?), 202)?;
        responses::message(&response.body, channel_id, &input.prompt)
    }

    pub fn list_messages(
        &self,
        channel_id: &str,
        limit: u16,
        before_id: Option<i64>,
    ) -> Result<MessagePage, Error> {
        validation::canonical_id("Channel ID", channel_id, 96)?;
        if !(1..=200).contains(&limit) || before_id.is_some_and(|id| id < 1) {
            return Err(Error::InvalidArgument(
                "Message page bounds are invalid.".into(),
            ));
        }
        let mut path = format!("/v1/integrations/channels/{channel_id}/messages?limit={limit}");
        if let Some(id) = before_id {
            path.push_str(&format!("&before_id={id}"));
        }
        let response = self.request("GET", &path, None, 200)?;
        responses::message_page(&response.body, channel_id)
    }

    pub fn get_job(&self, job_id: i64) -> Result<JobDetails, Error> {
        if job_id < 1 {
            return Err(Error::InvalidArgument("Job ID must be positive.".into()));
        }
        let response =
            self.request("GET", &format!("/v1/integrations/jobs/{job_id}"), None, 200)?;
        responses::job_details(&response.body, job_id)
    }

    fn request(
        &self,
        method: &str,
        path: &str,
        body: Option<Vec<u8>>,
        expected_status: u16,
    ) -> Result<crate::HttpResponse, Error> {
        let mut headers = BTreeMap::from([
            ("Authorization".into(), format!("Bearer {}", self.token)),
            ("Accept".into(), "application/json".into()),
        ]);
        if body.is_some() {
            headers.insert("Content-Type".into(), "application/json".into());
        }
        let response = self.transport.send(HttpRequest {
            method: method.into(),
            url: format!("{}{}", self.base_url, path),
            headers,
            body,
            timeout: self.timeout,
        })?;
        if response.body.len() > MAX_RESPONSE_BYTES {
            return Err(Error::Protocol(format!(
                "Omnidex integration response exceeds {MAX_RESPONSE_BYTES} bytes."
            )));
        }
        if response.status != expected_status {
            return Err(responses::api_error(response.status, &response.body));
        }
        let content_type = response
            .header("content-type")?
            .unwrap_or("")
            .split(';')
            .next()
            .unwrap_or("")
            .trim()
            .to_ascii_lowercase();
        if content_type != "application/json" {
            return Err(Error::Protocol(
                "Omnidex returned a non-JSON response.".into(),
            ));
        }
        Ok(response)
    }
}

#[derive(Serialize)]
struct DataSourceRequest<'a> {
    name: &'a str,
    driver: &'a str,
    execution_mode: &'a str,
    host: &'a str,
    port: u16,
    database_name: &'a str,
    username: &'a str,
    password: &'a str,
    ssl_mode: &'a str,
    use_dsn: bool,
    dsn: &'a str,
    authority_url: &'a str,
    credential_env: &'a str,
}

#[derive(Serialize)]
struct ChannelRequest<'a> {
    id: &'a str,
    name: &'a str,
    tags: &'a [String],
    workspace_root: &'a str,
    data_source_id: &'a str,
    mode: &'a str,
}

#[derive(Serialize)]
struct MessageRequest<'a> {
    prompt: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    delegated_data_authority_id: Option<&'a str>,
}

fn encode<T: Serialize>(value: &T) -> Result<Vec<u8>, Error> {
    serde_json::to_vec(value)
        .map_err(|error| Error::Protocol(format!("encode Omnidex request: {error}")))
}
