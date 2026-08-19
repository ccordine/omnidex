use std::collections::BTreeMap;
use std::io::Read;
use std::time::Duration;

use crate::Error;

const MAX_RESPONSE_BYTES: usize = 16 * 1024 * 1024;

#[derive(Debug, Clone)]
pub struct HttpRequest {
    pub method: String,
    pub url: String,
    pub headers: BTreeMap<String, String>,
    pub body: Option<Vec<u8>>,
    pub timeout: Duration,
}

#[derive(Debug, Clone)]
pub struct HttpResponse {
    pub status: u16,
    pub headers: BTreeMap<String, Vec<String>>,
    pub body: Vec<u8>,
}

impl HttpResponse {
    pub fn header(&self, name: &str) -> Result<Option<&str>, Error> {
        let mut matches = self
            .headers
            .iter()
            .filter(|(key, _)| key.eq_ignore_ascii_case(name));
        let Some((_, values)) = matches.next() else {
            return Ok(None);
        };
        if matches.next().is_some() || values.len() != 1 {
            return Err(Error::Protocol(
                "Omnidex response repeats an HTTP header.".into(),
            ));
        }
        Ok(values.first().map(String::as_str))
    }
}

pub trait Transport: Send + Sync {
    fn send(&self, request: HttpRequest) -> Result<HttpResponse, Error>;
}

pub struct ReqwestTransport {
    client: reqwest::blocking::Client,
}

impl ReqwestTransport {
    pub fn new() -> Result<Self, Error> {
        let client = reqwest::blocking::Client::builder()
            .redirect(reqwest::redirect::Policy::none())
            .build()
            .map_err(|error| Error::Transport(error.to_string()))?;
        Ok(Self { client })
    }
}

impl Transport for ReqwestTransport {
    fn send(&self, request: HttpRequest) -> Result<HttpResponse, Error> {
        let method = reqwest::Method::from_bytes(request.method.as_bytes())
            .map_err(|error| Error::Transport(error.to_string()))?;
        let mut builder = self
            .client
            .request(method, &request.url)
            .timeout(request.timeout);
        for (name, value) in request.headers {
            builder = builder.header(&name, &value);
        }
        if let Some(body) = request.body {
            builder = builder.body(body);
        }
        let response = builder
            .send()
            .map_err(|error| Error::Transport(error.to_string()))?;
        if response
            .content_length()
            .is_some_and(|length| length > MAX_RESPONSE_BYTES as u64)
        {
            return Err(Error::Protocol(format!(
                "Omnidex integration response exceeds {MAX_RESPONSE_BYTES} bytes."
            )));
        }
        let status = response.status().as_u16();
        let mut headers = BTreeMap::<String, Vec<String>>::new();
        for (name, value) in response.headers() {
            let value = value.to_str().map_err(|_| {
                Error::Protocol("Omnidex response contains a non-text HTTP header.".into())
            })?;
            headers
                .entry(name.as_str().to_ascii_lowercase())
                .or_default()
                .push(value.into());
        }
        let mut body = Vec::new();
        response
            .take((MAX_RESPONSE_BYTES + 1) as u64)
            .read_to_end(&mut body)
            .map_err(|error| Error::Transport(error.to_string()))?;
        if body.len() > MAX_RESPONSE_BYTES {
            return Err(Error::Protocol(format!(
                "Omnidex integration response exceeds {MAX_RESPONSE_BYTES} bytes."
            )));
        }
        Ok(HttpResponse {
            status,
            headers,
            body,
        })
    }
}
