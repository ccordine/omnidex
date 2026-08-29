# PHP and Laravel SDK

Install from this repository as a Composer path package until a registry release exists. Bind one server-side singleton in Laravel using secrets from `config/services.php`:

```php
use Omnidex\Integration\Client;

$this->app->singleton(Client::class, fn () => new Client(
    baseUrl: config('services.omnidex.url'),
    token: config('services.omnidex.token'),
));
```

Register a delegated source and submit a permission-bound turn from authenticated application code:

```php
use Omnidex\Integration\Authority;
use Omnidex\Integration\CreateChannelInput;
use Omnidex\Integration\DelegatedDataSourceInput;
use Omnidex\Integration\SendMessageInput;

$source = $client->registerDelegatedDataSource(new DelegatedDataSourceInput(
    name: 'Clinical',
    authorityUrl: 'https://application.internal',
    credentialEnv: 'OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN',
));
$client->createChannel(new CreateChannelInput(
    id: 'clinical-chat', name: 'Clinical', tags: ['clinical'],
    workspaceRoot: '/workspace', dataSourceId: $source->id,
));

$authorityId = Authority::createDelegatedId();
// Store the authority-to-user/tenant mapping in server-owned, expiring state first.
$turn = $client->sendMessage('clinical-chat', new SendMessageInput(
    prompt: 'Find the knee collection.',
    delegatedDataAuthorityId: $authorityId,
));
```

The native transport rejects redirects and bounds responses. Inject `Http\Transport` to use an application-standard HTTP stack without changing the typed protocol. `getJob`, `getChannel`, and `listMessages` reconcile server state.
