# Method and route contract

Every route requires exactly one `Authorization: Bearer <OMNIDEX_INTEGRATION_API_TOKEN>` header and returns `application/json` on the typed success path.

| SDK operation | HTTP operation | Success |
| --- | --- | --- |
| Register direct data source | `POST /v1/integrations/data-sources` | `201` |
| Register delegated data source | `POST /v1/integrations/data-sources` | `201` |
| Create assistant channel | `POST /v1/integrations/channels` | `201` |
| Get assistant channel | `GET /v1/integrations/channels/{channel_id}` | `200` |
| Send message | `POST /v1/integrations/channels/{channel_id}/messages` | `202` |
| List messages | `GET /v1/integrations/channels/{channel_id}/messages?limit=…&before_id=…` | `200` |
| Get job | `GET /v1/integrations/jobs/{job_id}` | `200` |

Channel and data-source identifiers are canonical lowercase identities. Transcript pagination uses an older-message cursor while each returned page remains chronological: pass `next_before_id` from one response as `before_id` in the next request. A non-null `next_before_id` is authoritative for `has_more=true`.

The exact delegated request examples are:

- [`delegated-data-source-request.json`](fixtures/delegated-data-source-request.json)
- [`create-channel-request.json`](fixtures/create-channel-request.json)
- [`delegated-message-request.json`](fixtures/delegated-message-request.json)

The stable integration bearer token authenticates the application backend to Omnidex. `delegated_data_authority_id` is a separate, single-turn opaque capability that lets the application authority recover its own permission context. Neither token is a database credential, and neither may be placed in a prompt.
