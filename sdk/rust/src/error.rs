use thiserror::Error;

#[derive(Debug, Error)]
pub enum Error {
    #[error("{0}")]
    InvalidArgument(String),
    #[error("Omnidex transport failed: {0}")]
    Transport(String),
    #[error("Omnidex protocol failed: {0}")]
    Protocol(String),
    #[error("Omnidex integration API failed with HTTP {status}: {message}")]
    Api { status: u16, message: String },
    #[error("Secure authority generation failed: {0}")]
    Authority(String),
}
