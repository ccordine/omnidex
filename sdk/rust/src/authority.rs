use crate::Error;

pub fn create_delegated_authority_id() -> Result<String, Error> {
    let mut bytes = [0_u8; 32];
    getrandom::getrandom(&mut bytes).map_err(|error| Error::Authority(error.to_string()))?;
    let mut value = String::with_capacity(68);
    value.push_str("dba_");
    for byte in bytes {
        use std::fmt::Write;
        write!(&mut value, "{byte:02x}").expect("writing to a String cannot fail");
    }
    Ok(value)
}

pub fn validate_delegated_authority_id(value: &str) -> Result<(), Error> {
    if value.len() != 68
        || !value.starts_with("dba_")
        || !value.as_bytes()[4..]
            .iter()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(byte))
    {
        return Err(Error::InvalidArgument(
            "Delegated authority must be one opaque dba_ identity.".into(),
        ));
    }
    Ok(())
}
