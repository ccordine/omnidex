# Java SDK

The package requires Java 21 and uses only the JDK for its public API and default HTTP transport.

```java
import com.omnidex.integration.*;

OmnidexClient client = new OmnidexClient(omnidexUrl, integrationToken);
DataSource source = client.registerDelegatedDataSource(new DelegatedDataSourceInput(
    "Clinical", "https://application.internal", "OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN"
));
client.createChannel(new CreateChannelInput(
    "clinical-chat", "Clinical", List.of("clinical"), "/workspace", source.id()
));
SendMessageResult turn = client.sendMessage(
    "clinical-chat",
    new SendMessageInput("Find the knee collection.", Authority.createDelegatedId())
);
```

Use `getJob`, `getChannel`, and `listMessages` for server-authoritative reads. The injectable `Transport` boundary supports custom enterprise TLS handling and deterministic tests. The built-in `JdkTransport` rejects redirects and bounds response bodies.
