<?php

declare(strict_types=1);

use Omnidex\Integration\Authority;
use Omnidex\Integration\Client;
use Omnidex\Integration\CreateChannelInput;
use Omnidex\Integration\DelegatedDataSourceInput;
use Omnidex\Integration\Http\Response;
use Omnidex\Integration\Http\Transport;
use Omnidex\Integration\OmnidexApiException;
use Omnidex\Integration\SendMessageInput;

spl_autoload_register(static function (string $class): void {
    $prefix = 'Omnidex\\Integration\\';
    if (!str_starts_with($class, $prefix)) {
        return;
    }
    $relative = str_replace('\\', '/', substr($class, strlen($prefix)));
    require dirname(__DIR__) . '/src/' . $relative . '.php';
});

const TOKEN = 'integration-token-0123456789abcdef';

final class FakeTransport implements Transport
{
    /** @var list<callable(string, string, array<string, string>, ?string, int): Response> */
    private array $handlers;

    /** @param list<callable(string, string, array<string, string>, ?string, int): Response> $handlers */
    public function __construct(array $handlers)
    {
        $this->handlers = $handlers;
    }

    public function send(string $method, string $url, array $headers, ?string $body, int $timeoutSeconds): Response
    {
        $handler = array_shift($this->handlers);
        if ($handler === null) {
            throw new RuntimeException('Unexpected transport call.');
        }
        return $handler($method, $url, $headers, $body, $timeoutSeconds);
    }

    public function remaining(): int
    {
        return count($this->handlers);
    }
}

function jsonResponse(int $status, array $value): Response
{
    return new Response($status, ['content-type' => 'application/json'], json_encode($value, JSON_THROW_ON_ERROR));
}

function same(mixed $expected, mixed $actual, string $label): void
{
    if ($expected !== $actual) {
        throw new RuntimeException($label . ' mismatch: ' . var_export($actual, true));
    }
}

function throws(callable $callback, string $class, string $fragment): Throwable
{
    try {
        $callback();
    } catch (Throwable $error) {
        if (!$error instanceof $class || !str_contains($error->getMessage(), $fragment)) {
            throw new RuntimeException('Unexpected exception: ' . $error::class . ': ' . $error->getMessage());
        }
        return $error;
    }
    throw new RuntimeException('Expected exception was not thrown.');
}

function testDelegatedRegistration(): void
{
    $transport = new FakeTransport([
        static function (string $method, string $url, array $headers, ?string $body): Response {
            same('POST', $method, 'method');
            same('https://omnidex.internal/v1/integrations/data-sources', $url, 'URL');
            same('Bearer ' . TOKEN, $headers['Authorization'], 'authorization');
            same([
                'name' => 'Clinical', 'driver' => 'postgres', 'execution_mode' => 'delegated',
                'host' => '', 'port' => 0, 'database_name' => '', 'username' => '', 'password' => '',
                'ssl_mode' => '', 'use_dsn' => false, 'dsn' => '',
                'authority_url' => 'https://application.internal',
                'credential_env' => 'OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN',
            ], json_decode((string) $body, true, 512, JSON_THROW_ON_ERROR), 'delegated body');
            return jsonResponse(201, ['source' => [
                'id' => 'source-1', 'name' => 'Clinical', 'driver' => 'postgres',
                'execution_mode' => 'delegated', 'authority_url' => 'https://application.internal',
                'credential_env' => 'OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN', 'read_only' => true,
            ]]);
        },
    ]);
    $client = new Client('https://omnidex.internal', TOKEN, $transport);
    $source = $client->registerDelegatedDataSource(new DelegatedDataSourceInput(
        'Clinical', 'https://application.internal', 'OMNIDEX_DELEGATED_AUTHORITY_APPLICATION_TOKEN',
    ));
    same('source-1', $source->id, 'source ID');
    same(0, $transport->remaining(), 'remaining calls');
}

function testChannelAndMessage(): void
{
    $authority = 'dba_' . str_repeat('a', 64);
    $prompt = "  Find the knee collection.\nKeep context. ";
    $transport = new FakeTransport([
        static fn (): Response => jsonResponse(201, ['channel' => [
            'id' => 'clinical-chat', 'scope' => 'user', 'name' => 'Clinical', 'tags' => ['clinical'],
            'project_id' => 9, 'workspace_root' => '/srv/app', 'data_source_id' => 'source-1', 'mode' => 'assistant',
        ]]),
        static function (string $method, string $url, array $headers, ?string $body) use ($authority, $prompt): Response {
            same('https://omnidex.internal/v1/integrations/channels/clinical-chat/messages', $url, 'message URL');
            same(['prompt' => $prompt, 'delegated_data_authority_id' => $authority],
                json_decode((string) $body, true, 512, JSON_THROW_ON_ERROR), 'message body');
            return jsonResponse(202, [
                'channel' => ['id' => 'clinical-chat', 'scope' => 'user', 'data_source_id' => 'source-1', 'mode' => 'assistant'],
                'user_message' => ['id' => 12, 'channel_id' => 'clinical-chat', 'role' => 'user',
                    'content' => $prompt, 'created_at' => '2026-08-19T00:00:00Z'],
                'job' => ['id' => 73, 'instruction' => $prompt, 'pipeline' => 'chat'],
            ]);
        },
    ]);
    $client = new Client('https://omnidex.internal', TOKEN, $transport);
    $channel = $client->createChannel(new CreateChannelInput(
        'clinical-chat', 'Clinical', ['clinical'], '/srv/app', 'source-1',
    ));
    same('source-1', $channel->dataSourceId, 'channel source');
    $result = $client->sendMessage('clinical-chat', new SendMessageInput($prompt, $authority));
    same(73, $result->job->id, 'job ID');
    same($prompt, $result->userMessage->content, 'exact prompt');
}

function testFailureBoundaries(): void
{
    $transport = new FakeTransport([
        static fn (): Response => jsonResponse(200, [
            'channel_id' => 'clinical-chat', 'messages' => [], 'next_before_id' => null,
            'has_more' => false, 'unknown' => true,
        ]),
        static fn (): Response => jsonResponse(409, ['error' => 'channel already has an active turn']),
    ]);
    $client = new Client('https://omnidex.internal', TOKEN, $transport);
    throws(fn () => $client->listMessages('clinical-chat'), UnexpectedValueException::class, 'unknown or missing');
    $error = throws(
        fn () => $client->sendMessage('clinical-chat', new SendMessageInput('question')),
        OmnidexApiException::class,
        'channel already has an active turn',
    );
    same(409, $error->status, 'API status');

    $never = new FakeTransport([]);
    $bounded = new Client('https://omnidex.internal', TOKEN, $never);
    throws(
        fn () => $bounded->sendMessage('clinical-chat', new SendMessageInput('question', 'invalid')),
        InvalidArgumentException::class,
        'opaque dba_',
    );
    throws(fn () => new Client('file:///tmp/omnidex', TOKEN), InvalidArgumentException::class, 'HTTP');
    throws(fn () => new Client('https://omnidex.internal/', TOKEN), InvalidArgumentException::class, 'trailing slash');
    throws(fn () => new Client('https://omnidex.internal', 'short'), InvalidArgumentException::class, '32');
    throws(
        fn () => $bounded->registerDelegatedDataSource(new DelegatedDataSourceInput(
            'Clinical', 'https://application.internal', 'OPENAI_API_KEY',
        )),
        InvalidArgumentException::class,
        'dedicated namespace',
    );
    Authority::validateDelegatedId(Authority::createDelegatedId());
}

testDelegatedRegistration();
testChannelAndMessage();
testFailureBoundaries();
fwrite(STDOUT, "PHP SDK contract tests passed.\n");
