<?php

declare(strict_types=1);

namespace Omnidex\Integration;

use Omnidex\Integration\Http\NativeTransport;
use Omnidex\Integration\Http\Response;
use Omnidex\Integration\Http\Transport;

final class Client
{
    private const MAX_RESPONSE_BYTES = 16 * 1024 * 1024;

    private readonly Transport $transport;

    public function __construct(
        private readonly string $baseUrl,
        private readonly string $token,
        ?Transport $transport = null,
        private readonly int $timeoutSeconds = 30,
    ) {
        Validation::configuration($baseUrl, $token);
        if ($timeoutSeconds < 1) {
            throw new \InvalidArgumentException('HTTP timeout must be a positive number of seconds.');
        }
        $this->transport = $transport ?? new NativeTransport();
    }

    public function registerDirectDataSource(DirectDataSourceInput $input): DataSource
    {
        Validation::directDataSource($input);
        return $this->registerDataSource([
            'name' => $input->name, 'driver' => 'postgres', 'execution_mode' => 'direct',
            'host' => $input->host, 'port' => $input->port, 'database_name' => $input->databaseName,
            'username' => $input->username, 'password' => $input->password, 'ssl_mode' => $input->sslMode,
            'use_dsn' => $input->useDsn, 'dsn' => $input->dsn,
            'authority_url' => '', 'credential_env' => '',
        ]);
    }

    public function registerDelegatedDataSource(DelegatedDataSourceInput $input): DataSource
    {
        Validation::delegatedDataSource($input);
        return $this->registerDataSource([
            'name' => $input->name, 'driver' => 'postgres', 'execution_mode' => 'delegated',
            'host' => '', 'port' => 0, 'database_name' => '', 'username' => '', 'password' => '',
            'ssl_mode' => '', 'use_dsn' => false, 'dsn' => '',
            'authority_url' => $input->authorityUrl, 'credential_env' => $input->credentialEnv,
        ]);
    }

    public function createChannel(CreateChannelInput $input): Channel
    {
        Validation::channel($input);
        $response = $this->request('POST', '/v1/integrations/channels', [
            'id' => $input->id, 'name' => $input->name, 'tags' => $input->tags,
            'workspace_root' => $input->workspaceRoot, 'data_source_id' => $input->dataSourceId,
            'mode' => 'assistant',
        ], 201);
        return ResponseDecoder::channelEnvelope($response, $input->id, $input->dataSourceId);
    }

    public function getChannel(string $channelId): Channel
    {
        Validation::canonicalId('Channel ID', $channelId, 96);
        $response = $this->request('GET', '/v1/integrations/channels/' . $channelId, null, 200);
        return ResponseDecoder::channelEnvelope($response, $channelId, null);
    }

    public function sendMessage(string $channelId, SendMessageInput $input): SendMessageResult
    {
        Validation::canonicalId('Channel ID', $channelId, 96);
        Validation::prompt($input->prompt);
        $body = ['prompt' => $input->prompt];
        if ($input->delegatedDataAuthorityId !== null) {
            Authority::validateDelegatedId($input->delegatedDataAuthorityId);
            $body['delegated_data_authority_id'] = $input->delegatedDataAuthorityId;
        }
        $response = $this->request(
            'POST', '/v1/integrations/channels/' . $channelId . '/messages', $body, 202,
        );
        return ResponseDecoder::messageEnvelope($response, $channelId, $input->prompt);
    }

    public function listMessages(string $channelId, int $limit = 24, ?int $beforeId = null): MessagePage
    {
        Validation::canonicalId('Channel ID', $channelId, 96);
        if ($limit < 1 || $limit > 200 || ($beforeId !== null && $beforeId < 1)) {
            throw new \InvalidArgumentException('Message page bounds are invalid.');
        }
        $query = ['limit' => (string) $limit];
        if ($beforeId !== null) {
            $query['before_id'] = (string) $beforeId;
        }
        $path = '/v1/integrations/channels/' . $channelId . '/messages?'
            . http_build_query($query, '', '&', PHP_QUERY_RFC3986);
        return ResponseDecoder::messagePage($this->request('GET', $path, null, 200), $channelId);
    }

    public function getJob(int $jobId): JobDetails
    {
        if ($jobId < 1) {
            throw new \InvalidArgumentException('Job ID must be positive.');
        }
        return ResponseDecoder::jobDetails(
            $this->request('GET', '/v1/integrations/jobs/' . $jobId, null, 200),
            $jobId,
        );
    }

    /** @param array<string, mixed> $body */
    private function registerDataSource(array $body): DataSource
    {
        return ResponseDecoder::dataSource(
            $this->request('POST', '/v1/integrations/data-sources', $body, 201),
        );
    }

    /** @param array<string, mixed>|null $body @return array<string, mixed> */
    private function request(string $method, string $path, ?array $body, int $expectedStatus): array
    {
        $encoded = $body === null ? null : json_encode(
            $body,
            JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES | JSON_UNESCAPED_UNICODE,
        );
        $headers = ['Authorization' => 'Bearer ' . $this->token, 'Accept' => 'application/json'];
        if ($body !== null) {
            $headers['Content-Type'] = 'application/json';
        }
        $response = $this->transport->send(
            $method,
            $this->baseUrl . $path,
            $headers,
            $encoded,
            $this->timeoutSeconds,
        );
        if (strlen($response->body) > self::MAX_RESPONSE_BYTES) {
            throw new \RuntimeException('Omnidex integration response exceeds 16777216 bytes.');
        }
        if ($response->status !== $expectedStatus) {
            try {
                $error = ResponseDecoder::json($response->body);
            } catch (\Throwable) {
                $error = [];
            }
            throw ResponseDecoder::error($error, $response->status);
        }
        $mediaType = strtolower(trim(explode(';', $response->header('content-type'), 2)[0]));
        if ($mediaType !== 'application/json') {
            throw new \UnexpectedValueException('Omnidex returned a non-JSON response.');
        }
        return ResponseDecoder::json($response->body);
    }
}
