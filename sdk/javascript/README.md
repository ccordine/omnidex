# JavaScript SDK

The ESM package targets trusted Node.js/server runtimes. Never expose the integration bearer in a browser bundle.

```js
import { newDelegatedAuthorityId, OmnidexClient } from "@omnidex/integration-sdk";

const client = new OmnidexClient({
  baseUrl: process.env.OMNIDEX_URL,
  token: process.env.OMNIDEX_INTEGRATION_API_TOKEN,
});

const source = await client.registerDelegatedDataSource({
  name: "Clinical",
  authorityUrl: "https://application.internal",
  credentialEnv: "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN",
});
await client.createChannel({
  id: "clinical-chat", name: "Clinical", tags: ["clinical"],
  workspaceRoot: "/workspace", dataSourceId: source.id,
});
const turn = await client.sendMessage("clinical-chat", {
  prompt: "Find the knee collection.",
  delegatedDataAuthorityId: newDelegatedAuthorityId(),
});
```

All calls accept an `AbortSignal`. `listMessages` is cursor-paginated; `getJob` and `getChannel` read authoritative server state. A custom Fetch implementation can be injected for controlled TLS or tests.
