use serde::de::DeserializeOwned;
use serde::Deserialize;

use crate::{
    Channel, ChannelMessage, DataSource, Error, JobDetails, MessagePage, SendMessageResult,
};

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct DataSourceEnvelope {
    source: DataSource,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct ChannelEnvelope {
    channel: Channel,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct ErrorEnvelope {
    error: String,
}

pub(crate) fn data_source(raw: &[u8]) -> Result<DataSource, Error> {
    let source = decode::<DataSourceEnvelope>(raw)?.source;
    if source.id.trim().is_empty()
        || source.name.trim().is_empty()
        || source.driver != "postgres"
        || !matches!(source.execution_mode.as_str(), "direct" | "delegated")
        || !source.read_only
    {
        return protocol("Omnidex returned an invalid data-source authority.");
    }
    Ok(source)
}

pub(crate) fn channel(raw: &[u8], id: &str, source_id: Option<&str>) -> Result<Channel, Error> {
    let channel = decode::<ChannelEnvelope>(raw)?.channel;
    validate_channel(&channel)?;
    if channel.id != id
        || source_id.is_some_and(|expected| channel.data_source_id != expected)
        || channel.mode != "assistant"
    {
        return protocol("Omnidex returned a channel outside the requested authority.");
    }
    Ok(channel)
}

pub(crate) fn message(
    raw: &[u8],
    channel_id: &str,
    prompt: &str,
) -> Result<SendMessageResult, Error> {
    let result = decode::<SendMessageResult>(raw)?;
    validate_channel(&result.channel)?;
    validate_message(&result.user_message)?;
    validate_job(result.job.id)?;
    if result.channel.id != channel_id
        || result.user_message.channel_id != channel_id
        || result.user_message.content != prompt
    {
        return protocol("Omnidex returned a message outside the requested authority.");
    }
    Ok(result)
}

pub(crate) fn message_page(raw: &[u8], channel_id: &str) -> Result<MessagePage, Error> {
    let page = decode::<MessagePage>(raw)?;
    if page.channel_id != channel_id
        || page.has_more != page.next_before_id.is_some()
        || page.next_before_id.is_some_and(|id| id < 1)
    {
        return protocol("Omnidex returned contradictory message-page authority.");
    }
    for message in &page.messages {
        validate_message(message)?;
    }
    Ok(page)
}

pub(crate) fn job_details(raw: &[u8], job_id: i64) -> Result<JobDetails, Error> {
    let details = decode::<JobDetails>(raw)?;
    validate_job(details.job.id)?;
    if details.job.id != job_id {
        return protocol("Omnidex returned a different job authority.");
    }
    Ok(details)
}

pub(crate) fn api_error(status: u16, raw: &[u8]) -> Error {
    let message = decode::<ErrorEnvelope>(raw)
        .ok()
        .map(|value| value.error)
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| "invalid error envelope".into());
    Error::Api { status, message }
}

fn validate_channel(channel: &Channel) -> Result<(), Error> {
    if channel.id.trim().is_empty()
        || channel.scope.trim().is_empty()
        || channel.mode.trim().is_empty()
    {
        return protocol("Omnidex returned an invalid channel authority.");
    }
    Ok(())
}

fn validate_message(message: &ChannelMessage) -> Result<(), Error> {
    if message.id < 1
        || message.channel_id.trim().is_empty()
        || !matches!(message.role.as_str(), "user" | "assistant")
    {
        return protocol("Omnidex returned an invalid channel message.");
    }
    Ok(())
}

fn validate_job(id: i64) -> Result<(), Error> {
    if id < 1 {
        return protocol("Omnidex returned an invalid job identity.");
    }
    Ok(())
}

fn decode<T: DeserializeOwned>(raw: &[u8]) -> Result<T, Error> {
    serde_json::from_slice(raw)
        .map_err(|error| Error::Protocol(format!("decode Omnidex response: {error}")))
}

fn protocol<T>(message: &str) -> Result<T, Error> {
    Err(Error::Protocol(message.into()))
}
