use std::collections::HashSet;

use url::Url;

use crate::{CreateChannelInput, DelegatedDataSourceInput, DirectDataSourceInput, Error};

const SSL_MODES: [&str; 6] = [
    "disable",
    "allow",
    "prefer",
    "require",
    "verify-ca",
    "verify-full",
];

pub(crate) fn configuration(base_url: &str, token: &str) -> Result<(), Error> {
    base_url_value(base_url, "Omnidex base URL")?;
    if !(32..=4096).contains(&token.len())
        || !token.bytes().all(|byte| (0x21..=0x7e).contains(&byte))
    {
        return invalid(
            "Omnidex integration token must contain 32..4096 exact visible ASCII bytes.",
        );
    }
    Ok(())
}

pub(crate) fn direct_data_source(input: &DirectDataSourceInput) -> Result<(), Error> {
    exact_text(&input.name, "Data-source name")?;
    if input.port == 0 {
        return invalid("PostgreSQL port must be between 1 and 65535.");
    }
    if !SSL_MODES.contains(&input.ssl_mode.as_str()) {
        return invalid("PostgreSQL SSL mode is unsupported.");
    }
    if input.use_dsn {
        exact_text(&input.dsn, "PostgreSQL DSN")?;
    } else {
        exact_text(&input.host, "PostgreSQL host")?;
        exact_text(&input.database_name, "PostgreSQL database")?;
        exact_text(&input.username, "PostgreSQL username")?;
    }
    Ok(())
}

pub(crate) fn delegated_data_source(input: &DelegatedDataSourceInput) -> Result<(), Error> {
    exact_text(&input.name, "Data-source name")?;
    base_url_value(&input.authority_url, "Delegated authority URL")?;
    const PREFIX: &str = "OMNIDEX_DELEGATED_AUTHORITY_";
    let suffix = input.credential_env.strip_prefix(PREFIX).unwrap_or("");
    let name = suffix.strip_suffix("_TOKEN").unwrap_or("").as_bytes();
    if name.is_empty()
        || input.credential_env.len() > 128
        || !name[0].is_ascii_uppercase()
        || !name
            .iter()
            .all(|byte| byte.is_ascii_uppercase() || byte.is_ascii_digit() || *byte == b'_')
    {
        return invalid(
            "Credential environment variable is outside the dedicated namespace OMNIDEX_DELEGATED_AUTHORITY_*.",
        );
    }
    Ok(())
}

pub(crate) fn channel(input: &CreateChannelInput) -> Result<(), Error> {
    canonical_id("Channel ID", &input.id, 96)?;
    canonical_id("Data-source ID", &input.data_source_id, 128)?;
    exact_text(&input.name, "Channel name")?;
    exact_text(&input.workspace_root, "Channel workspace root")?;
    if input.tags.len() > 32 {
        return invalid("Channel tags exceed 32 entries.");
    }
    let mut seen = HashSet::new();
    for tag in &input.tags {
        if tag.is_empty()
            || tag != tag.trim()
            || tag != &tag.to_lowercase()
            || tag.len() > 64
            || !seen.insert(tag)
        {
            return invalid("Channel tags must be exact, lowercase, bounded, and unique.");
        }
    }
    Ok(())
}

pub(crate) fn canonical_id(label: &str, value: &str, maximum: usize) -> Result<(), Error> {
    let bytes = value.as_bytes();
    if bytes.is_empty()
        || bytes.len() > maximum
        || !bytes[0].is_ascii_lowercase() && !bytes[0].is_ascii_digit()
        || !bytes.iter().all(|byte| {
            byte.is_ascii_lowercase() || byte.is_ascii_digit() || b"_.:-".contains(byte)
        })
    {
        return invalid(&format!("{label} is not canonical."));
    }
    Ok(())
}

pub(crate) fn prompt(value: &str) -> Result<(), Error> {
    if value.trim().is_empty() || value.len() > 4096 || value.contains('\0') {
        return invalid("Prompt must contain 1..4096 exact nonblank UTF-8 bytes without NUL.");
    }
    Ok(())
}

fn base_url_value(value: &str, label: &str) -> Result<(), Error> {
    if value.is_empty() || value != value.trim() || value.ends_with('/') {
        return invalid(&format!(
            "{label} must be canonical without a trailing slash."
        ));
    }
    let parsed = Url::parse(value).map_err(|_| {
        Error::InvalidArgument(format!(
            "{label} must be one HTTP(S) origin or canonical base path."
        ))
    })?;
    if !matches!(parsed.scheme(), "http" | "https")
        || parsed.host_str().is_none()
        || !parsed.username().is_empty()
        || parsed.password().is_some()
        || parsed.query().is_some()
        || parsed.fragment().is_some()
    {
        return invalid(&format!(
            "{label} must be one HTTP(S) origin or canonical base path."
        ));
    }
    Ok(())
}

fn exact_text(value: &str, label: &str) -> Result<(), Error> {
    if value.is_empty() || value != value.trim() || value.contains('\0') {
        return invalid(&format!("{label} must be exact nonblank text."));
    }
    Ok(())
}

fn invalid<T>(message: &str) -> Result<T, Error> {
    Err(Error::InvalidArgument(message.into()))
}
