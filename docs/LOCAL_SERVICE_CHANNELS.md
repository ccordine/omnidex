# Local Service Channels

Omnidex can run as a local HTTP service for other apps. A channel is a non-agent conversation surface: it follows the model/persona definition you configure, retrieves channel-scoped memory, calls the selected model, stores the turn, and returns a reply.

Channels do not run shell commands, patch files, or enter the autonomous agent loop. Use queued jobs when you want agent work. Use channels when you want a local assistant, support integration, roleplay participant, narrator, NPC, tutoring bot, or app-specific instruction follower.

## Install Ollama

Install Ollama from:

```text
https://ollama.com/download
```

Start it:

```bash
ollama serve
```

Pull a useful default model:

```bash
ollama pull qwen2.5:7b
ollama pull nomic-embed-text
```

Omnidex also self-heals first-use model misses for Ollama generation. If a configured model is not available and Ollama returns a missing-model error, the local Ollama client calls `/api/pull` for that model and retries the request. The first request can take as long as the model download.

For Docker, expose host Ollama beyond loopback:

```bash
OLLAMA_HOST=0.0.0.0:11434 ollama serve
```

Then set:

```bash
OLLAMA_BASE_URL=http://host.docker.internal:11434
```

For local non-Docker core:

```bash
OLLAMA_BASE_URL=http://localhost:11434
```

## Install Omnidex

From a checkout:

```bash
git clone <your-omnidex-repo-url> omnidex
cd omnidex
go build -o bin/core ./cmd/core
go build -o bin/omni ./cmd/omni
```

Or use Docker:

```bash
cp .env.example .env
docker compose up --build
```

Core listens on `http://localhost:8090` by default in Docker compose.

## Run Core As A Local Service

Repository-backed mode gives durable channels, durable messages, memory retrieval, and memory persistence:

```bash
export LLM_PROVIDER=ollama
export OLLAMA_BASE_URL=http://localhost:11434
export OLLAMA_MODEL=qwen2.5:7b
export OLLAMA_EMBEDDING_MODEL=nomic-embed-text
export DATABASE_URL=postgres://omnidex:omnidex@localhost:5432/omnidex?sslmode=disable
export MIGRATE_ON_STARTUP=true

./bin/core
```

Wrapper-only mode is useful for simple local embedding in another app. It exposes the persona and channel APIs without the queue/worker database. Channels are stored in memory for that process:

```bash
export WRAPPER_ONLY=true
export LLM_PROVIDER=ollama
export OLLAMA_BASE_URL=http://localhost:11434
export OLLAMA_MODEL=qwen2.5:7b

./bin/core
```

## Channel Concepts

A channel has:

- `id`: stable identifier from your app, such as `support-user-123`.
- `persona`: `assistant`, `roleplay`, or `narrate`.
- `system`: the model definition, rules, character sheet, or support policy.
- `provider`: usually `ollama` or `openai`.
- `model`: model name for this channel.
- `context`: JSON state your app owns.
- `tags`: memory scope tags. Omnidex always adds `channel:<id>`.

Each message request:

1. stores the user message,
2. retrieves relevant memory tagged for the channel,
3. appends recent channel messages as history,
4. calls the configured persona/model,
5. stores the assistant reply,
6. persists the user/reply as channel memory unless `remember:false`.

## Create A Support Assistant Channel

```bash
curl -X POST http://localhost:8090/v1/channels \
  -H 'content-type: application/json' \
  -d '{
    "id": "support-user-123",
    "name": "Support User 123",
    "persona": "assistant",
    "system": "You are a concise support assistant. Use known account context and ask for missing sensitive details instead of inventing.",
    "provider": "ollama",
    "model": "qwen2.5:7b",
    "context": {
      "product": "Omnidex",
      "plan": "pro"
    },
    "tags": ["support", "billing"]
  }'
```

Send a message:

```bash
curl -X POST http://localhost:8090/v1/channels/support-user-123/messages \
  -H 'content-type: application/json' \
  -d '{
    "prompt": "Can you help me understand my invoice?"
  }'
```

Response shape:

```json
{
  "channel": {"id": "support-user-123", "persona": "assistant"},
  "user_message": {"role": "user", "content": "Can you help me understand my invoice?"},
  "reply_message": {"role": "assistant", "content": "..."},
  "output": "...",
  "model": "qwen2.5:7b",
  "persona": "assistant",
  "latency_ms": 1234,
  "memory": []
}
```

## Add Durable Memory

In repository-backed mode you can seed durable channel memory:

```bash
curl -X POST http://localhost:8090/v1/memory \
  -H 'content-type: application/json' \
  -d '{
    "source": "manual",
    "kind": "preference",
    "content": "User prefers invoice summaries by email before renewal.",
    "tags": ["channel:support-user-123", "support", "billing"]
  }'
```

The next channel message retrieves that memory automatically.

## Roleplay Channel

```bash
curl -X POST http://localhost:8090/v1/channels \
  -H 'content-type: application/json' \
  -d '{
    "id": "rp-campaign-01",
    "persona": "roleplay",
    "system": "Stay in character as Captain Mara, a practical airship captain. Never narrate the user character actions.",
    "model": "qwen2.5:7b",
    "context": {
      "scene": "Night fog over the harbor",
      "party": ["Mara", "Ivo", "Selene"]
    },
    "tags": ["rp", "campaign-01"]
  }'
```

```bash
curl -X POST http://localhost:8090/v1/channels/rp-campaign-01/messages \
  -H 'content-type: application/json' \
  -d '{
    "prompt": "I ask Mara what she sees through the fog."
  }'
```

## Narrator Channel

```bash
curl -X POST http://localhost:8090/v1/channels \
  -H 'content-type: application/json' \
  -d '{
    "id": "story-01",
    "persona": "narrate",
    "system": "Write cinematic but concise narrative prose. Preserve continuity from memory and recent messages.",
    "model": "qwen2.5:7b",
    "tags": ["story", "draft-01"]
  }'
```

Then post messages to `/v1/channels/story-01/messages`.

## Per-Message Overrides

Override model, system, context, history, memory limit, or remembering for one message:

```bash
curl -X POST http://localhost:8090/v1/channels/support-user-123/messages \
  -H 'content-type: application/json' \
  -d '{
    "model": "llama3.1:8b",
    "system": "Answer as a terse API support assistant.",
    "context": {"ticket_id": "TCK-1001"},
    "history": [{"role": "user", "content": "Earlier context from my app"}],
    "memory_limit": 4,
    "remember": false,
    "prompt": "Draft the next support reply."
  }'
```

`remember:false` prevents that user/reply pair from being persisted as channel memory. The messages are still stored in the channel message log.

## JavaScript Integration

```js
async function sendOmnidexMessage(channelId, prompt) {
  const res = await fetch(`http://localhost:8090/v1/channels/${channelId}/messages`, {
    method: "POST",
    headers: {"content-type": "application/json"},
    body: JSON.stringify({prompt})
  });
  if (!res.ok) throw new Error(await res.text());
  const data = await res.json();
  return data.output;
}
```

## Python Integration

```python
import requests

def send(channel_id: str, prompt: str) -> str:
    res = requests.post(
        f"http://localhost:8090/v1/channels/{channel_id}/messages",
        json={"prompt": prompt},
        timeout=180,
    )
    res.raise_for_status()
    return res.json()["output"]
```

## Go Integration

```go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

func main() {
	body, _ := json.Marshal(map[string]string{
		"prompt": "Summarize this support ticket.",
	})
	resp, err := http.Post(
		"http://localhost:8090/v1/channels/support-user-123/messages",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	var out struct {
		Output string `json:"output"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		panic(err)
	}
	fmt.Println(out.Output)
}
```

## Direct Persona Endpoints

Use these when your app wants stateless calls:

- `POST /v1/instruct`
- `POST /v1/roleplay`
- `POST /v1/narrate`
- `POST /v1/reasoning`

Example:

```bash
curl -X POST http://localhost:8090/v1/instruct \
  -H 'content-type: application/json' \
  -d '{
    "model": "qwen2.5:7b",
    "system": "You are an API support assistant.",
    "context": {"product": "Omnidex"},
    "history": [],
    "prompt": "Explain how channels work in one paragraph."
  }'
```

Direct persona endpoints do not automatically persist channel memory. Use channels for memory-backed conversations.

## Operational Notes

- First use of a missing Ollama model can be slow because Omnidex pulls it before retrying.
- If Ollama is unreachable from Docker, bind Ollama to `0.0.0.0:11434` and use `OLLAMA_BASE_URL=http://host.docker.internal:11434`.
- Use smaller models for high-volume support or RP channels and larger models for complex reasoning.
- Use `tags` to scope memory per app, tenant, character, campaign, or customer.
- Keep `system` stable. Put changing app state in `context`.
