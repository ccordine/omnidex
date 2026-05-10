package artifacts

import (
    "encoding/json"
    "fmt"
)

func MarshalPayload(kind string, version string, payload any) (Envelope, error) {
    raw, err := json.Marshal(payload)
    if err != nil {
        return Envelope{}, fmt.Errorf("marshal artifact payload: %w", err)
    }
    env := Envelope{
        Kind:    kind,
        Version: version,
        Payload: raw,
    }
    if err := env.Validate(); err != nil {
        return Envelope{}, err
    }
    return env, nil
}

func DecodePayload[T any](env Envelope) (T, error) {
    var out T
    if err := env.Validate(); err != nil {
        return out, err
    }
    if err := json.Unmarshal(env.Payload, &out); err != nil {
        return out, fmt.Errorf("decode artifact payload: %w", err)
    }
    return out, nil
}
